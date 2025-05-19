package jsonrpc2

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
)

type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      interface{} `json:"id,omitempty"` // omit if notification
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ErrorObject    `json:"error,omitempty"`
	ID      interface{}     `json:"id"`
}

type ErrorObject struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (e *ErrorObject) Error() string {
	return fmt.Sprintf("jsonrpc: code: %d, message: %s", e.Code, e.Message)
}

type Client struct {
	encodeMu sync.Mutex // To protect concurrent writes to encoder

	conn    io.ReadWriteCloser
	encoder *json.Encoder // To write requests to conn
	decoder *json.Decoder // To read responses from conn

	pendingMu sync.Mutex
	pending   map[string]chan *Response // Map request ID (string) to response channel

	nextID uint64

	closing  chan struct{}
	shutdown chan struct{}

	errMu   sync.Mutex
	connErr error
}

func NewClient(conn io.ReadWriteCloser) *Client {
	c := &Client{
		conn:     conn,
		encoder:  json.NewEncoder(conn),
		decoder:  json.NewDecoder(conn),
		pending:  make(map[string]chan *Response),
		closing:  make(chan struct{}),
		shutdown: make(chan struct{}),
	}
	go c.readLoop()
	return c
}

func (c *Client) newRequestID() string {
	return strconv.FormatUint(atomic.AddUint64(&c.nextID, 1), 10)
}

func (c *Client) Call(ctx context.Context, method string, params interface{}, result interface{}) error {
	reqID := c.newRequestID()
	req := &Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      reqID,
	}

	respChan := make(chan *Response, 1)

	// Deferred cleanup for this request's pending channel
	defer func() {
		c.pendingMu.Lock()
		// Check if the channel is still the one we created,
		// as readLoop might have processed it and deleted/closed it.
		if ch, ok := c.pending[reqID]; ok && ch == respChan {
			delete(c.pending, reqID)
			// close(respChan) // readLoop is responsible for closing if it sends a response
		}
		c.pendingMu.Unlock()
	}()

	c.encodeMu.Lock()
	err := c.encoder.Encode(req)
	c.encodeMu.Unlock()
	if err != nil {
		return err // Deferred cleanup will run
	}

	select {
	case <-ctx.Done():
		return ctx.Err() // Deferred cleanup will run
	case resp, ok := <-respChan:
		if !ok { // Channel was closed by readLoop's error handling or successful send+close
			// If closed by readLoop error, c.Err() should be populated
			// If closed after successful send, resp would have been received before !ok.
			// This case implies Call might have raced with readLoop's error handling.
			return c.Err()
		}
		if resp.Error != nil {
			return resp.Error
		}
		// Check if result is nil before unmarshalling
		if result != nil && resp.Result != nil && len(resp.Result) > 0 && string(resp.Result) != "null" {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return err
			}
		}
		return nil
	case <-c.closing:
		return c.Err() // Deferred cleanup will run
	}
}

func (c *Client) Notify(ctx context.Context, method string, params interface{}) error {
	req := &Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.encodeMu.Lock()
	err := c.encoder.Encode(req)
	c.encodeMu.Unlock()
	return err
}

func (c *Client) Close() error {
	close(c.closing)
	<-c.shutdown
	return c.conn.Close()
}

func (c *Client) Err() error {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.connErr
}

func (c *Client) readLoop() {
	for {
		var resp Response
		if err := c.decoder.Decode(&resp); err != nil {
			if err == io.EOF {
				err = fmt.Errorf("connection closed")
			}
			c.errMu.Lock()
			c.connErr = err
			c.errMu.Unlock()
			close(c.closing)
			break
		}

		idStr := fmt.Sprintf("%v", resp.ID)
		c.pendingMu.Lock()
		respChan, ok := c.pending[idStr]
		delete(c.pending, idStr)
		c.pendingMu.Unlock()

		if ok {
			respChan <- &resp
			close(respChan)
		}
	}
	close(c.shutdown)
}
