package jsonrpc2

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCallSuccess(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	go func() {
		req := &Request{
			JSONRPC: "2.0",
			Method:  "testMethod",
			Params:  map[string]interface{}{"param1": "value1"},
			ID:      "1",
		}
		resp := &Response{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`{"result": "success"}`),
			ID:      "1",
		}
		json.NewEncoder(writer).Encode(req)
		json.NewEncoder(writer).Encode(resp)
	}()

	var result map[string]string
	err := c.Call(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"}, &result)
	assert.NoError(t, err)
	assert.Equal(t, "success", result["result"])
}

func TestCallJSONRPCError(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	go func() {
		req := &Request{
			JSONRPC: "2.0",
			Method:  "testMethod",
			Params:  map[string]interface{}{"param1": "value1"},
			ID:      "1",
		}
		resp := &Response{
			JSONRPC: "2.0",
			Error:   &Error{Code: -32601, Message: "Method not found"},
			ID:      "1",
		}
		json.NewEncoder(writer).Encode(req)
		json.NewEncoder(writer).Encode(resp)
	}()

	var result map[string]string
	err := c.Call(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"}, &result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Method not found")
}

func TestCallContextCancelled(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var result map[string]string
	err := c.Call(ctx, "testMethod", map[string]interface{}{"param1": "value1"}, &result)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestCallConnectionClosed(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	writer.Close()

	var result map[string]string
	err := c.Call(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"}, &result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection closed by remote")
}

func TestNotifySuccess(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	go func() {
		req := &Request{
			JSONRPC: "2.0",
			Method:  "testMethod",
			Params:  map[string]interface{}{"param1": "value1"},
		}
		json.NewEncoder(writer).Encode(req)
	}()

	err := c.Notify(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"})
	assert.NoError(t, err)
}

func TestNotifyConnectionClosed(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	writer.Close()

	err := c.Notify(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection closed by remote")
}

func TestSubscribe(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	subChan := c.Subscribe()

	go func() {
		notification := &Notification{
			Method: "testMethod",
			Params: json.RawMessage(`{"param1": "value1"}`),
		}
		json.NewEncoder(writer).Encode(notification)
	}()

	select {
	case notification := <-subChan:
		assert.Equal(t, "testMethod", notification.Method)
		assert.Equal(t, `{"param1": "value1"}`, string(notification.Params))
	case <-time.After(1 * time.Second):
		t.Errorf("did not receive notification in time")
	}
}

func TestClose(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	err := c.Close()
	assert.NoError(t, err)
	assert.Error(t, c.Err())
}

func TestCloseMultipleCalls(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	err := c.Close()
	assert.NoError(t, err)
	err = c.Close()
	assert.NoError(t, err)
}

func TestCallAfterClose(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	err := c.Close()
	assert.NoError(t, err)

	var result map[string]string
	err = c.Call(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"}, &result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client is closing")
}

func TestNotifyAfterClose(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	err := c.Close()
	assert.NoError(t, err)

	err = c.Notify(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client is closing")
}

func TestIDTypes(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	go func() {
		req := &Request{
			JSONRPC: "2.0",
			Method:  "testMethod",
			Params:  map[string]interface{}{"param1": "value1"},
			ID:      "1",
		}
		resp := &Response{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`{"result": "success"}`),
			ID:      "1",
		}
		json.NewEncoder(writer).Encode(req)
		json.NewEncoder(writer).Encode(resp)
	}()

	var result map[string]string
	err := c.Call(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"}, &result)
	assert.NoError(t, err)
	assert.Equal(t, "success", result["result"])

	go func() {
		req := &Request{
			JSONRPC: "2.0",
			Method:  "testMethod",
			Params:  map[string]interface{}{"param1": "value1"},
			ID:      1,
		}
		resp := &Response{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`{"result": "success"}`),
			ID:      1,
		}
		json.NewEncoder(writer).Encode(req)
		json.NewEncoder(writer).Encode(resp)
	}()

	err = c.Call(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"}, &result)
	assert.NoError(t, err)
	assert.Equal(t, "success", result["result"])
}

func TestSeparateReadWriteCloser(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	go func() {
		req := &Request{
			JSONRPC: "2.0",
			Method:  "testMethod",
			Params:  map[string]interface{}{"param1": "value1"},
			ID:      "1",
		}
		resp := &Response{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`{"result": "success"}`),
			ID:      "1",
		}
		json.NewEncoder(writer).Encode(req)
		json.NewEncoder(writer).Encode(resp)
	}()

	var result map[string]string
	err := c.Call(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"}, &result)
	assert.NoError(t, err)
	assert.Equal(t, "success", result["result"])
}

func TestSameReadWriteCloser(t *testing.T) {
	conn := &net.Conn{}
	c := Connect(conn, conn)

	go func() {
		req := &Request{
			JSONRPC: "2.0",
			Method:  "testMethod",
			Params:  map[string]interface{}{"param1": "value1"},
			ID:      "1",
		}
		resp := &Response{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`{"result": "success"}`),
			ID:      "1",
		}
		json.NewEncoder(conn).Encode(req)
		json.NewEncoder(conn).Encode(resp)
	}()

	var result map[string]string
	err := c.Call(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"}, &result)
	assert.NoError(t, err)
	assert.Equal(t, "success", result["result"])
}

func TestErrAfterConnectionError(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	writer.Close()

	var result map[string]string
	err := c.Call(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"}, &result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection closed by remote")

	assert.Error(t, c.Err())
}

func TestConcurrentCalls(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	go func() {
		for i := 0; i < 10; i++ {
			req := &Request{
				JSONRPC: "2.0",
				Method:  "testMethod",
				Params:  map[string]interface{}{"param1": "value1"},
				ID:      strconv.Itoa(i),
			}
			resp := &Response{
				JSONRPC: "2.0",
				Result:  json.RawMessage(`{"result": "success"}`),
				ID:      strconv.Itoa(i),
			}
			json.NewEncoder(writer).Encode(req)
			json.NewEncoder(writer).Encode(resp)
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var result map[string]string
			err := c.Call(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"}, &result)
			assert.NoError(t, err)
			assert.Equal(t, "success", result["result"])
		}(i)
	}

	wg.Wait()
}

func TestConcurrentCallsAndNotifies(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	go func() {
		for i := 0; i < 10; i++ {
			req := &Request{
				JSONRPC: "2.0",
				Method:  "testMethod",
				Params:  map[string]interface{}{"param1": "value1"},
				ID:      strconv.Itoa(i),
			}
			resp := &Response{
				JSONRPC: "2.0",
				Result:  json.RawMessage(`{"result": "success"}`),
				ID:      strconv.Itoa(i),
			}
			json.NewEncoder(writer).Encode(req)
			json.NewEncoder(writer).Encode(resp)
		}
		for i := 0; i < 10; i++ {
			notification := &Notification{
				Method: "testMethod",
				Params: json.RawMessage(`{"param1": "value1"}`),
			}
			json.NewEncoder(writer).Encode(notification)
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var result map[string]string
			err := c.Call(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"}, &result)
			assert.NoError(t, err)
			assert.Equal(t, "success", result["result"])
		}(i)
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := c.Notify(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"})
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()
}

func TestNullResultUnmarshalling(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	go func() {
		req := &Request{
			JSONRPC: "2.0",
			Method:  "testMethod",
			Params:  map[string]interface{}{"param1": "value1"},
			ID:      "1",
		}
		resp := &Response{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`null`),
			ID:      "1",
		}
		json.NewEncoder(writer).Encode(req)
		json.NewEncoder(writer).Encode(resp)
	}()

	var result map[string]string
	err := c.Call(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"}, &result)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestMalformedJSONMessages(t *testing.T) {
	reader, writer := io.Pipe()
	c := Connect(reader, writer)

	go func() {
		req := &Request{
			JSONRPC: "2.0",
			Method:  "testMethod",
			Params:  map[string]interface{}{"param1": "value1"},
			ID:      "1",
		}
		json.NewEncoder(writer).Encode(req)
		writer.Write([]byte("malformed"))
	}()

	var result map[string]string
	err := c.Call(context.Background(), "testMethod", map[string]interface{}{"param1": "value1"}, &result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "malformed message envelope")
}
