package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"sync"
)

// JSONRPC2Request represents a JSON-RPC 2.0 request object.
// As per net/rpc, the Params field is a single value (or a struct treated as one).
// The ID is uint64 to match rpc.Request.Seq.
type JSONRPC2Request struct {
	JSONRPC string `json:"jsonrpc"` // must be "2.0"
	Method  string `json:"method"`  // the method to invoke on the server
	Params  any    `json:"params"`  // params used for invoking the method
	ID      uint64 `json:"id"`      // request ID
}

// JSONRPC2Response represents a JSON-RPC 2.0 response object.
// The ID is uint64 to match rpc.Response.Seq.
type JSONRPC2Response struct {
	JSONRPC string           `json:"jsonrpc"`          // must be "2.0"
	Result  *json.RawMessage `json:"result,omitempty"` // the result of calling the method
	Error   any              `json:"error,omitempty"`  // error object if an error occurred
	ID      uint64           `json:"id"`               // response ID, should match request ID
}

// JSONRPC2ClientCodec implements the rpc.ClientCodec interface for JSON-RPC 2.0.
type JSONRPC2ClientCodec struct {
	dec *json.Decoder // for reading JSON responses
	enc *json.Encoder // for writing JSON requests
	c   io.Closer     // to close the underlying connection

	reqMutex        sync.Mutex // Protects seq and pendingRequests
	seq             uint64
	pendingRequests map[uint64]string // Stores method for rpc.Response

	// bodyMutex protects lastResultForBody
	bodyMutex         sync.Mutex
	lastResultForBody *json.RawMessage
}

var _ rpc.ClientCodec = (*JSONRPC2ClientCodec)(nil)

// NewJSONRPC2ClientCodec returns a new rpc.ClientCodec using JSON-RPC 2.0 on conn.
func NewJSONRPC2ClientCodec(conn io.ReadWriteCloser) rpc.ClientCodec {
	return &JSONRPC2ClientCodec{
		dec:             json.NewDecoder(conn),
		enc:             json.NewEncoder(conn),
		c:               conn,
		pendingRequests: make(map[uint64]string),
		// lastResultForBody is implicitly nil and bodyMutex is zero-valued
	}
}

// WriteRequest writes a JSON-RPC request to the connection.
func (codec *JSONRPC2ClientCodec) WriteRequest(req *rpc.Request, params any) error {
	codec.reqMutex.Lock()
	codec.seq++
	id := codec.seq
	codec.pendingRequests[id] = req.ServiceMethod
	codec.reqMutex.Unlock()

	jReq := &JSONRPC2Request{
		JSONRPC: "2.0",
		Method:  req.ServiceMethod,
		Params:  params,
		ID:      id,
	}
	return codec.enc.Encode(jReq)
}

// jsonError represents a generic JSON-RPC error structure.
type jsonError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *jsonError) Error() string {
	return fmt.Sprintf("rpc: server error %d: %s", e.Code, e.Message)
}

// ReadResponseHeader reads the JSON-RPC response header.
// The header is the entire response object in JSON-RPC.
func (codec *JSONRPC2ClientCodec) ReadResponseHeader(resp *rpc.Response) error {
	var jResp JSONRPC2Response
	if err := codec.dec.Decode(&jResp); err != nil {
		return err
	}

	codec.reqMutex.Lock()
	resp.Seq = jResp.ID
	resp.ServiceMethod = codec.pendingRequests[jResp.ID]
	delete(codec.pendingRequests, jResp.ID)
	codec.reqMutex.Unlock()

	resp.Error = ""
	if jResp.Error != nil {
		errBytes, err := json.Marshal(jResp.Error)
		if err != nil {
			resp.Error = "rpc: failed to parse error object from server"
			return nil
		}
		var serverError jsonError
		if err := json.Unmarshal(errBytes, &serverError); err != nil {
			var strError string
			if umErr := json.Unmarshal(errBytes, &strError); umErr == nil {
				resp.Error = strError
				return nil
			}
			resp.Error = "rpc: failed to unmarshal error object from server"
			return nil
		}
		resp.Error = serverError.Error()
	}

	codec.bodyMutex.Lock() // New: Lock bodyMutex
	if jResp.Result != nil && jResp.Error == nil {
		codec.lastResultForBody = jResp.Result
	} else {
		codec.lastResultForBody = nil // Ensure it's cleared if no result or if error
	}
	codec.bodyMutex.Unlock() // New: Unlock bodyMutex

	// This condition needs to be outside the lock and after error processing
	if jResp.Result == nil && jResp.Error == nil {
		return fmt.Errorf("rpc: server_response: invalid response: missing result and error for id %d", jResp.ID)
	}

	return nil
}

// ReadResponseBody unmarshals the result from the response into the body.
// It uses the lastResultForBody field set by the preceding ReadResponseHeader call.
func (codec *JSONRPC2ClientCodec) ReadResponseBody(body any) error {
	codec.bodyMutex.Lock()
	resultToUse := codec.lastResultForBody
	codec.lastResultForBody = nil // Consume it, ensuring it's used only once
	codec.bodyMutex.Unlock()

	if body == nil { // If rpc.Client Call/Go passes a nil reply value.
		return nil
	}
	if resultToUse == nil { // No result was stored (e.g., error in response, or malformed response)
		// This can happen if ReadResponseHeader encountered an error or no result.
		// rpc.Client should not call ReadResponseBody if ReadResponseHeader returned an error
		// or if resp.Error was set. If it does, and resultToUse is nil,
		// unmarshalling nil into body would likely panic or fail.
		// Returning nil is safe as body remains unchanged.
		return nil
	}
	return json.Unmarshal(*resultToUse, body)
}

// Close closes the underlying connection.
func (codec *JSONRPC2ClientCodec) Close() error {
	return codec.c.Close()
}

// Dial connects to a JSON-RPC server at the specified network address.
func Dial(network, address string) (*rpc.Client, error) {
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	codec := NewJSONRPC2ClientCodec(conn)
	return rpc.NewClientWithCodec(codec), nil
}
