package jsonrpc2

import "encoding/json"

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

// ParseResponse unmarshals a JSON response and separates the id, result (as json.RawMessage), and error fields.
func ParseResponse(jsonResponse []byte) (id interface{}, result *json.RawMessage, errResp *ErrorObject, parseErr error) {
	var resp Response
	parseErr = json.Unmarshal(jsonResponse, &resp)
	if parseErr != nil {
		return nil, nil, nil, parseErr
	}
	// Note: The JSON-RPC 2.0 spec says the id field in a response MUST match the id field in the request,
	// or be null if there was an error parsing the request id (which this client-side parser doesn't deal with directly).
	// It's also possible for id to be null for notifications that illicit an error.
	// The jsonrpc field is optional in responses according to some interpretations, but we check if present.
	if resp.JSONRPC != "" && resp.JSONRPC != "2.0" {
		// This is a stricter check than the spec absolutely requires for responses, but good practice.
		return resp.ID, nil, &ErrorObject{Code: -32600, Message: "Invalid JSON-RPC version"}, nil
	}
	return resp.ID, resp.Result, resp.Error, nil
}
