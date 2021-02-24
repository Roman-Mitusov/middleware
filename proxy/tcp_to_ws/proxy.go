package tcp_to_ws

import (
	"github.com/Roman-Mitusov/middleware/proxy"
	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/savsgio/gotils"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"io"
	"net"
)

var (
	//Default TCP dialer
	DefaultTcpDialer = &fasthttp.TCPDialer{Concurrency: 1000}

	// Default web socket upgrader to upgrade the incoming connections
	DefaultWebSocketUpgrader = &websocket.FastHTTPUpgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

type TcpToWSProxy struct {
	PrepareRequest func(ctx *fiber.Ctx) error
	TcpDialer      *fasthttp.TCPDialer
	Upgrader       *websocket.FastHTTPUpgrader
}

func (p *TcpToWSProxy) ProxyTcpToWS(ctx *fiber.Ctx) (err error) {
	logger := proxy.DefaultLogger()

	if b := websocket.FastHTTPIsWebSocketUpgrade(ctx.Context()); b {
		logger.Infof("Request is upgraded %v", b)
	}

	var (
		dialer   = DefaultTcpDialer
		upgrader = DefaultWebSocketUpgrader
	)

	if p.TcpDialer != nil {
		dialer = p.TcpDialer
	}

	if p.Upgrader != nil {
		upgrader = p.Upgrader
	}

	if p.PrepareRequest != nil {
		if err = p.PrepareRequest(ctx); err != nil {
			return err
		}
	}

	upgrader.CheckOrigin = func(ctx *fasthttp.RequestCtx) bool {
		return true
	}

	targetHost := gotils.B2S(ctx.Request().URI().Host())

	tcpConn, err := dialer.Dial(targetHost)
	if err != nil {
		logger.Errorf("Unable to establish tcp connection to host=%s with error=%v", targetHost, err)
		return err
	}

	err = upgrader.Upgrade(ctx.Context(), func(clientConn *websocket.Conn) {
		defer clientConn.Close()
		var (
			errClient = make(chan error, 1)
			message   string
		)

		logger.Info("Upgrade handler working")
		go copyTcpResponseToWebSocketConnection(clientConn, tcpConn, errClient, logger)

		for {
			select {
			case err = <-errClient:
				message = "tcptowsproxy: Error when copying response from tcp to ws: %v"
			}

			// log error except '*websocket.CloseError'
			if _, ok := err.(*websocket.CloseError); !ok {
				logger.Errorf(message, err)
			}
		}
	})

	if err != nil {
		logger.Errorf("tcptowsproxy: couldn't upgrade %s", err)
		return err
	}

	return nil
}

func copyTcpResponseToWebSocketConnection(dst *websocket.Conn, src net.Conn, errChan chan error, logger *logrus.Logger) {
	logger.Info("tcptowsproxy: start copying tcp response bytes to client WebSocket connection")

	logger.Info("tcptowsproxy: obtaining WebSocket connection writer....")
	writer, err := dst.NextWriter(websocket.BinaryMessage)
	if err != nil {
		errChan <- err
		logrus.Errorf("tcptowsproxy: error obtaining writer from WebSocket client connection err=%v", err)
	}

	logger.Info("tcptowsproxy: start copying tcp response to WebSocket client connection ...")
	if _, err := io.Copy(writer, src); err != nil {
		errChan <- err
		logger.Errorf("tcptowsproxy: error copy tcp response to WebSocket client connection err=%v", err)
	}
}
