package jsonrpc2

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Request represents a JSON-RPC 2.0 request object.
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      interface{} `json:"id,omitempty"`
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
	// SendRequest sends a pre-formatted JSON-RPC request and returns the server's response.
	// It is the transport's responsibility to handle message framing if necessary (e.g., for stream-based transports).
	SendRequest(ctx context.Context, requestPayload []byte) (responsePayload []byte, err error)
}

// ErrorObject represents a JSON-RPC 2.0 error object.
type ErrorObject struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// FormatRequest creates a JSON-RPC request object and marshals it to JSON.
// The id can be a string, number, or null. If id is nil, it will be omitted (for notifications).
func FormatRequest(method string, params interface{}, id interface{}) ([]byte, error) {
	req := Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}
	return json.Marshal(req)
}

// Client represents a JSON-RPC 2.0 client.
// It manages request IDs and uses a Transport for communication.
type Client struct {
	transport Transport
	nextID    uint64
	mu        sync.Mutex // Protects nextID
}

// NewClient creates a new JSON-RPC client with the given transport.
func NewClient(transport Transport) *Client {
	return &Client{
		transport: transport,
		nextID:    1, // Start with ID 1
	}
}

// Call sends a JSON-RPC request to the server and waits for a response.
// The method is the RPC method name, params is the parameters object (can be nil),
// and result is a pointer where the successful response's result field will be unmarshalled.
func (c *Client) Call(ctx context.Context, method string, params interface{}, result interface{}) error {
	c.mu.Lock()
	currentID := c.nextID
	c.nextID++
	c.mu.Unlock()

	reqBytes, err := FormatRequest(method, params, currentID)
	if err != nil {
		return fmt.Errorf("jsonrpc: failed to format request: %w", err)
	}

	respBytes, err := c.transport.SendRequest(ctx, reqBytes)
	if err != nil {
		return fmt.Errorf("jsonrpc: transport error: %w", err)
	}

	// It's possible for a transport to return no response for a non-notification request
	// if the underlying connection is closed before a response is received, or if the transport
	// is inherently one-way for some reason. A JSON-RPC server should always respond to non-notifications.
	if len(respBytes) == 0 {
		return fmt.Errorf("jsonrpc: received empty response from transport for request ID %v", currentID)
	}

	respID, respResult, respError, parseErr := ParseResponse(respBytes)
	if parseErr != nil {
		return fmt.Errorf("jsonrpc: failed to parse response: %w", parseErr)
	}

	// Ensure respID is comparable with currentID.
	// JSON numbers are unmarshalled as float64.
	var responseID uint64
	switch id := respID.(type) {
	case float64:
		responseID = uint64(id)
	case int:
		responseID = uint64(id)
	case int64:
		responseID = uint64(id)
	case uint64:
		responseID = id
	default:
		return fmt.Errorf("jsonrpc: response ID type mismatch (expected numeric, got %T for value %v)", respID, respID)
	}

	if responseID != currentID {
		return fmt.Errorf("jsonrpc: response ID mismatch (expected %v, got %v)", currentID, responseID)
	}

	if respError != nil {
		return fmt.Errorf("jsonrpc: server error (%d): %s", respError.Code, respError.Message)
	}

	if respResult == nil && result != nil {
		return nil
	}

	if result != nil && respResult != nil {
		err = json.Unmarshal(*respResult, result)
		if err != nil {
			return fmt.Errorf("jsonrpc: failed to unmarshal result: %w", err)
		}
	}

	return nil
}

// Notify sends a JSON-RPC notification (a request without an ID).
// It does not wait for a response from the server.
func (c *Client) Notify(ctx context.Context, method string, params interface{}) error {
	reqBytes, err := FormatRequest(method, params, nil)
	if err != nil {
		return fmt.Errorf("jsonrpc: failed to format notification: %w", err)
	}

	_, err = c.transport.SendRequest(ctx, reqBytes)
	if err != nil {
		return fmt.Errorf("jsonrpc: transport error during notify: %w", err)
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
	return resp.ID, resp.Result, resp.Error, nil
}
