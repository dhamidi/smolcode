# Overview

This is the Model Context Protocol (MCP) package, implementing a client capable of talking to multiple MCP servers.

This package is responsible for:

- discovering and starting locally-running MCP servers in a subprocess,
- querying these servers for tools,
- passing tool call requests to these servers,
- and obtaining the results of tool calls.

The MCP transport supported by this package is JSONRPC-2.0 over stdio.

# Important Concepts and Types

## mcp/jsonrpc2 package

The `mcp/jsonrpc2` package provides a client implementation for the JSON-RPC 2.0 protocol. It is designed to be transport-agnostic, meaning it can be used with various communication mechanisms like standard I/O, HTTP, or WebSockets.

The full JSON-RPC 2.0 specification can be found in `mcp/jsonrpc2/spec.md`.

### Key Features:

*   **Client Implementation**: Provides a `Client` type to manage communication with a JSON-RPC server.
*   **Request/Response Handling**: Supports sending requests (`Call`), receiving responses, and sending notifications (`Notify`).
*   **Server-to-Client Notifications**: Allows registering handlers for notifications sent by the server.
*   **Transport Abstraction**: Uses a `Transport` interface (`Send` and `Receive` methods) for message exchange, allowing flexibility in the underlying communication channel.
*   **Context Aware**: Operations like `Call` and the client's lifecycle are managed using `context.Context`.

### Core Types:

*   `Request`: Represents a JSON-RPC request object.
*   `Response`: Represents a JSON-RPC response object.
*   `ErrorObject`: Represents a JSON-RPC error object.
*   `Client`: The main type for interacting with a JSON-RPC server.
*   `Transport`: An interface that must be implemented to handle the actual sending and receiving of byte payloads over a communication channel.

### Basic Usage:

1.  **Implement a `Transport`**:
    You need to provide an implementation of the `Transport` interface that handles the specifics of your communication channel (e.g., reading from and writing to a `net.Conn` or an `io.ReadWriteCloser` for stdio).

    ```go
    type myStdioTransport struct {
        encoder *json.Encoder
        decoder *json.Decoder
        closer  io.Closer
    }

    func NewMyStdioTransport(rwc io.ReadWriteCloser) *myStdioTransport {
        return &myStdioTransport{
            encoder: json.NewEncoder(rwc), // Assumes messages are newline-separated JSON
            decoder: json.NewDecoder(r),    // Assumes messages are newline-separated JSON
            closer:  rwc,
        }
    }

    func (t *myStdioTransport) Send(ctx context.Context, payload []byte) error {
        // In a real stdio transport, you'd write the payload directly.
        // Here, we assume payload is a fully formed JSON message.
        // For line-based JSON, you might need to marshal an object and then write it.
        // This example is simplified; a robust implementation would handle message framing.
        var req jsonrpc2.Request // Or any struct that json.Marshal can handle
        if err := json.Unmarshal(payload, &req); err != nil {
             // If payload is already marshaled, this unmarshal is not needed.
             // Direct write might be:
             // _, err := t.writer.Write(payload)
             // if err == nil { _, err = t.writer.Write([]byte{'\n'}); } // Add newline
             // return err
            return fmt.Errorf("myStdioTransport.Send: could not unmarshal payload to re-encode: %w", err)
        }
        return t.encoder.Encode(req) // Encodes and adds a newline
    }

    func (t *myStdioTransport) Receive(ctx context.Context) ([]byte, error) {
        // This needs to read one full JSON-RPC message.
        // If using json.Decoder, it handles finding the boundaries of a JSON object.
        var raw json.RawMessage
        if err := t.decoder.Decode(&raw); err != nil {
            return nil, err
        }
        return []byte(raw), nil
    }

    func (t *myStdioTransport) Close() error {
        if t.closer != nil {
            return t.closer.Close()
        }
        return nil
    }
    ```

2.  **Create and Use the Client**:

    ```go
    import (
        "context"
        "fmt"
        "log"
        "os" // For a stdio example

        "your_module/mcp/jsonrpc2" // Adjust path accordingly
    )

    // Example with a hypothetical stdio-based server
    func main() {
        // In a real scenario, rwc would be connected to a server's stdio pipes.
        // For this example, let's assume os.Stdin and os.Stdout are the server.
        // This is a simplified example; a real implementation needs careful pipe management.
        // For instance, using os.Pipe() to create pipes for a child process.
        
        // This is a placeholder for a real ReadWriteCloser connected to a server.
        // For example, if you start a subprocess, these would be its stdin/stdout.
        type stdioReadWriteCloser struct {
            io.Reader
            io.Writer
            io.Closer
        }

        // This example won't run directly without a server on os.Stdin/os.Stdout
        // or a proper ReadWriteCloser.
        // rwc := &stdioReadWriteCloser{Reader: os.Stdin, Writer: os.Stdout, Closer: os.Stdin} // Not a complete example for Closer

        // Create your transport (e.g., using the myStdioTransport defined above)
        // For this example, we'll use a mock transport as creating a real stdio one is complex here.
        // Replace with your actual transport implementation.
        
        // --- Start of section to replace with actual transport ---
        // This mock setup is illustrative.
        // In a real application, `serverReadBuf` and `serverWriteBuf` would be
        // connected to an actual JSON-RPC server (e.g., via stdin/stdout of a subprocess).
        serverReadBuf := new(bytes.Buffer)  // What the client reads from the server
        serverWriteBuf := new(bytes.Buffer) // What the client writes to the server

        mockIo := &mockReadWriteCloser{
            reader: serverReadBuf,
            writer: serverWriteBuf,
            // closeFunc: func() error { return nil }, // Optional close behavior
        }
        transport := jsonrpc2.NewMyStdioTransport(mockIo) // Assuming NewMyStdioTransport exists and is adapted
        // --- End of section to replace with actual transport ---


        client := jsonrpc2.NewClient(transport)

        // Start the client's listener goroutine.
        // This is crucial for processing responses and notifications.
        go func() {
            log.Println("Client listener starting...")
            err := client.Listen()
            if err != nil && err != context.Canceled {
                log.Fatalf("Client listener error: %v", err)
            }
            log.Println("Client listener stopped.")
        }()
        defer client.Close() // Ensure the client is closed when main exits

        // Example: Register a handler for a server notification
        client.OnNotification("server/hello", func(params *json.RawMessage) error {
            if params != nil {
                log.Printf("Received 'server/hello' notification with params: %s", string(*params))
            } else {
                log.Printf("Received 'server/hello' notification with no params")
            }
            return nil
        })

        // Example: Make an RPC call
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        callParams := struct {
            Name string `json:"name"`
        }{Name: "World"}
        
        var result struct {
            Message string `json:"message"`
        }

        // Simulate server responding (for mock transport example)
        go func() {
            // This part simulates the server side for the mock transport.
            // In a real scenario, the server would be a separate process.
            time.Sleep(100 * time.Millisecond) // Give client time to send
            
            // Read what client sent (if needed to construct response)
            // For simplicity, we just craft a response.
            // In a real server, you would parse serverWriteBuf.Bytes()

            // Craft a response to ID 1 (client.Call will use ID 1 for the first call)
            // This ID matching is crucial.
            respBytes, _ := json.Marshal(jsonrpc2.Response{
                JSONRPC: "2.0",
                ID:      1, // Important: Match the client's request ID
                Result:  json.RawMessage(`'''{"message":"Hello, World!"}'''`),
            })
            serverReadBuf.Write(respBytes)
            serverReadBuf.Write([]byte{'\n'}) // If using newline-delimited JSON
        }()


        err := client.Call(ctx, jsonrpc2.ClientCallArgs{
            Method: "myMethod/echo",
            Params: callParams,
        }, &result)

        if err != nil {
            log.Fatalf("RPC Call failed: %v", err)
        }
        log.Printf("RPC Call successful. Result: %s", result.Message)

        // Example: Send a notification
        notifyParams := struct {
            Status string `json:"status"`
        }{Status: "ClientInitialized"}
        
        err = client.Notify(ctx, jsonrpc2.ClientNotifyArgs{
            Method: "client/initialized",
            Params: notifyParams,
        })
        if err != nil {
            log.Fatalf("Notify failed: %v", err)
        }
        log.Println("Notification sent successfully.")
        
        // Keep main running for a bit to allow listener to process potential incoming messages
        time.Sleep(1 * time.Second) 
    }

    // Helper mock for ReadWriteCloser for the example.
    // In a real application, this would be a connection like net.Conn or os.File pipes.
    type mockReadWriteCloser struct {
        reader io.Reader
        writer io.Writer
        closeFunc func() error
    }
    func (m *mockReadWriteCloser) Read(p []byte) (n int, err error) {
        return m.reader.Read(p)
    }
    func (m *mockReadWriteCloser) Write(p []byte) (n int, err error) {
        return m.writer.Write(p)
    }
    func (m *mockReadWriteCloser) Close() error {
        if m.closeFunc != nil {
            return m.closeFunc()
        }
        return nil
    }

    // You would also need to adapt/create NewMyStdioTransport if it's not already defined
    // in your jsonrpc2 package or a utility package.
    // For example:
    // func NewMyStdioTransport(rwc io.ReadWriteCloser) *myStdioTransport { ... }
    // as shown in the transport implementation section.

    ```

    This example demonstrates the basic flow: create a transport, instantiate a client with it, start the listener, and then make calls or send notifications. Remember to replace the mock transport and `myStdioTransport` with your actual transport implementation tailored to your specific communication channel (e.g., stdio pipes for a subprocess, a WebSocket connection, etc.).
    The `client.go` file contains the detailed implementation of these types and methods.


# Initialization Message

The client first needs to send this initialization message:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {},
    "clientInfo": {
      "name": "smolcode",
      "version": "1.0.0"
    }
  }
}
```

The server then proceeds with a response to this message.

After receiving the response, the client then needs to confirm reception of the response with the following notification:

```json
{
  "jsonrpc": "2.0",
  "method": "notifications/initialized"
}
```

# mcp.go - the MCP protocol

This file encapsulates communication with MCP servers.

Here is its public interface:

```go
fetchServer := mcp.NewServer("uvx", "mcp-server-fetch")
err := fetchServer.Start() // starts the subprocess, and takes care of the initialization handshake
tools, err := fetchServer.ListTools() // returns list of tool definitions
fetchTool, found := tools.ByName("fetch")
fetchTool.Name // "fetch"
fetchTool.Description // "Long description of when to use the fetch tool"
fetchTool.RawInputSchema // raw json bytes of the input schema
content, err := fetchServer.Call("fetch", map[string]any{...}) // content is []mcp.ToolResultContent
fetchServer.Close() // shut down the server

type ToolResultContent struct {
  Type string // "text" or "image"
  Text string // non-empty when type == "text"
  Data string // non-empty base64 encoded data when type == "image"
  MimeType string // non-empty mime type for type == "image"
}
```

## Tools

### Listing tools - `tools/list`

Request:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/list",
  "params": {
    "cursor": "optional-cursor-value"
  }
}
```

Response:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "tools": [
      {
        "name": "get_weather",
        "description": "Get current weather information for a location",
        "inputSchema": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "City name or zip code"
            }
          },
          "required": ["location"]
        }
      }
    ],
    "nextCursor": "next-page-cursor"
  }
}
```

### Calling tools - `tools/call`

Request:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "get_weather",
    "arguments": {
      "location": "New York"
    }
  }
}
```

Response:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Current weather in New York:\nTemperature: 72°F\nConditions: Partly cloudy"
      }
    ],
    "isError": false
  }
}
```
