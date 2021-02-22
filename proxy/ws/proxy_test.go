package ws

import (
	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"log"
	"testing"
	"time"
)

func Test_WebSocket_Reverse_Proxy(t *testing.T) {
	//start backend
	go startWebSocketBackendServerForTest()

	//start reverse proxy
	go startWebSocketReverseProxyServer()

	// Workaround to wait for backend and reverse proxy to start
	// Wait groups won't work for some reason
	time.Sleep(1 * time.Millisecond)

	// Establish client web socket connection to reverse proxy
	conn, resp, err := websocket.DefaultDialer.Dial("ws://localhost:8081/", nil)
	// Verify connection was successfully established
	require.Nilf(t, err, "WebSocket handshake error - %v", err)

	t.Log(resp)
	// Verify response has Switching Protocols status
	require.Equalf(t, fiber.StatusSwitchingProtocols, resp.StatusCode, "Could not connect to proxy. Response status code should be 101")

	// Client Web Socket request message
	sendmsg := []byte("hello")
	// Verify message can be sent to web socket reverse proxy
	err = conn.WriteMessage(websocket.TextMessage, sendmsg)
	require.Nilf(t, err, "Could not write message msg = %s to WebSocket connection because of err = %v", sendmsg, err)

	// client read echo message
	messageType, recvmsg, err := conn.ReadMessage()
	// Verify client can read the message from client conection
	require.Nilf(t, err, "Unable to reqd message from WebSocket connection. The cause is -  %v", err)

	t.Logf("Recieved message - %s", recvmsg)
	// Verify request and response content are the same
	require.Equal(t, string(sendmsg), string(recvmsg))

	// Verify message equality
	require.Equal(t, websocket.TextMessage, messageType)
	require.Equal(t, sendmsg, recvmsg)

}

// Function to start backend server serving the WebSocket connections
func startWebSocketBackendServerForTest() {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	upgrader := websocket.FastHTTPUpgrader{}
	app.Get("/echo", func(ctx *fiber.Ctx) error {
		err := upgrader.Upgrade(ctx.Context(), func(ws *websocket.Conn) {
			defer ws.Close()
			for {
				mt, message, err := ws.ReadMessage()
				if err != nil {
					log.Println("read:", err)
					break
				}
				log.Printf("recv: %s", message)
				err = ws.WriteMessage(mt, message)
				if err != nil {
					log.Println("write:", err)
					break
				}
			}
		})

		if err != nil {
			if _, ok := err.(websocket.HandshakeError); ok {
				log.Println(err)
			}
			return err
		}
		return nil
	})

	if err := app.Listen(":8080"); err != nil {
		log.Fatalf("websocket backend server `Listen` quit, err=%v", err)
	}

}

// Function to start WebSocket reverse proxy
func startWebSocketReverseProxyServer() {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	wsp := &ReverseWSProxy{
		PrepareRequest: func(ctx *fiber.Ctx) error {
			ctx.Request().SetRequestURI("ws://localhost:8080/echo")
			return nil
		},
	}
	app.Get("/", func(ctx *fiber.Ctx) error {
		return wsp.ProxyWS(ctx)
	})

	if err := app.Listen(":8081"); err != nil {
		log.Fatalf("websocket proxy server `Listen` quit, err=%v", err)
	}

}
