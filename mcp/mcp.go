package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings" // Added for NewServer
	"sync"    // Added for request ID generation

	"github.com/dhamidi/smolcode/mcp/jsonrpc2" // Assuming this is the correct path
)

// ToolResultContent defines the structure for content returned by a tool call.
type ToolResultContent struct {
	Type     string `json:"type"`               // "text" or "image"
	Text     string `json:"text,omitempty"`     // non-empty when type == "text"
	Data     string `json:"data,omitempty"`     // non-empty base64 encoded data when type == "image"
	MimeType string `json:"mimeType,omitempty"` // non-empty mime type for type == "image"
}

// Tool defines the structure for a tool's metadata.
type Tool struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	RawInputSchema json.RawMessage `json:"inputSchema"` // raw json bytes of the input schema
}

// Tools is a collection of Tool.
type Tools []Tool

// stdioReadWriteCloser bundles an io.Reader, io.Writer, and io.Closer for stdio pipes.
// It ensures both pipes are closed.
type stdioReadWriteCloser struct {
	io.Reader
	io.Writer
	stdinCloser  io.Closer
	stdoutCloser io.Closer
}

// Close closes both the stdin and stdout closers.
func (s *stdioReadWriteCloser) Close() error {
	err1 := s.stdinCloser.Close()  // Typically pipe writer
	err2 := s.stdoutCloser.Close() // Typically pipe reader
	if err1 != nil {
		return err1
	}
	return err2 // Return second error if first was nil, or first error
}

// Server represents an MCP server process and the client to communicate with it.
type Server struct {
	id      string
	cmdPath string
	cmdArgs []string // Changed from cmd string to cmdPath and cmdArgs

	proc      *exec.Cmd
	rpcClient *jsonrpc2.Client
	closer    io.Closer // To close the subprocess's pipes

	requestIDLock    sync.Mutex // To protect access to requestIDCounter
	requestIDCounter int64      // For generating unique JSON-RPC request IDs
}

// --- Structs for JSON-RPC requests and responses ---

// InitializeParams defines the parameters for the "initialize" request.
type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

// InitializeResult defines the result for the "initialize" response.
// Based on typical JSON-RPC, but mcp/docs.md doesn't specify its structure.
// Assuming it might be an empty object or contain server capabilities.
type InitializeResult struct {
	Capabilities map[string]interface{} `json:"capabilities,omitempty"`
}

// ToolsListParams defines the parameters for the "tools/list" request.
type ToolsListParams struct {
	Cursor string `json:"cursor,omitempty"`
}

// ToolsListResult defines the result for the "tools/list" response.
type ToolsListResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// ToolsCallParams defines the parameters for the "tools/call" request.
type ToolsCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ToolsCallResult defines the result for the "tools/call" response.
type ToolsCallResult struct {
	Content []ToolResultContent `json:"content"`
	IsError bool                `json:"isError"`
}

// NewServer initializes a new Server instance.
// The cmd string is split into a command and its arguments.
func NewServer(id string, cmd string) *Server {
	// Basic command splitting. A more robust solution might be needed for complex cases.
	// For now, let's assume cmd is just the executable name if no args,
	// or space-separated if args exist. This is a simplification.
	// parts := strings.Fields(cmd)
	// cmdPath := parts[0]
	// var cmdArgs []string
	// if len(parts) > 1 {
	// 	cmdArgs = parts[1:]
	// }
	// return &Server{id: id, cmdPath: cmdPath, cmdArgs: cmdArgs, requestIDCounter: 0}
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		// Or return an error: fmt.Errorf("cmd cannot be empty")
		return nil
	}
	cmdPath := parts[0]
	var cmdArgs []string
	if len(parts) > 1 {
		cmdArgs = parts[1:]
	}
	return &Server{
		id:      id,
		cmdPath: cmdPath,
		cmdArgs: cmdArgs,
		// rpcClient, proc, and closer will be set in Start()
		requestIDCounter: 0, // Initialized to 0
	}
}

// Start starts the server subprocess and performs the initialization handshake.
func (s *Server) Start(ctx context.Context) error {
	s.proc = exec.CommandContext(ctx, s.cmdPath, s.cmdArgs...)

	stdin, err := s.proc.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	stdout, err := s.proc.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	rwc := &stdioReadWriteCloser{
		Reader:       stdout,
		Writer:       stdin,
		stdinCloser:  stdin,
		stdoutCloser: stdout,
	}
	s.closer = rwc // Store for later closing in Server.Close()

	transport := NewStdioTransport(rwc)
	s.rpcClient = jsonrpc2.NewClient(transport, s.generateRequestID)

	// Start the server process
	if err := s.proc.Start(); err != nil {
		return fmt.Errorf("failed to start server process: %w", err)
	}

	go func() {
		err := s.rpcClient.Listen()
		if err != nil && err != io.EOF && err != context.Canceled && !strings.Contains(err.Error(), "file already closed") {
			fmt.Fprintf(os.Stderr, "MCP client listener error: %v\n", err)
		}
	}()

	initParams := InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    map[string]interface{}{},
		ClientInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{
			Name:    "smolcode-mcp-client",
			Version: "0.1.0",
		},
	}

	var initResult InitializeResult
	callArgs := jsonrpc2.ClientCallArgs{
		Method: "initialize",
		Params: initParams,
	}
	if err := s.rpcClient.Call(ctx, callArgs, &initResult); err != nil {
		_ = s.Close() // Attempt to clean up if handshake fails, ignore error from Close here
		return fmt.Errorf("jsonrpc call to 'initialize' failed: %w", err)
	}

	notifyArgs := jsonrpc2.ClientNotifyArgs{
		Method: "notifications/initialized",
	}
	if err := s.rpcClient.Notify(ctx, notifyArgs); err != nil {
		_ = s.Close() // Attempt to clean up, ignore error from Close here
		return fmt.Errorf("jsonrpc notify to 'notifications/initialized' failed: %w", err)
	}

	return nil
}

// ListTools sends a "tools/list" request to the server.
func (s *Server) ListTools(ctx context.Context) (Tools, error) {
	listParams := ToolsListParams{ /* Cursor: "" */ } // Empty cursor for the first request
	var listResult ToolsListResult

	callArgs := jsonrpc2.ClientCallArgs{
		Method: "tools/list",
		Params: listParams,
	}

	if err := s.rpcClient.Call(ctx, callArgs, &listResult); err != nil {
		return nil, fmt.Errorf("jsonrpc call to 'tools/list' failed: %w", err)
	}

	// TODO: Handle pagination using listResult.NextCursor if necessary.
	// For now, we return only the first page of tools.
	return listResult.Tools, nil
}

// ByName finds a tool by its name from a list of tools.
func (t Tools) ByName(name string) (Tool, bool) {
	for _, tool := range t {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}

// Call sends a "tools/call" request to the server for the specified tool.
func (s *Server) Call(ctx context.Context, toolName string, params map[string]any) ([]ToolResultContent, error) {
	callPayload := ToolsCallParams{
		Name:      toolName,
		Arguments: params,
	}
	var callResult ToolsCallResult

	callArgs := jsonrpc2.ClientCallArgs{
		Method: "tools/call",
		Params: callPayload,
	}

	if err := s.rpcClient.Call(ctx, callArgs, &callResult); err != nil {
		return nil, fmt.Errorf("jsonrpc call to 'tools/call' (tool: %s) failed: %w", toolName, err)
	}

	if callResult.IsError {
		// mcp/docs.md implies IsError might be true with content that could be an error message.
		// For now, we just return a generic error if IsError is true.
		// A more sophisticated error handling might try to extract details from callResult.Content.
		if len(callResult.Content) > 0 && callResult.Content[0].Type == "text" {
			return callResult.Content, fmt.Errorf("tool call for '%s' failed with server-side error: %s", toolName, callResult.Content[0].Text)
		}
		return callResult.Content, fmt.Errorf("tool call for '%s' failed with server-side error", toolName)
	}

	return callResult.Content, nil
}

// generateRequestID generates a new unique ID for a JSON-RPC request.
func (s *Server) generateRequestID() interface{} {
	s.requestIDLock.Lock()
	defer s.requestIDLock.Unlock()
	s.requestIDCounter++
	return s.requestIDCounter
}

// Close shuts down the server and cleans up resources.
func (s *Server) Close() error {
	var firstErr error

	// 1. Close the jsonrpc2.Client connection.
	// This should also signal the listener goroutine to stop and close the transport.
	if s.rpcClient != nil {
		if err := s.rpcClient.Close(); err != nil {
			firstErr = fmt.Errorf("failed to close rpc client: %w", err)
		}
	}

	// 2. Close the stdio pipes (reader/writer/closer for the transport)
	if s.closer != nil {
		if err := s.closer.Close(); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("failed to close server pipes: %w", err)
			} else {
				fmt.Fprintf(os.Stderr, "Additional error while closing server pipes: %v\n", err)
			}
		}
	}

	// 3. Terminate the server subprocess.
	if s.proc != nil && s.proc.Process != nil {
		// Attempt to signal the process to terminate gracefully.
		if err := s.proc.Process.Signal(os.Interrupt); err != nil {
			// If interrupt fails or is not supported, try to kill.
			if killErr := s.proc.Process.Kill(); killErr != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("failed to kill server process: %w", killErr)
				} else {
					fmt.Fprintf(os.Stderr, "Additional error while killing server process: %v\n", killErr)
				}
			}
		}
		// Wait for the process to exit to release resources.
		_, waitErr := s.proc.Process.Wait()
		// We only care about Wait errors if Signal/Kill didn't already cause an expected error.
		if waitErr != nil && !strings.Contains(waitErr.Error(), "signal: interrupt") && !strings.Contains(waitErr.Error(), "exit status 1") && !strings.Contains(waitErr.Error(), "killed") {
			if firstErr == nil {
				if !strings.Contains(waitErr.Error(), "Wait was already called") {
					firstErr = fmt.Errorf("error waiting for server process to exit: %w", waitErr)
				}
			} else {
				fmt.Fprintf(os.Stderr, "Additional error while waiting for server process: %v\n", waitErr)
			}
		}
	}

	return firstErr
}

// stdioTransport implements jsonrpc2.Transport for stdio.
type stdioTransport struct {
	encoder *json.Encoder
	decoder *json.Decoder
	closer  io.Closer
}

// NewStdioTransport creates a new transport for stdio communication.
func NewStdioTransport(rwc io.ReadWriteCloser) *stdioTransport {
	return &stdioTransport{
		encoder: json.NewEncoder(rwc),
		decoder: json.NewDecoder(rwc), // Corrected: should be rwc, not just r
		closer:  rwc,
	}
}

// Send sends a payload.
func (t *stdioTransport) Send(ctx context.Context, req jsonrpc2.Request) error {
	// The mcp/docs.md example shows `payload []byte` but jsonrpc2.Client.Call uses `jsonrpc2.Request`.
	// We will assume the client marshals the request, and we just encode it here.
	// Or, if client.Call expects Transport to handle full request objects:
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Ensure payload is written atomically if possible, or handle framing (e.g. newline)
		// The json.Encoder handles adding a newline by default.
		return t.encoder.Encode(req)
	}
}

// Receive receives a payload.
func (t *stdioTransport) Receive(ctx context.Context) (jsonrpc2.Response, error) {
	// The mcp/docs.md example shows `[]byte` but jsonrpc2.Client.Listen expects `jsonrpc2.Response`.
	// We will assume the client expects a fully formed Response object.
	var resp jsonrpc2.Response
	// The json.Decoder handles reading one JSON object at a time from the stream.
	// We need to handle context cancellation during blocking Decode.
	// This is tricky as Decode doesn't take a context.
	// A common pattern is to use a separate goroutine for Decode and use a channel,
	// or to make the underlying ReadWriteCloser context-aware (e.g., by closing it).

	errChan := make(chan error, 1)
	respChan := make(chan jsonrpc2.Response, 1)

	go func() {
		var rawResp jsonrpc2.Response // Decode into a temporary to avoid race if Decode hangs and ctx expires
		if err := t.decoder.Decode(&rawResp); err != nil {
			errChan <- err
			return
		}
		respChan <- rawResp
	}()

	select {
	case <-ctx.Done():
		// Attempt to unblock the Decode by closing the underlying connection.
		// This is crucial if the Decode is stuck.
		if t.closer != nil {
			// Best effort, error ignored as we are already in an error path (ctx.Err())
			_ = t.closer.Close()
		}
		return jsonrpc2.Response{}, ctx.Err()
	case err := <-errChan:
		return jsonrpc2.Response{}, err
	case r := <-respChan:
		return r, nil
	}
}

// Close closes the transport.
func (t *stdioTransport) Close() error {
	if t.closer != nil {
		return t.closer.Close()
	}
	return nil
}
