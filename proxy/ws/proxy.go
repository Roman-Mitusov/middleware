package ws

import (
	"bytes"
	"github.com/Roman-Mitusov/middleware/proxy"
	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"io"
	"net"
	"net/http"
)

var (
	// Default web socket upgrader to upgrade the incoming connections
	DefaultWebSocketUpgrader = &websocket.FastHTTPUpgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	// Default websocket dialer to establish connections with backends
	DefaultWebSocketDialer = websocket.DefaultDialer
)

type ReverseWSProxy struct {
	PrepareRequest func(ctx *fiber.Ctx) error
	Upgrader       *websocket.FastHTTPUpgrader
	Dialer         *websocket.Dialer
}

func (wsp *ReverseWSProxy) ProxyWS(ctx *fiber.Ctx) (err error) {
	logger := proxy.DefaultLogger()

	if b := websocket.FastHTTPIsWebSocketUpgrade(ctx.Context()); b {
		logger.Infof("Request is upgraded %v", b)
	}

	var (
		req      = ctx.Request()
		resp     = ctx.Response()
		dialer   = DefaultWebSocketDialer
		upgrader = DefaultWebSocketUpgrader
	)

	if wsp.Dialer != nil {
		dialer = wsp.Dialer
	}

	if wsp.Upgrader != nil {
		upgrader = wsp.Upgrader
	}

	upgrader.CheckOrigin = func(ctx *fasthttp.RequestCtx) bool {
		return true
	}

	if wsp.PrepareRequest != nil {
		if err = wsp.PrepareRequest(ctx); err != nil {
			return err
		}
	}

	// Pass headers from the incoming request to the dialer to forward them to
	// the final destinations.
	requestHeader := http.Header{}
	if origin := req.Header.Peek("Origin"); string(origin) != "" {
		requestHeader.Add("Origin", string(origin))
	}

	if prot := req.Header.Peek("Sec-WebSocket-Protocol"); string(prot) != "" {
		requestHeader.Add("Sec-WebSocket-Protocol", string(prot))
	}

	if cookie := req.Header.Peek("Cookie"); string(cookie) != "" {
		requestHeader.Add("Sec-WebSocket-Protocol", string(cookie))
	}

	if string(req.Host()) != "" {
		requestHeader.Set("Host", string(req.Host()))
	}

	// Pass X-Forwarded-For headers too, code below is a part of
	// httputil.ReverseProxy. See http://en.wikipedia.org/wiki/X-Forwarded-For
	// for more information
	// TODO: use RFC7239 http://tools.ietf.org/html/rfc7239
	if clientIP, _, err := net.SplitHostPort(ctx.IP()); err == nil {
		// If we aren't the first proxy retain prior
		// X-Forwarded-For information as a comma+space
		// separated list and fold multiple headers into one.
		if prior := req.Header.Peek("X-Forwarded-For"); string(prior) != "" {
			clientIP = string(prior) + ", " + clientIP
		}
		requestHeader.Set("X-Forwarded-For", clientIP)
	}

	// Set the originating protocol of the incoming HTTP request. The SSL might
	// be terminated on our site and because we doing proxy adding this would
	// be helpful for applications on the backend.
	requestHeader.Set("X-Forwarded-Proto", "http")
	if ctx.Context().IsTLS() {
		requestHeader.Set("X-Forwarded-Proto", "https")
	}

	targetUrl := req.URI().String()

	backendConnection, backendResp, err := dialer.Dial(targetUrl, requestHeader)
	if err != nil {
		logger.Errorf("websocketproxy: couldn't dial to remote backend url=%s, err=%v", targetUrl, err)
		logger.Infof("resp_backent =%v", backendResp)
		if backendResp != nil {
			if err := wsCopyResponse(resp, backendResp); err != nil {
				logger.Errorf("could not finish wsCopyResponse, err=%v", err)
			}
		} else {
			_ = ctx.Status(fiber.StatusServiceUnavailable).SendString(err.Error())
		}
		return
	}

	// Now upgrade the existing incoming request to a WebSocket connection.
	// Also pass the header that we gathered from the Dial handshake.
	err = upgrader.Upgrade(ctx.Context(), func(clientConn *websocket.Conn) {
		defer clientConn.Close()
		var (
			errClient  = make(chan error, 1)
			errBackend = make(chan error, 1)
			message    string
		)

		logger.Info("Upgrade handler working")
		go replicateWebsocketConn(clientConn, backendConnection, errClient, logger)  // response
		go replicateWebsocketConn(backendConnection, clientConn, errBackend, logger) // request

		for {
			select {
			case err = <-errClient:
				message = "websocketproxy: Error when copying response: %v"
			case err = <-errBackend:
				message = "websocketproxy: Error when copying request: %v"
			}

			// log error except '*websocket.CloseError'
			if _, ok := err.(*websocket.CloseError); !ok {
				logger.Errorf(message, err)
			}
		}
	})

	if err != nil {
		logger.Errorf("websocketproxy: couldn't upgrade %s", err)
		return err
	}

	return nil
}

// replicateWebsocketConn to
// copy message from src to dst
func replicateWebsocketConn(dst, src *websocket.Conn, errChan chan error, logger *logrus.Logger) {
	for {
		msgType, msg, err := src.ReadMessage()
		if err != nil {
			// true: handle websocket close error
			logger.Infof("src.ReadMessage failed, msgType=%d, msg=%s, err=%v", msgType, msg, err)
			if ce, ok := err.(*websocket.CloseError); ok {
				msg = websocket.FormatCloseMessage(ce.Code, ce.Text)
			} else {
				logger.Errorf("src.ReadMessage failed, err=%v", err)
				msg = websocket.FormatCloseMessage(websocket.CloseAbnormalClosure, err.Error())
			}

			errChan <- err
			if err = dst.WriteMessage(websocket.CloseMessage, msg); err != nil {
				logger.Errorf("write close message failed, err=%v", err)
			}
			break
		}

		err = dst.WriteMessage(msgType, msg)
		if err != nil {
			logger.Errorf("dst.WriteMessage failed, err=%v", err)
			errChan <- err
			break
		}
	}
}

// wsCopyResponse .
// to help copy origin websocket response to client
func wsCopyResponse(dst *fasthttp.Response, src *http.Response) error {
	for k, vv := range src.Header {
		for _, v := range vv {
			dst.Header.Add(k, v)
		}
	}

	dst.SetStatusCode(src.StatusCode)
	defer src.Body.Close()

	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, src.Body); err != nil {
		return err
	}
	dst.SetBody(buf.Bytes())
	return nil
}
