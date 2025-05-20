package jsonrpc2

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Request represents a JSON-RPC 2.0 request object.
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      interface{} `json:"id,omitempty"`
}

// RequestArgs encapsulates the arguments for FormatRequest.
type RequestArgs struct {
	Method string
	Params interface{}
	ID     interface{}
}

// IncomingMessage is used to initially unmarshal any incoming JSON-RPC message
// to determine if it's a request, response, or notification for dispatching.
type IncomingMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	Method  string           `json:"method,omitempty"` // Present in requests/notifications
	Params  *json.RawMessage `json:"params,omitempty"` // Present in requests/notifications
	ID      interface{}      `json:"id,omitempty"`     // Present in requests and responses (even if null for some responses)
	Result  *json.RawMessage `json:"result,omitempty"` // Present in successful responses
	Error   *ErrorObject     `json:"error,omitempty"`  // Present in error responses
}

// Response represents a JSON-RPC 2.0 response object.
type Response struct {
	JSONRPC string           `json:"jsonrpc"`
	Result  *json.RawMessage `json:"result,omitempty"`
	Error   *ErrorObject     `json:"error,omitempty"`
	ID      interface{}      `json:"id"`
}

// Transport defines the interface for sending and receiving JSON-RPC messages.
// This allows for different communication mechanisms (e.g., HTTP, WebSockets, net.Conn) to be used.
type Transport interface {
	// Send sends a pre-formatted JSON-RPC message payload.
	Send(ctx context.Context, payload []byte) error
	// Receive waits for and returns the next JSON-RPC message payload from the underlying connection.
	// It is the transport's responsibility to handle message framing if necessary.
	Receive(ctx context.Context) (payload []byte, err error)
}

// ErrorObject represents a JSON-RPC 2.0 error object.
type ErrorObject struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// FormatRequest creates a JSON-RPC request object and marshals it to JSON.
func FormatRequest(args RequestArgs) ([]byte, error) {
	req := Request{
		JSONRPC: "2.0",
		Method:  args.Method,
		Params:  args.Params,
		ID:      args.ID,
	}
	return json.Marshal(req)
}

// Client represents a JSON-RPC 2.0 client.
// It manages request IDs and uses a Transport for communication.
type Client struct {
	transport Transport
	nextID    uint64
	idMu      sync.Mutex // Protects nextID

	// For handling server-to-client notifications
	notificationHandlers   map[string]func(params *json.RawMessage) error
	notificationHandlersMu sync.Mutex

	// For correlating responses to client-initiated calls
	pendingCalls   map[interface{}]chan *Response // key is request ID
	pendingCallsMu sync.Mutex

	// Lifecycle management for the listener goroutine
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ClientCallArgs encapsulates the arguments for the Client.Call method,
// excluding the context and result destination.
type ClientCallArgs struct {
	Method string
	Params interface{}
}

// ClientNotifyArgs encapsulates the arguments for the Client.Notify method,
// excluding the context.
type ClientNotifyArgs struct {
	Method string
	Params interface{}
}

// NewClient creates a new JSON-RPC client with the given transport.
// The client will not start listening for messages until its Listen method is called.
func NewClient(transport Transport) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		transport: transport,
		nextID:    1, // Start with ID 1
		// idMu is zero-value sync.Mutex, which is fine

		notificationHandlers: make(map[string]func(params *json.RawMessage) error),
		// notificationHandlersMu is zero-value sync.Mutex

		pendingCalls: make(map[interface{}]chan *Response),
		// pendingCallsMu is zero-value sync.Mutex

		ctx:    ctx,
		cancel: cancel,
		// wg is zero-value sync.WaitGroup
	}
}

// OnNotification registers a handler function for a given server notification method.
// If a handler already exists for the method, it will be overwritten.
// The handler receives the parameters of the notification and should return an error
// if processing fails (though this error is typically only for logging by the client listener).
func (c *Client) OnNotification(method string, handler func(params *json.RawMessage) error) {
	c.notificationHandlersMu.Lock()
	defer c.notificationHandlersMu.Unlock()
	c.notificationHandlers[method] = handler
}

// Call sends a JSON-RPC request to the server and waits for a response.
// args contains the method and parameters for the call.
// resultDest is a pointer where the successful response's result field will be unmarshalled.
func (c *Client) Call(ctx context.Context, args ClientCallArgs, resultDest interface{}) error {
	c.idMu.Lock()
	currentID := c.nextID
	c.nextID++
	c.idMu.Unlock()

	reqBytes, err := FormatRequest(RequestArgs{Method: args.Method, Params: args.Params, ID: currentID})
	if err != nil {
		return fmt.Errorf("jsonrpc: failed to format request: %w", err)
	}

	// Create a channel to receive the response
	respChan := make(chan *Response, 1) // Buffered channel of size 1

	c.pendingCallsMu.Lock()
	// Check if client is closing or has closed
	select {
	case <-c.ctx.Done():
		c.pendingCallsMu.Unlock()
		return fmt.Errorf("jsonrpc: client is closed: %w", c.ctx.Err())
	default:
		// Proceed if client context is not done
	}
	c.pendingCalls[currentID] = respChan
	c.pendingCallsMu.Unlock()

	// Ensure the pending call is cleaned up if Call returns before response is received or processed
	defer func() {
		c.pendingCallsMu.Lock()
		delete(c.pendingCalls, currentID)
		c.pendingCallsMu.Unlock()
	}()

	// Send the request
	if err := c.transport.Send(ctx, reqBytes); err != nil {
		return fmt.Errorf("jsonrpc: transport failed to send request: %w", err)
	}

	// Wait for the response or context cancellation
	select {
	case <-ctx.Done(): // Context for this specific call
		return fmt.Errorf("jsonrpc: call timed out or was cancelled: %w", ctx.Err())
	case <-c.ctx.Done(): // Client's main context, indicates listener might be shutting down
		return fmt.Errorf("jsonrpc: client is closing: %w", c.ctx.Err())
	case resp := <-respChan:
		if resp == nil {
			// This might happen if the listen loop closes the channel during shutdown without sending a response
			return fmt.Errorf("jsonrpc: call for ID %v aborted due to client shutdown or an issue in listener", currentID)
		}
		// We have a response (which could be an error response from the server)
		if resp.Error != nil {
			return fmt.Errorf("jsonrpc: server error (code: %d): %s", resp.Error.Code, resp.Error.Message)
		}

		// Successful response
		if resp.Result == nil && resultDest != nil {
			// Server sent back a success response but with a null/omitted result field.
			// If resultDest is non-nil, the caller expects a value.
			// We don't error here; resultDest will remain in its zero state or unchanged.
			return nil
		}
		if resultDest != nil && resp.Result != nil {
			if err := json.Unmarshal(*resp.Result, resultDest); err != nil {
				return fmt.Errorf("jsonrpc: failed to unmarshal result: %w", err)
			}
		}
		return nil
	}
}

// Notify sends a JSON-RPC notification (a request without an ID).
// args contains the method and parameters for the notification.
// It does not wait for a response from the server.
func (c *Client) Notify(ctx context.Context, args ClientNotifyArgs) error {
	// ID is nil for notifications, FormatRequest handles omitempty for ID
	reqBytes, err := FormatRequest(RequestArgs{Method: args.Method, Params: args.Params, ID: nil})
	if err != nil {
		return fmt.Errorf("jsonrpc: failed to format notification: %w", err)
	}

	// For Notify, we send the request but don't expect a response payload in the typical RPC sense.
	// The transport might return an error if sending fails (e.g., connection closed).
	// It might also return a payload if the transport is, for example, HTTP and it gives an HTTP status response.
	// However, per JSON-RPC, no response is sent for notifications. So we ignore responsePayload.
	err = c.transport.Send(ctx, reqBytes)
	if err != nil {
		return fmt.Errorf("jsonrpc: transport error during notify: %w", err)
	}

	return nil
}

// Listen starts the client's listening goroutine.
// This method will block until the client's context is canceled (e.g., by calling Close)
// or an unrecoverable error occurs in the transport's Receive method.
// All server-to-client notifications and responses to client calls are processed here.
func (c *Client) Listen() error {
	c.wg.Add(1)
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done(): // Client is closing
			c.cleanupPendingCalls(c.ctx.Err())
			return c.ctx.Err()
		default:
			// Non-blocking check for client context before blocking on Receive
		}

		payload, err := c.transport.Receive(c.ctx) // Pass client's main context
		if err != nil {
			// If the error is due to client context being canceled, it's an expected shutdown.
			if c.ctx.Err() != nil && (err == context.Canceled || err == context.DeadlineExceeded || err.Error() == "context canceled") {
				c.cleanupPendingCalls(c.ctx.Err())
				return c.ctx.Err()
			}
			// Otherwise, it's an unexpected transport error.
			fmt.Printf("jsonrpc: error receiving message from transport: %v\n", err) // Basic logging
			c.cleanupPendingCalls(err)                                               // Notify pending calls about the error
			return fmt.Errorf("jsonrpc: transport receive error: %w", err)
		}

		if len(payload) == 0 { // Should not happen with a well-behaved transport unless connection closed cleanly
			continue
		}

		var incomingMsg IncomingMessage
		if err := json.Unmarshal(payload, &incomingMsg); err != nil {
			fmt.Printf("jsonrpc: error unmarshalling incoming message: %v: %s\n", err, string(payload))
			continue
		}

		// Dispatch the message
		if incomingMsg.Method != "" { // It's a request or notification from server
			c.notificationHandlersMu.Lock()
			handler, ok := c.notificationHandlers[incomingMsg.Method]
			c.notificationHandlersMu.Unlock()

			if ok {
				go func(p *json.RawMessage) {
					if hErr := handler(p); hErr != nil {
						fmt.Printf("jsonrpc: notification handler for method '%s' failed: %v\n", incomingMsg.Method, hErr)
					}
				}(incomingMsg.Params)
			} else {
				fmt.Printf("jsonrpc: no handler for notification method '%s'\n", incomingMsg.Method)
			}
		} else if incomingMsg.ID != nil { // It's a response to a client call
			if incomingMsg.Error != nil && incomingMsg.Result != nil {
				fmt.Printf("jsonrpc: received response with ID %v that has both result and error fields\n", incomingMsg.ID)
				continue // Invalid response, skip
			}
			if incomingMsg.Error == nil && incomingMsg.Result == nil && incomingMsg.JSONRPC == "2.0" { // ID is present, JSONRPC is present, but no result/error
				fmt.Printf("jsonrpc: received response with ID %v that has neither result nor error field\n", incomingMsg.ID)
				continue // Invalid response, skip
			}

			var mapKey interface{}
			switch idVal := incomingMsg.ID.(type) {
			case float64: // JSON numbers are float64
				mapKey = uint64(idVal)
			case string:
				mapKey = idVal // If IDs were strings
			default:
				mapKey = incomingMsg.ID // Use as is, assuming consistent types or Call side handles it
			}

			c.pendingCallsMu.Lock()
			ch, ok := c.pendingCalls[mapKey]
			c.pendingCallsMu.Unlock()

			if ok && ch != nil {
				responseForCall := &Response{
					JSONRPC: incomingMsg.JSONRPC,
					Result:  incomingMsg.Result,
					Error:   incomingMsg.Error,
					ID:      incomingMsg.ID,
				}
				select {
				case ch <- responseForCall:
				case <-c.ctx.Done():
				}
			} else {
				fmt.Printf("jsonrpc: received response for unknown or already handled ID: %v\n", mapKey)
			}
		} else {
			fmt.Printf("jsonrpc: received ill-formed message (no method and no/null ID for dispatch): %s\n", string(payload))
		}
	}
}

// cleanupPendingCalls is called when the listener is shutting down to error out any pending calls.
func (c *Client) cleanupPendingCalls(errReason error) {
	c.pendingCallsMu.Lock()
	defer c.pendingCallsMu.Unlock()
	for id, ch := range c.pendingCalls {
		if ch != nil {
			// Attempt to send a nil to unblock Call; it will then see client context is done.
			// This helps Call return with a client closing error rather than just its own timeout.
			// If Call has already returned/timed out, this send won't block due to default case.
			select {
			case ch <- nil:
			default:
			}
			close(ch)
		}
		delete(c.pendingCalls, id)
	}
}

// Close shuts down the client's listener goroutine and cleans up resources.
// It cancels the client's main context, which signals the listener to stop.
// Close waits for the listener goroutine to complete.
// If the underlying transport implements io.Closer, its Close method is also called.
func (c *Client) Close() error {
	c.cancel()  // Signal the listener goroutine to stop via context cancellation
	c.wg.Wait() // Wait for the listener goroutine to finish

	// At this point, the listen loop has exited and called cleanupPendingCalls.
	// cleanupPendingCalls should have closed channels and cleared the map.
	// For safety, ensure map is clear if cleanupPendingCalls had issues or missed something,
	// though ideally it should handle everything.
	c.pendingCallsMu.Lock()
	for id, ch := range c.pendingCalls {
		if ch != nil {
			close(ch)
		}
		delete(c.pendingCalls, id)
	}
	c.pendingCalls = make(map[interface{}]chan *Response) // Re-initialize to be safe
	c.pendingCallsMu.Unlock()

	// If the transport supports closing, close it.
	if closer, ok := c.transport.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			// Log this error, but don't let it prevent other cleanup or shadow client context errors.
			fmt.Printf("jsonrpc: error closing transport: %v\n", err)
			// Decide if this should be returned. Often, primary interest is if client loop shutdown cleanly.
			// For now, return it if it's the only error.
			return fmt.Errorf("jsonrpc: error closing transport: %w", err)
		}
	}
	return nil
}

// ParseResponse unmarshals a JSON response and separates the id, result (as json.RawMessage), and error fields.
func ParseResponse(jsonResponse []byte) (id interface{}, result *json.RawMessage, errResp *ErrorObject, parseErr error) {
	var resp Response
	parseErr = json.Unmarshal(jsonResponse, &resp)
	if parseErr != nil {
		return nil, nil, nil, parseErr
	}
	if resp.JSONRPC != "" && resp.JSONRPC != "2.0" {
		return resp.ID, nil, &ErrorObject{Code: -32600, Message: "Invalid JSON-RPC version"}, nil
	}

	// Validate mutual exclusivity of result and error
	if resp.Error != nil && resp.Result != nil {
		return resp.ID, nil, nil, fmt.Errorf("jsonrpc: response contains both result and error fields")
	}
	if resp.Error == nil && resp.Result == nil {
		// A valid success response must have a "result" field (even if null), and an error response must have an "error" field.
		return resp.ID, nil, nil, fmt.Errorf("jsonrpc: response contains neither result nor error field")
	}

	return resp.ID, resp.Result, resp.Error, nil
}
