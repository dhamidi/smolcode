# Overview

Reference: ./spec.md - the JSON-RPC 2.0 spec.

This document details the public interface for the `jsonrpc2` package.

Here is an example of communicating with a server:

```go
package main

conn := jsonrpc2.Connect(readFromServer, writeToServer)
defer conn.Close()
ctx := context.Background()
params := &InitializeParams{}
reply  := &InitializeReply{}
if err := conn.Call(ctx, "initialize", params, &reply); err != nil {
  if protocolError, ok := err.(*jsonrpc2.Error); ok {
    // handle protocol error
  } else {
    // handle IO error
  }
}

if err := conn.Notify(ctx, "initialized", &InitializedParams{}); err != nil {
  // handle IO error
}

notifications := conn.Subscribe() // returns a <-chan *jsonrpc2.Notification
for notification := range notifications {
  // notification.Method is a string
  // notification.Params is a json.RawMessage
  if notification.Method == "notifications/initialized" {
    dest := &InitializedParams{}
    json.Unmarshal(notification.Params, dest)
  }
}
```

# Implementation Details

The call to `jsonrpc2.Connect` creates a `*jsonrpc2.Connection`, which starts two go routines internally:

- one constantly decoding the readFromServer io.Reader,
- and one writing to writeToServer

Calling `conn.Call` assigns an ID to the request, assembles a matching \*jsonprc2.Message by serializing params, and sends it down a channel consumed by the writer Go routine.
