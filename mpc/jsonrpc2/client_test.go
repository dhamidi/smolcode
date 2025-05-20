package jsonrpc2_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mpc/jsonrpc2" // Assuming mpc/jsonrpc2 is the module path
)

// FakeTransport is a mock implementation of the jsonrpc2.Transport interface for testing.
type FakeTransport struct {
	ToClient   chan []byte // Payloads pushed here by the test will be received by the client's Receive call
	FromClient chan []byte // Payloads sent by the client via Send will appear here for the test to inspect

	sendErr    error
	receiveErr error
	mu         sync.Mutex
	closed     bool
	closeChan  chan struct{} // Used to signal Receive that Close has been called
}

// NewFakeTransport creates a new FakeTransport with initialized channels.
func NewFakeTransport() *FakeTransport {
	return &FakeTransport{
		ToClient:   make(chan []byte, 10), // Buffered to prevent deadlocks in simple tests
		FromClient: make(chan []byte, 10),
		closeChan:  make(chan struct{}),
	}
}

// Close implements the io.Closer interface.
// It signals that the transport is closed and unblocks any pending Receive calls.
func (ft *FakeTransport) Close() error {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	if ft.closed {
		return nil // Already closed
	}

	ft.closed = true
	close(ft.closeChan) // Signal Receive to stop

	// Closing these channels will cause any active Send/Receive on them to panic if not handled.
	// However, the select statements in Send/Receive with ft.closeChan should handle this.
	// It also signals to any test goroutines ranging over them that they are done.
	// It's generally safe to close channels that are written to by one party and read by another
	// once the writer is done, or in a cleanup like this.
	close(ft.FromClient)
	close(ft.ToClient)

	return nil
}

// PushToClient sends a payload to the client as if it came from the server.
// This is a helper for tests to inject messages into the client's Receive loop.
func (ft *FakeTransport) PushToClient(payload []byte) error {
	ft.mu.Lock()
	if ft.closed {
		ft.mu.Unlock()
		return io.ErrClosedPipe
	}
	ft.mu.Unlock()

	select {
	case ft.ToClient <- payload:
		return nil
	case <-ft.closeChan:
		return io.ErrClosedPipe
	}
}

// PopFromClient retrieves a payload sent by the client, with a timeout.
// This is a helper for tests to get messages the client has sent.
func (ft *FakeTransport) PopFromClient(t *testing.T, timeout time.Duration) []byte {
	t.Helper()
	select {
	case payload, ok := <-ft.FromClient:
		if !ok {
			t.Log("PopFromClient: FromClient channel was closed")
			return nil // Channel closed
		}
		return payload
	case <-time.After(timeout):
		t.Errorf("PopFromClient: timed out after %v waiting for message from client", timeout)
		return nil
	}
}

// SetSendError sets an error to be returned by the next Send call.
func (ft *FakeTransport) SetSendError(err error) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.sendErr = err
}

// SetReceiveError sets an error to be returned by the next Receive call.
func (ft *FakeTransport) SetReceiveError(err error) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.receiveErr = err
}

func TestClient_Call_Success(t *testing.T) {
	transport := NewFakeTransport()
	client := jsonrpc2.NewClient(transport)

	// Start the client's listener in a goroutine
	go func() {
		err := client.Listen()
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.ErrClosedPipe) {
			t.Logf("Client Listen exited with error: %v", err)
			// We might not want to fail the test here if the error is expected during close
		}
	}()
	defer func() {
		err := client.Close() // This will also close the transport
		assert.NoError(t, err, "Client Close should not error")
	}()

	type SumParams struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	type SumResult struct {
		Total int `json:"total"`
	}

	callArgs := jsonrpc2.ClientCallArgs{
		Method: "calculator.sum",
		Params: SumParams{A: 5, B: 3},
	}
	var resultDest SumResult
	var requestID uint64 // To capture the ID for the response

	// Goroutine to simulate server receiving request and sending response
	go func() {
		// 1. Receive request from client
		reqBytes := transport.PopFromClient(t, 2*time.Second)
		require.NotNil(t, reqBytes, "Should have received request from client")

		var receivedReq jsonrpc2.Request
		err := json.Unmarshal(reqBytes, &receivedReq)
		require.NoError(t, err, "Failed to unmarshal request from client")

		assert.Equal(t, "2.0", receivedReq.JSONRPC)
		assert.Equal(t, callArgs.Method, receivedReq.Method)

		// Capture the ID (assuming it's a number that comes as float64)
		idFloat, ok := receivedReq.ID.(float64)
		require.True(t, ok, "Request ID should be a float64 (from JSON number)")
		requestID = uint64(idFloat)
		assert.Greater(t, requestID, uint64(0), "Request ID should be positive")

		// Check params (optional, but good practice)
		var receivedParams SumParams
		paramsBytes, _ := json.Marshal(receivedReq.Params)
		json.Unmarshal(paramsBytes, &receivedParams)
		assert.Equal(t, callArgs.Params.(SumParams).A, receivedParams.A)
		assert.Equal(t, callArgs.Params.(SumParams).B, receivedParams.B)

		// 2. Prepare and send response
		responsePayload := fmt.Sprintf(`{"jsonrpc": "2.0", "id": %d, "result": {"total": %d}}`,
			requestID,
			callArgs.Params.(SumParams).A+callArgs.Params.(SumParams).B,
		)
		err = transport.PushToClient([]byte(responsePayload))
		require.NoError(t, err, "Failed to push response to client")
	}()

	// Make the call
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Call(ctx, callArgs, &resultDest)

	assert.NoError(t, err, "client.Call should succeed")
	assert.Equal(t, 8, resultDest.Total, "Result total should be correct")

	// Give a brief moment for the client.Close() to propagate if needed, though PopFromClient handles timeout
	time.Sleep(10 * time.Millisecond)
}

// Receive implements the jsonrpc2.Transport interface.
// It receives a payload from the ToClient channel, which the test can push to.
func (ft *FakeTransport) Receive(ctx context.Context) ([]byte, error) {
	ft.mu.Lock()
	if ft.closed && len(ft.ToClient) == 0 { // If closed and no pending messages, return error immediately
		ft.mu.Unlock()
		return nil, io.ErrClosedPipe
	}
	receiveErr := ft.receiveErr
	ft.mu.Unlock()

	if receiveErr != nil {
		return nil, receiveErr
	}

	select {
	case payload, ok := <-ft.ToClient:
		if !ok { // Channel closed, means transport is definitively closed
			ft.mu.Lock()
			ft.closed = true // Ensure consistent state
			ft.mu.Unlock()
			return nil, io.ErrClosedPipe
		}
		return payload, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-ft.closeChan: // Explicit close signal
		return nil, io.ErrClosedPipe
	}
}

// Close implements the io.Closer interface.
// It signals that the transport is closed and unblocks any pending Receive calls.
func (ft *FakeTransport) Close() error {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	if ft.closed {
		return nil // Already closed
	}

	ft.closed = true
	close(ft.closeChan) // Signal Receive to stop

	// Closing these channels will cause any active Send/Receive on them to panic if not handled.
	// However, the select statements in Send/Receive with ft.closeChan should handle this.
	// It also signals to any test goroutines ranging over them that they are done.
	// It's generally safe to close channels that are written to by one party and read by another
	// once the writer is done, or in a cleanup like this.
	close(ft.FromClient)
	close(ft.ToClient)

	return nil
}

// PushToClient sends a payload to the client as if it came from the server.
// This is a helper for tests to inject messages into the client's Receive loop.
func (ft *FakeTransport) PushToClient(payload []byte) error {
	ft.mu.Lock()
	if ft.closed {
		ft.mu.Unlock()
		return io.ErrClosedPipe
	}
	ft.mu.Unlock()

	select {
	case ft.ToClient <- payload:
		return nil
	case <-ft.closeChan:
		return io.ErrClosedPipe
	}
}

// PopFromClient retrieves a payload sent by the client, with a timeout.
// This is a helper for tests to get messages the client has sent.
func (ft *FakeTransport) PopFromClient(t *testing.T, timeout time.Duration) []byte {
	t.Helper()
	select {
	case payload, ok := <-ft.FromClient:
		if !ok {
			t.Log("PopFromClient: FromClient channel was closed")
			return nil // Channel closed
		}
		return payload
	case <-time.After(timeout):
		t.Errorf("PopFromClient: timed out after %v waiting for message from client", timeout)
		return nil
	}
}

// SetSendError sets an error to be returned by the next Send call.
func (ft *FakeTransport) SetSendError(err error) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.sendErr = err
}

// SetReceiveError sets an error to be returned by the next Receive call.
func (ft *FakeTransport) SetReceiveError(err error) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.receiveErr = err
}

func TestClient_Call_Success(t *testing.T) {
	transport := NewFakeTransport()
	client := jsonrpc2.NewClient(transport)

	// Start the client's listener in a goroutine
	go func() {
		err := client.Listen()
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.ErrClosedPipe) {
			t.Logf("Client Listen exited with error: %v", err)
			// We might not want to fail the test here if the error is expected during close
		}
	}()
	defer func() {
		err := client.Close() // This will also close the transport
		assert.NoError(t, err, "Client Close should not error")
	}()

	type SumParams struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	type SumResult struct {
		Total int `json:"total"`
	}

	callArgs := jsonrpc2.ClientCallArgs{
		Method: "calculator.sum",
		Params: SumParams{A: 5, B: 3},
	}
	var resultDest SumResult
	var requestID uint64 // To capture the ID for the response

	// Goroutine to simulate server receiving request and sending response
	go func() {
		// 1. Receive request from client
		reqBytes := transport.PopFromClient(t, 2*time.Second)
		require.NotNil(t, reqBytes, "Should have received request from client")

		var receivedReq jsonrpc2.Request
		err := json.Unmarshal(reqBytes, &receivedReq)
		require.NoError(t, err, "Failed to unmarshal request from client")

		assert.Equal(t, "2.0", receivedReq.JSONRPC)
		assert.Equal(t, callArgs.Method, receivedReq.Method)

		// Capture the ID (assuming it's a number that comes as float64)
		idFloat, ok := receivedReq.ID.(float64)
		require.True(t, ok, "Request ID should be a float64 (from JSON number)")
		requestID = uint64(idFloat)
		assert.Greater(t, requestID, uint64(0), "Request ID should be positive")

		// Check params (optional, but good practice)
		var receivedParams SumParams
		paramsBytes, _ := json.Marshal(receivedReq.Params)
		json.Unmarshal(paramsBytes, &receivedParams)
		assert.Equal(t, callArgs.Params.(SumParams).A, receivedParams.A)
		assert.Equal(t, callArgs.Params.(SumParams).B, receivedParams.B)

		// 2. Prepare and send response
		responsePayload := fmt.Sprintf(`{"jsonrpc": "2.0", "id": %d, "result": {"total": %d}}`,
			requestID,
			callArgs.Params.(SumParams).A+callArgs.Params.(SumParams).B,
		)
		err = transport.PushToClient([]byte(responsePayload))
		require.NoError(t, err, "Failed to push response to client")
	}()

	// Make the call
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Call(ctx, callArgs, &resultDest)

	assert.NoError(t, err, "client.Call should succeed")
	assert.Equal(t, 8, resultDest.Total, "Result total should be correct")

	// Give a brief moment for the client.Close() to propagate if needed, though PopFromClient handles timeout
	time.Sleep(10 * time.Millisecond)
}

// Send implements the jsonrpc2.Transport interface.
// It sends the payload to the FromClient channel for the test to inspect.
func (ft *FakeTransport) Send(ctx context.Context, payload []byte) error {
	ft.mu.Lock()
	if ft.closed {
		ft.mu.Unlock()
		return io.ErrClosedPipe
	}
	sendErr := ft.sendErr
	ft.mu.Unlock()

	if sendErr != nil {
		return sendErr
	}

	select {
	case ft.FromClient <- payload:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-ft.closeChan: // If transport is closed while trying to send
		return io.ErrClosedPipe
	}
}

// Close implements the io.Closer interface.
// It signals that the transport is closed and unblocks any pending Receive calls.
func (ft *FakeTransport) Close() error {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	if ft.closed {
		return nil // Already closed
	}

	ft.closed = true
	close(ft.closeChan) // Signal Receive to stop

	// Closing these channels will cause any active Send/Receive on them to panic if not handled.
	// However, the select statements in Send/Receive with ft.closeChan should handle this.
	// It also signals to any test goroutines ranging over them that they are done.
	// It's generally safe to close channels that are written to by one party and read by another
	// once the writer is done, or in a cleanup like this.
	close(ft.FromClient)
	close(ft.ToClient)

	return nil
}

// PushToClient sends a payload to the client as if it came from the server.
// This is a helper for tests to inject messages into the client's Receive loop.
func (ft *FakeTransport) PushToClient(payload []byte) error {
	ft.mu.Lock()
	if ft.closed {
		ft.mu.Unlock()
		return io.ErrClosedPipe
	}
	ft.mu.Unlock()

	select {
	case ft.ToClient <- payload:
		return nil
	case <-ft.closeChan:
		return io.ErrClosedPipe
	}
}

// PopFromClient retrieves a payload sent by the client, with a timeout.
// This is a helper for tests to get messages the client has sent.
func (ft *FakeTransport) PopFromClient(t *testing.T, timeout time.Duration) []byte {
	t.Helper()
	select {
	case payload, ok := <-ft.FromClient:
		if !ok {
			t.Log("PopFromClient: FromClient channel was closed")
			return nil // Channel closed
		}
		return payload
	case <-time.After(timeout):
		t.Errorf("PopFromClient: timed out after %v waiting for message from client", timeout)
		return nil
	}
}

// SetSendError sets an error to be returned by the next Send call.
func (ft *FakeTransport) SetSendError(err error) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.sendErr = err
}

// SetReceiveError sets an error to be returned by the next Receive call.
func (ft *FakeTransport) SetReceiveError(err error) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.receiveErr = err
}

func TestClient_Call_Success(t *testing.T) {
	transport := NewFakeTransport()
	client := jsonrpc2.NewClient(transport)

	// Start the client's listener in a goroutine
	go func() {
		err := client.Listen()
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.ErrClosedPipe) {
			t.Logf("Client Listen exited with error: %v", err)
			// We might not want to fail the test here if the error is expected during close
		}
	}()
	defer func() {
		err := client.Close() // This will also close the transport
		assert.NoError(t, err, "Client Close should not error")
	}()

	type SumParams struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	type SumResult struct {
		Total int `json:"total"`
	}

	callArgs := jsonrpc2.ClientCallArgs{
		Method: "calculator.sum",
		Params: SumParams{A: 5, B: 3},
	}
	var resultDest SumResult
	var requestID uint64 // To capture the ID for the response

	// Goroutine to simulate server receiving request and sending response
	go func() {
		// 1. Receive request from client
		reqBytes := transport.PopFromClient(t, 2*time.Second)
		require.NotNil(t, reqBytes, "Should have received request from client")

		var receivedReq jsonrpc2.Request
		err := json.Unmarshal(reqBytes, &receivedReq)
		require.NoError(t, err, "Failed to unmarshal request from client")

		assert.Equal(t, "2.0", receivedReq.JSONRPC)
		assert.Equal(t, callArgs.Method, receivedReq.Method)

		// Capture the ID (assuming it's a number that comes as float64)
		idFloat, ok := receivedReq.ID.(float64)
		require.True(t, ok, "Request ID should be a float64 (from JSON number)")
		requestID = uint64(idFloat)
		assert.Greater(t, requestID, uint64(0), "Request ID should be positive")

		// Check params (optional, but good practice)
		var receivedParams SumParams
		paramsBytes, _ := json.Marshal(receivedReq.Params)
		json.Unmarshal(paramsBytes, &receivedParams)
		assert.Equal(t, callArgs.Params.(SumParams).A, receivedParams.A)
		assert.Equal(t, callArgs.Params.(SumParams).B, receivedParams.B)

		// 2. Prepare and send response
		responsePayload := fmt.Sprintf(`{"jsonrpc": "2.0", "id": %d, "result": {"total": %d}}`,
			requestID,
			callArgs.Params.(SumParams).A+callArgs.Params.(SumParams).B,
		)
		err = transport.PushToClient([]byte(responsePayload))
		require.NoError(t, err, "Failed to push response to client")
	}()

	// Make the call
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Call(ctx, callArgs, &resultDest)

	assert.NoError(t, err, "client.Call should succeed")
	assert.Equal(t, 8, resultDest.Total, "Result total should be correct")

	// Give a brief moment for the client.Close() to propagate if needed, though PopFromClient handles timeout
	time.Sleep(10 * time.Millisecond)
}

// Receive implements the jsonrpc2.Transport interface.
// It receives a payload from the ToClient channel, which the test can push to.
func (ft *FakeTransport) Receive(ctx context.Context) ([]byte, error) {
	ft.mu.Lock()
	if ft.closed && len(ft.ToClient) == 0 { // If closed and no pending messages, return error immediately
		ft.mu.Unlock()
		return nil, io.ErrClosedPipe
	}
	receiveErr := ft.receiveErr
	ft.mu.Unlock()

	if receiveErr != nil {
		return nil, receiveErr
	}

	select {
	case payload, ok := <-ft.ToClient:
		if !ok { // Channel closed, means transport is definitively closed
			ft.mu.Lock()
			ft.closed = true // Ensure consistent state
			ft.mu.Unlock()
			return nil, io.ErrClosedPipe
		}
		return payload, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-ft.closeChan: // Explicit close signal
		return nil, io.ErrClosedPipe
	}
}

// Close implements the io.Closer interface.
// It signals that the transport is closed and unblocks any pending Receive calls.
func (ft *FakeTransport) Close() error {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	if ft.closed {
		return nil // Already closed
	}

	ft.closed = true
	close(ft.closeChan) // Signal Receive to stop

	// Closing these channels will cause any active Send/Receive on them to panic if not handled.
	// However, the select statements in Send/Receive with ft.closeChan should handle this.
	// It also signals to any test goroutines ranging over them that they are done.
	// It's generally safe to close channels that are written to by one party and read by another
	// once the writer is done, or in a cleanup like this.
	close(ft.FromClient)
	close(ft.ToClient)

	return nil
}

// PushToClient sends a payload to the client as if it came from the server.
// This is a helper for tests to inject messages into the client's Receive loop.
func (ft *FakeTransport) PushToClient(payload []byte) error {
	ft.mu.Lock()
	if ft.closed {
		ft.mu.Unlock()
		return io.ErrClosedPipe
	}
	ft.mu.Unlock()

	select {
	case ft.ToClient <- payload:
		return nil
	case <-ft.closeChan:
		return io.ErrClosedPipe
	}
}

// PopFromClient retrieves a payload sent by the client, with a timeout.
// This is a helper for tests to get messages the client has sent.
func (ft *FakeTransport) PopFromClient(t *testing.T, timeout time.Duration) []byte {
	t.Helper()
	select {
	case payload, ok := <-ft.FromClient:
		if !ok {
			t.Log("PopFromClient: FromClient channel was closed")
			return nil // Channel closed
		}
		return payload
	case <-time.After(timeout):
		t.Errorf("PopFromClient: timed out after %v waiting for message from client", timeout)
		return nil
	}
}

// SetSendError sets an error to be returned by the next Send call.
func (ft *FakeTransport) SetSendError(err error) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.sendErr = err
}

// SetReceiveError sets an error to be returned by the next Receive call.
func (ft *FakeTransport) SetReceiveError(err error) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.receiveErr = err
}

func TestClient_Call_Success(t *testing.T) {
	transport := NewFakeTransport()
	client := jsonrpc2.NewClient(transport)

	// Start the client's listener in a goroutine
	go func() {
		err := client.Listen()
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.ErrClosedPipe) {
			t.Logf("Client Listen exited with error: %v", err)
			// We might not want to fail the test here if the error is expected during close
		}
	}()
	defer func() {
		err := client.Close() // This will also close the transport
		assert.NoError(t, err, "Client Close should not error")
	}()

	type SumParams struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	type SumResult struct {
		Total int `json:"total"`
	}

	callArgs := jsonrpc2.ClientCallArgs{
		Method: "calculator.sum",
		Params: SumParams{A: 5, B: 3},
	}
	var resultDest SumResult
	var requestID uint64 // To capture the ID for the response

	// Goroutine to simulate server receiving request and sending response
	go func() {
		// 1. Receive request from client
		reqBytes := transport.PopFromClient(t, 2*time.Second)
		require.NotNil(t, reqBytes, "Should have received request from client")

		var receivedReq jsonrpc2.Request
		err := json.Unmarshal(reqBytes, &receivedReq)
		require.NoError(t, err, "Failed to unmarshal request from client")

		assert.Equal(t, "2.0", receivedReq.JSONRPC)
		assert.Equal(t, callArgs.Method, receivedReq.Method)

		// Capture the ID (assuming it's a number that comes as float64)
		idFloat, ok := receivedReq.ID.(float64)
		require.True(t, ok, "Request ID should be a float64 (from JSON number)")
		requestID = uint64(idFloat)
		assert.Greater(t, requestID, uint64(0), "Request ID should be positive")

		// Check params (optional, but good practice)
		var receivedParams SumParams
		paramsBytes, _ := json.Marshal(receivedReq.Params)
		json.Unmarshal(paramsBytes, &receivedParams)
		assert.Equal(t, callArgs.Params.(SumParams).A, receivedParams.A)
		assert.Equal(t, callArgs.Params.(SumParams).B, receivedParams.B)

		// 2. Prepare and send response
		responsePayload := fmt.Sprintf(`{"jsonrpc": "2.0", "id": %d, "result": {"total": %d}}`,
			requestID,
			callArgs.Params.(SumParams).A+callArgs.Params.(SumParams).B,
		)
		err = transport.PushToClient([]byte(responsePayload))
		require.NoError(t, err, "Failed to push response to client")
	}()

	// Make the call
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Call(ctx, callArgs, &resultDest)

	assert.NoError(t, err, "client.Call should succeed")
	assert.Equal(t, 8, resultDest.Total, "Result total should be correct")

	// Give a brief moment for the client.Close() to propagate if needed, though PopFromClient handles timeout
	time.Sleep(10 * time.Millisecond)
}
