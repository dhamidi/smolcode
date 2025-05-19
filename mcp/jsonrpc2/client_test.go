package jsonrpc2

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCallSuccess(t *testing.T) {
	clientReadFromServer := new(bytes.Buffer) // Server writes to this, client reads from it
	clientWriteToServer := new(bytes.Buffer)  // Client writes to this, server reads from it

	// Create a client connection that reads from clientReadFromServer and writes to clientWriteToServer
	c := Connect(clientReadFromServer, clientWriteToServer)

	go func() {
		t.Logf("Server: Goroutine started")
		// Client Connects to (reader, writer)
		// Client writes its requests to 'writer'. Data flows writer -> reader.
		// Client reads responses from 'reader'.
		// Server goroutine must decode from 'reader' and encode to 'writer'.

		serverDecoder := json.NewDecoder(clientWriteToServer)  // Server reads from the buffer client writes to
		serverEncoder := json.NewEncoder(clientReadFromServer) // Server writes to the buffer client reads from

		t.Logf("Server: Decoding request...")
		var clientReq Request
		if err := serverDecoder.Decode(&clientReq); err != nil {
			// If server fails to decode, client call will likely fail/timeout.
			// Consider t.Log or similar if debugging hangs.
			t.Logf("Server: Error decoding request: %v", err)
			return
		}
		t.Logf("Server: Request decoded: %+v", clientReq)

		// Simulate successful response
		respToSend := &Response{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`{"result": "success"}`),
			ID:      clientReq.ID,
		}
		t.Logf("Server: Encoding response: %+v", respToSend)
		if errEncode := serverEncoder.Encode(respToSend); errEncode != nil {
			t.Logf("Server: Error encoding response: %v", errEncode)
		} else {
			t.Logf("Server: Response encoded and sent")
		}
		// Send a notification to keep the readLoop engaged
		notification := &Request{
			JSONRPC: "2.0",
			Method:  "server.event",
			Params:  map[string]string{"data": "keepalive"},
			// ID is omitted for notifications, making it a Notification
		}
		t.Logf("Server: Encoding notification: %+v", notification)
		if errNotify := serverEncoder.Encode(notification); errNotify != nil {
			t.Logf("Server: Error encoding notification: %v", errNotify)
		} else {
			t.Logf("Server: Notification encoded and sent")
		}

		t.Logf("Server: Goroutine blocking to keep connection alive for test")
		select {} // Block forever
	}()

	var result map[string]string
	err := c.Call(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"}, &result)
	assert.NoError(t, err, "c.Call should succeed without error")
	assert.NotNil(t, result, "Result map should not be nil after successful call")
	assert.Equal(t, "success", result["result"], "Result field did not match")
}
