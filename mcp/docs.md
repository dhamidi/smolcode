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

The full specification can be found here: https://www.jsonrpc.org/specification

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
