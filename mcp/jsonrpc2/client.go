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
	Error   *Error          `json:"error,omitempty"`
	ID      interface{}     `json:"id"`
}

type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Notification is a message received from the server that is not a response to a request.
type Notification struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type multiCloser []io.Closer

func (mc multiCloser) Close() error {
	var err error
	for _, c := range mc {
		if e := c.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

// incomingMessage is used to determine if a message is a Response or Notification
type incomingMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
}

// noopCloser is an io.Closer that does nothing.
type noopCloser struct{}

func (noopCloser) Close() error { return nil }

func (e *Error) Error() string {
	return fmt.Sprintf("jsonrpc: code: %d, message: %s", e.Code, e.Message)
}

type Connection struct {
	encodeMu sync.Mutex // To protect concurrent writes to encoder

	reader  io.Reader
	writer  io.Writer
	encoder *json.Encoder // To write requests to writer
	decoder *json.Decoder // To read responses from reader
	closer  io.Closer     // For closing the underlying connection(s)

	notificationChan chan *Notification // Channel for server notifications

	pendingMu sync.Mutex
	pending   map[string]chan *Response // Map request ID (string) to response channel

	nextID uint64

	closing  chan struct{}
	shutdown chan struct{}

	errMu   sync.Mutex
	connErr error
}

func Connect(reader io.Reader, writer io.Writer) *Connection {
	var closer io.Closer
	closers := []io.Closer{}
	if rCloser, ok := reader.(io.Closer); ok {
		closers = append(closers, rCloser)
	}
	if wCloser, ok := writer.(io.Closer); ok {
		isSame := false
		// Check if reader and writer are the same closable entity to avoid double adding/closing
		if len(closers) > 0 {
			// A more robust check might be needed if they are different types but wrap the same resource.
			// For common cases like net.Conn, this simple reference check for io.Closer interface should suffice.
			if _, rok := reader.(io.ReadWriteCloser); rok {
				if _, wok := writer.(io.ReadWriteCloser); wok {
					if reader == writer { // Check if they are the exact same instance
						isSame = true
					}
				}
			} else if closers[0] == wCloser { // Fallback if not RWC, check if the closer interfaces are the same instance
				isSame = true
			}
		}
		if !isSame {
			closers = append(closers, wCloser)
		}
	}

	if len(closers) == 0 {
		closer = noopCloser{}
	} else if len(closers) == 1 {
		closer = closers[0]
	} else {
		closer = multiCloser(closers)
	}

	c := &Connection{
		reader:           reader,
		writer:           writer,
		encoder:          json.NewEncoder(writer), // Use writer for encoder
		decoder:          json.NewDecoder(reader), // Use reader for decoder
		closer:           closer,                  // Use the determined closer
		pending:          make(map[string]chan *Response),
		notificationChan: make(chan *Notification, 10), // Buffer for notifications
		closing:          make(chan struct{}),
		shutdown:         make(chan struct{}),
	}
	go c.readLoop()
	return c
}

func (c *Connection) newRequestID() string {
	return strconv.FormatUint(atomic.AddUint64(&c.nextID, 1), 10)
}

func (c *Connection) Call(ctx context.Context, method string, params interface{}, result interface{}) error {
	reqID := c.newRequestID()
	req := &Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      reqID,
	}

	respChan := make(chan *Response, 1)

	c.pendingMu.Lock()
	// Check if client is closing before adding to pending map
	select {
	case <-c.closing:
		c.pendingMu.Unlock()
		if c.connErr != nil { // Check if there's an underlying connection error
			return c.connErr
		}
		return fmt.Errorf("jsonrpc2: client is closing") // Generic closing error
	default:
		// Continue if not closing
	}
	c.pending[reqID] = respChan
	c.pendingMu.Unlock()

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

func (c *Connection) Notify(ctx context.Context, method string, params interface{}) error {
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

func (c *Connection) Close() error {
	close(c.closing)
	<-c.shutdown
	return c.closer.Close()
}

func (c *Connection) Err() error {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.connErr
}

// Subscribe returns a channel that receives unsolicited server notifications.
func (c *Connection) Subscribe() <-chan *Notification {
	return c.notificationChan
}

func (c *Connection) readLoop() {
	defer close(c.shutdown) // Ensure shutdown is closed on loop exit
	defer func() {          // Clean up pending requests and notification channel on error/exit
		c.pendingMu.Lock()
		for id, ch := range c.pending {
			// Don't send error here, Call will get it from c.Err() or ctx timeout
			// Or, if c.connErr is set, Call should check it.
			// Closing the channel is the signal that no more response will come.
			close(ch)
			delete(c.pending, id)
		}
		c.pendingMu.Unlock()

		// Close notificationChan only if it hasn't been closed already by some other path
		// (e.g. if readLoop exits cleanly after c.closing was signaled externally)
		// A select with a default case can check if a channel is closed without blocking.
		// However, it's simpler to just try to close it; a panic on double close means a logic error.
		// For safety, and as per Go's recommendation, only the sender should close a channel,
		// or it should be closed by a select on a done/closing channel.
		// Since readLoop is the sender for notificationChan (conceptually), it should close it.
		// To prevent panic on double close (e.g. if Close() also tries to close it),
		// we can add a flag or ensure Close() doesn't close it directly.
		// Given c.closing is used to signal termination, this defer will run once readLoop is exiting.
		select {
		case <-c.notificationChan:
			// already closed or has item, not safe to close here unless we are sure
		default:
			close(c.notificationChan)
		}
	}()

	for {
		var rawMessage json.RawMessage
		if err := c.decoder.Decode(&rawMessage); err != nil {
			if err == io.EOF {
				err = fmt.Errorf("jsonrpc2: connection closed by remote")
			} else {
				err = fmt.Errorf("jsonrpc2: decode error: %w", err)
			}

			c.errMu.Lock()
			if c.connErr == nil { // Set error only if not already set (e.g., by Close)
				c.connErr = err
			}
			c.errMu.Unlock()

			select {
			case <-c.closing: // Already closing or closed
			default:
				close(c.closing) // Signal connection error to other parts
			}
			return // Exit readLoop
		}

		var checker incomingMessage
		if err := json.Unmarshal(rawMessage, &checker); err != nil {
			err = fmt.Errorf("jsonrpc2: malformed message envelope: %w, raw: %s", err, string(rawMessage))
			c.errMu.Lock()
			if c.connErr == nil {
				c.connErr = err
			}
			c.errMu.Unlock()
			select {
			case <-c.closing:
			default:
				close(c.closing)
			}
			return
		}

		if checker.ID != nil && string(checker.ID) != "null" { // It's a Response if ID is present and not null
			var resp Response
			if err := json.Unmarshal(rawMessage, &resp); err != nil {
				err = fmt.Errorf("jsonrpc2: malformed response body: %w, raw: %s", err, string(rawMessage))
				c.errMu.Lock()
				if c.connErr == nil {
					c.connErr = err
				}
				c.errMu.Unlock()
				select {
				case <-c.closing:
				default:
					close(c.closing)
				}
				return
			}

			var idStr string
			// Unmarshal resp.ID (which is interface{}) into a string for map key
			switch v := resp.ID.(type) {
			case string:
				idStr = v
			case float64: // encoding/json unmarshals numbers into float64 by default
				idStr = strconv.FormatFloat(v, 'f', -1, 64)
			case json.Number: // If decoder is configured to use json.Number
				idStr = v.String()
			default:
				// Fallback, but spec says ID should be string or number (or null)
				idStr = fmt.Sprintf("%v", v)
			}

			c.pendingMu.Lock()
			respChan, ok := c.pending[idStr]
			if ok {
				delete(c.pending, idStr)
			}
			c.pendingMu.Unlock()

			if ok {
				select {
				case respChan <- &resp:
				case <-c.closing: // If connection is closing, don't block
				}
				close(respChan)
			} else {
				// fmt.Printf("jsonrpc2: received response for unknown or timed out request ID: %s\n", idStr)
			}
		} else if checker.Method != "" { // It's a Notification if Method is present (ID is typically absent or null for notifications)
			type wireNotification struct { // Use a temporary struct for full unmarshalling
				JSONRPC string          `json:"jsonrpc"`
				Method  string          `json:"method"`
				Params  json.RawMessage `json:"params,omitempty"`
			}
			var wn wireNotification
			if err := json.Unmarshal(rawMessage, &wn); err != nil {
				err = fmt.Errorf("jsonrpc2: malformed notification body: %w, raw: %s", err, string(rawMessage))
				c.errMu.Lock()
				if c.connErr == nil {
					c.connErr = err
				}
				c.errMu.Unlock()
				select {
				case <-c.closing:
				default:
					close(c.closing)
				}
				return
			}

			notificationToSend := &Notification{Method: wn.Method, Params: wn.Params}
			select {
			case c.notificationChan <- notificationToSend:
			case <-c.closing:
				return
			}
		} else {
			// fmt.Printf("jsonrpc2: received unclassifiable message: %s\n", string(rawMessage))
		}

		select {
		case <-c.closing:
			return // Exit loop if closing is signaled
		default:
			// Continue reading
		}
	}
}
