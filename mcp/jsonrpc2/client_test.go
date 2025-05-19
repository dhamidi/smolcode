package jsonrpc2

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestClientConcurrentCalls(t *testing.T) {
	input := &bytes.Buffer{}
	output := &bytes.Buffer{}

	client := NewClient(io.MultiReader(input, output))

	// Simulate server responses
	go func() {
		time.Sleep(100 * time.Millisecond)
		resp1 := Response{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`"result1"`),
			ID:      "1",
		}
		json.NewEncoder(output).Encode(resp1)

		time.Sleep(100 * time.Millisecond)
		resp2 := Response{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`"result2"`),
			ID:      "2",
		}
		json.NewEncoder(output).Encode(resp2)
	}()

	ctx := context.Background()
	var result1, result2 string

	// Make concurrent calls
	go func() {
		err := client.Call(ctx, "method1", nil, &result1)
		assert.NoError(t, err)
		assert.Equal(t, "result1", result1)
	}()

	go func() {
		err := client.Call(ctx, "method2", nil, &result2)
		assert.NoError(t, err)
		assert.Equal(t, "result2", result2)
	}()

	// Wait for both calls to complete
	time.Sleep(300 * time.Millisecond)
}

func TestClientReadError(t *testing.T) {
	input := &bytes.Buffer{}
	output := &bytes.Buffer{}

	client := NewClient(io.MultiReader(input, output))

	// Simulate server error response
	go func() {
		time.Sleep(100 * time.Millisecond)
		resp := Response{
			JSONRPC: "2.0",
			Error: &ErrorObject{
				Code:    -32601,
				Message: "Method not found",
			},
			ID: "1",
		}
		json.NewEncoder(output).Encode(resp)
	}()

	ctx := context.Background()
	var result string

	// Make a call that should return an error
	err := client.Call(ctx, "nonexistentmethod", nil, &result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Method not found")
}

func TestClientConcurrentErrors(t *testing.T) {
	input := &bytes.Buffer{}
	output := &bytes.Buffer{}

	client := NewClient(io.MultiReader(input, output))

	// Simulate server error responses
	go func() {
		time.Sleep(100 * time.Millisecond)
		resp1 := Response{
			JSONRPC: "2.0",
			Error: &ErrorObject{
				Code:    -32601,
				Message: "Method not found",
			},
			ID: "1",
		}
		json.NewEncoder(output).Encode(resp1)

		time.Sleep(100 * time.Millisecond)
		resp2 := Response{
			JSONRPC: "2.0",
			Error: &ErrorObject{
				Code:    -32601,
				Message: "Method not found",
			},
			ID: "2",
		}
		json.NewEncoder(output).Encode(resp2)
	}()

	ctx := context.Background()
	var result1, result2 string

	// Make concurrent calls that should return errors
	go func() {
		err := client.Call(ctx, "nonexistentmethod1", nil, &result1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Method not found")
	}()

	go func() {
		err := client.Call(ctx, "nonexistentmethod2", nil, &result2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Method not found")
	}()

	// Wait for both calls to complete
	time.Sleep(300 * time.Millisecond)
}

func TestClientClose(t *testing.T) {
	input := &bytes.Buffer{}
	output := &bytes.Buffer{}

	client := NewClient(io.MultiReader(input, output))

	// Close the client
	err := client.Close()
	assert.NoError(t, err)

	// Ensure further calls fail
	ctx := context.Background()
	var result string
	err = client.Call(ctx, "method", nil, &result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection closed")
}
