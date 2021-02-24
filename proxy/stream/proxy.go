package stream

import (
	"bytes"
	"fmt"
	"github.com/Roman-Mitusov/middleware/proxy"
	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"io"
)

// Default web socket upgrader to upgrade the incoming connections
var DefaultWebSocketUpgrader = &websocket.FastHTTPUpgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type WSStreamReverseProxy struct {
	// This method should not be nul
	// It prepares the stream which should be send to client
	// As an example streaming kubernetes Pod logs somewhere, for example, using selenoid-ui
	// Error will be thrown if this field will be set to nil or not used
	// Mandatory
	PrepareStream func() (io.ReadCloser, error)
	// Upgrader is used to upgrade regular http connection into WenSocket one
	Upgrader *websocket.FastHTTPUpgrader
}

func (p *WSStreamReverseProxy) ProxyStream(ctx *fiber.Ctx) error {

	logger := proxy.DefaultLogger()

	if b := websocket.FastHTTPIsWebSocketUpgrade(ctx.Context()); b {
		logger.Infof("Request is upgraded %v", b)
	}

	var upgrader = DefaultWebSocketUpgrader
	// Set Check origin to false because we don't connect to other server but copy stream directly to opened connection
	upgrader.CheckOrigin = func(ctx *fasthttp.RequestCtx) bool {
		return false
	}

	if p.Upgrader != nil {
		upgrader = p.Upgrader
	}

	if p.PrepareStream == nil {
		return fmt.Errorf("unable to obatain stream to proxy to client connection. Please prepare the stream")
	}

	if stream, err := p.PrepareStream(); err == nil {
		err = upgrader.Upgrade(ctx.Context(), func(clientConn *websocket.Conn) {
			defer clientConn.Close()
			var (
				streamError = make(chan error, 1)
				message     string
			)

			logger.Info("Upgrade handler working")
			go sendStreamResponseToClientConn(stream, clientConn, streamError, logger)

			for {
				select {
				case err = <-streamError:
					message = "websocketstreamproxy: Error when streaming the response: %v"
				}

				// log error except '*websocket.CloseError'
				if _, ok := err.(*websocket.CloseError); !ok {
					logger.Errorf(message, err)
				}
			}
		})
		if err != nil {
			logger.Errorf("websocketstreamproxy: couldn't upgrade %s", err)
			return err
		}
	} else {
		logger.Errorf("No stream available to proxy to client web socket connection. PrepareStream function returned err = %v", err)
		return err
	}

	return nil
}

// Send content of the stream response to client web socket connection
func sendStreamResponseToClientConn(streamResponse io.ReadCloser, clientConn *websocket.Conn, errChan chan error, logger *logrus.Logger) {
	defer streamResponse.Close()
	buff := bytes.NewBuffer(nil)
	if _, err := io.Copy(buff, streamResponse); err != nil {
		logger.Errorf("Unable to copy stream response to buffer. The cause is - %v", err)
		errChan <- err
	}

	if err := clientConn.WriteMessage(websocket.BinaryMessage, buff.Bytes()); err != nil {
		logger.Errorf("Unable to write stream response to client connection. The cause is - %v", err)
		errChan <- err
	}
}
