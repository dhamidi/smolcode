# Overview

This is the Model Context Protocol (MCP) package, implementing a client capable of talking to multiple MCP servers.

This package is responsible for:

- discovering and starting locally-running MCP servers in a subprocess,
- querying these servers for tools,
- passing tool call requests to these servers,
- and obtaining the results of tool calls.

The MCP transport supported by this package is JSONRPC-2.0 over stdio.

# Important Concepts and Types

## jsonrpc.go

This file contains the subset of the JSONRPC-2.0 protocol that this package implements.

The full specification can be found here: <https://www.jsonrpc.org/specification>

```go
// see the output of `go doc -all -u -src net/rpc/jsonrpc.clientCodec` for inspiration
type JSONRPC2ClientCodec struct {
}

type JSONRPC2Request struct {
  JSONRPC string        `json:"jsonrpc"`       // must be "2.0"
  Method string         `json:"method"`        // the method to invoke on the server
  Params map[string]any `json:"params"`        // params used for invoking the method
  ID     *int            `json:"id,omitempty"` // optional ID (missing = notification)
}

type JSONRPC2Response struct {
  JSONRPC string           `json:"jsonrpc"`         // must be "2.0"
  Result  *json.RawMessage `json:"result"`          // the result of calling the method
  Error   map[string]any   `json:"error,omitempty"` // optional error from
}
var _ (rpc.ClientCodec) = &JSONRPC2ClientCodec{}
```

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
