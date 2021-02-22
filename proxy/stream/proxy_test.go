package stream

import (
	"bytes"
	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"io"
	"io/ioutil"
	"log"
	"testing"
	"time"
)

func Test_Stream_Successfully_Sent_To_Client(t *testing.T) {
	// start stream proxy
	go startWebSocketStreamProxyServer()

	// Workaround to wait for backend and reverse proxy to start
	// Wait groups won't work for some reason
	time.Sleep(1 * time.Millisecond)

	conn, resp, err := websocket.DefaultDialer.Dial("ws://localhost:8081/", nil)
	// Verify connection was successfully established
	require.Nilf(t, err, "Error connecting to WebSocket endpoint. Err = %v", err)

	t.Log(resp)
	// Verify response has Switching Protocols status
	require.Equalf(t, fiber.StatusSwitchingProtocols, resp.StatusCode, "Initial webSocket handshake was failed. Status code should be 101")

	// client read echo message
	messageType, recvmsg, err := conn.ReadMessage()
	// Verify client can read the message from client conection
	require.Nilf(t, err, "Unable to read message from WebSocket connection")

	expectedMsg := "hello"

	t.Logf("Recieved message - %s", recvmsg)
	// Verify request and response content are the same
	require.Equal(t, expectedMsg, string(recvmsg))

	// Verify message equality
	require.Equalf(t, websocket.BinaryMessage, messageType, "Actual message is not BinaryMessage, but should be")
	require.Equal(t, []byte(expectedMsg), recvmsg)

}

func Test_Stream_Proxy_Return_Error_When_PrepareStream_Function_Is_Nil(t *testing.T) {

	// start stream proxy
	go startBrokenStreamProxy()

	// Workaround to wait for backend and reverse proxy to start
	// Wait groups won't work for some reason
	time.Sleep(1 * time.Millisecond)

	conn, resp, err := websocket.DefaultDialer.Dial("ws://localhost:8090/", nil)
	require.Nilf(t, conn, "Connection should be nil if stream proxy was not set up in a right way")
	require.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
	require.NotNil(t, err)

}

// Function to start working stream proxy
func startWebSocketStreamProxyServer() {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	wsp := &WSStreamReverseProxy{
		PrepareStream: func() (io.ReadCloser, error) {
			return ioutil.NopCloser(bytes.NewReader([]byte("hello"))), nil
		},
	}
	app.Get("/", func(ctx *fiber.Ctx) error {
		return wsp.ProxyStream(ctx)
	})

	if err := app.Listen(":8081"); err != nil {
		log.Fatalf("websocket proxy server `Listen` quit, err=%v", err)
	}

}

// Function to start broken stream proxy
func startBrokenStreamProxy() {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	wsp := &WSStreamReverseProxy{}
	app.Get("/", func(ctx *fiber.Ctx) error {
		return wsp.ProxyStream(ctx)
	})

	if err := app.Listen(":8090"); err != nil {
		log.Fatalf("websocket proxy server `Listen` quit, err=%v", err)
	}

}
