package websocket

import (
	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"log"
	"testing"
	"time"
)

func TestWebSocketHandlerCanHandleConnections(t *testing.T) {
	// Start WebSocket echo server
	go startWebSocketServer()

	time.Sleep(1 * time.Millisecond)

	// Connect to websocket echo server
	conn, resp, err := websocket.DefaultDialer.Dial("ws://localhost:8080/echo", nil)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	assert.NotNil(t, conn)
	assert.Equal(t, fiber.StatusSwitchingProtocols, resp.StatusCode)

	t.Logf("Initial response after WebSocket handshake resp=%v", resp)

	message := []byte("Hello from test")
	// Verify can write the message from client connection
	err = conn.WriteMessage(websocket.BinaryMessage, message)
	assert.Nil(t, err)

	// Verify can read response from server
	msgType, rcvMsg, err := conn.ReadMessage()
	assert.Nil(t, err)
	assert.NotNil(t, rcvMsg)
	assert.Equal(t, websocket.BinaryMessage, msgType)
	assert.Equal(t, message, rcvMsg)
}

func startWebSocketServer() {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	app.Get("/echo", HandleWebSocket(func(c *Conn) {
		var (
			mt  int
			msg []byte
			err error
		)
		for {
			if mt, msg, err = c.ReadMessage(); err != nil {
				log.Println("read:", err)
				break
			}
			log.Printf("recv: %s", msg)

			if err = c.WriteMessage(mt, msg); err != nil {
				log.Println("write:", err)
				break
			}
		}
	}))

	if err := app.Listen(":8080"); err != nil {
		log.Fatalf("websocket backend server `Listen` quit, err=%v", err)
	}

}
