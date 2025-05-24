package main

import (
	"bufio"
	"context"

	// "encoding/json" // No longer directly used here
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/dhamidi/smolcode/mcp/jsonrpc2" // Updated import path
)

// TeeReadCloser wraps an io.Reader (the TeeReader) and an io.Closer (the original stdout pipe)
// to satisfy the io.ReadCloser interface.
type TeeReadCloser struct {
	reader io.Reader
	closer io.Closer
}

// Read reads from the TeeReader.
func (trc *TeeReadCloser) Read(p []byte) (n int, err error) {
	return trc.reader.Read(p)
}

// Close closes the underlying stdout pipe.
func (trc *TeeReadCloser) Close() error {
	return trc.closer.Close()
}

// SubprocessTransport implements the jsonrpc2.Transport interface
// for communication with a subprocess via stdin/stdout.
// It also implements io.Closer to manage the subprocess lifecycle.
type SubprocessTransport struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     *bufio.Reader // Use bufio.Reader for ReadBytes
	stdoutPipe io.ReadCloser // Keep the original pipe for closing
	stderrPipe io.ReadCloser // Store stderr pipe for closing and separate reading goroutine

	closed   chan struct{}
	closeErr error
	once     sync.Once
}

// NewSubprocessTransport starts the given command and sets up pipes for communication.
func NewSubprocessTransport(command string, args ...string) (*SubprocessTransport, error) {
	cmd := exec.Command(command, args...)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdinPipe.Close()
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// Tee stdout to os.Stdout for real-time observation
	teeStdoutReader := io.TeeReader(stdoutPipe, os.Stdout)

	stderrPipeForCmd, err := cmd.StderrPipe()
	if err != nil {
		stdinPipe.Close()
		stdoutPipe.Close()
		return nil, fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdinPipe.Close()
		stdoutPipe.Close()
		stderrPipeForCmd.Close()
		return nil, fmt.Errorf("failed to start command: %w", err)
	}
	log.Printf("Subprocess started (PID: %d): %s %v", cmd.Process.Pid, command, args)

	spt := &SubprocessTransport{
		cmd:        cmd,
		stdin:      stdinPipe,
		stdout:     bufio.NewReader(teeStdoutReader),
		stdoutPipe: stdoutPipe,       // Store the original pipe for closing
		stderrPipe: stderrPipeForCmd, // Store for closing
		closed:     make(chan struct{}),
	}

	// Goroutine to capture and log stderr from spt.stderrPipe
	go func(stderrToRead io.ReadCloser) {
		stderrScanner := bufio.NewScanner(stderrToRead)
		for stderrScanner.Scan() {
			log.Printf("MCP Server STDERR: %s", stderrScanner.Text())
		}
		if scanErr := stderrScanner.Err(); scanErr != nil {
			if scanErr != io.EOF && scanErr != os.ErrClosed {
				log.Printf("Error reading subprocess stderr: %v", scanErr)
			}
		}
	}(stderrPipeForCmd) // Pass the pipe to the goroutine

	return spt, nil
}

// Send implements the jsonrpc2.Transport interface.
// It sends a pre-formatted JSON-RPC message payload, adding a newline delimiter.
func (spt *SubprocessTransport) Send(ctx context.Context, payload []byte) error {
	select {
	case <-spt.closed:
		return spt.closeErr // Or a more specific "transport closed" error
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if _, err := spt.stdin.Write(payload); err != nil {
		spt.Close() // Attempt to close on send error
		return fmt.Errorf("transport: failed to write payload: %w", err)
	}
	if _, err := spt.stdin.Write([]byte{'\n'}); err != nil { // Add newline delimiter
		spt.Close() // Attempt to close on send error
		return fmt.Errorf("transport: failed to write newline delimiter: %w", err)
	}
	return nil
}

// Receive implements the jsonrpc2.Transport interface.
// It waits for and returns the next JSON-RPC message payload from the underlying connection,
// reading until a newline delimiter.
func (spt *SubprocessTransport) Receive(ctx context.Context) (payload []byte, err error) {
	done := make(chan struct{}) // Channel to signal completion or timeout
	var readErr error
	var line []byte

	go func() {
		line, readErr = spt.stdout.ReadBytes('\n') // Use '\n' for ReadBytes
		close(done)
	}()

	select {
	case <-spt.closed:
		return nil, spt.closeErr // Prefer closeErr if transport is explicitly closed
	case <-ctx.Done():
		return nil, ctx.Err() // Context cancelled or timed out
	case <-done:
		// Operation completed
	}

	if readErr != nil {
		if readErr == io.EOF && len(line) > 0 {
			log.Printf("Transport: Received EOF with partial data: %s", string(line))
			return trimSpace(line), nil // Return the trimmed partial line
		} else if readErr == io.EOF {
			log.Println("Transport: Received EOF from subprocess stdout.")
			spt.Close()        // Subprocess exited
			return nil, io.EOF // Signal clean EOF
		}
		select {
		case <-spt.closed:
			return nil, spt.closeErr
		default:
			log.Printf("Transport: Error reading from subprocess stdout: %v", readErr)
			return nil, fmt.Errorf("transport: receive error: %w", readErr)
		}
	}

	trimmedLine := trimSpace(line)
	if len(trimmedLine) == 0 {
		return nil, fmt.Errorf("transport: received empty line")
	}
	return trimmedLine, nil
}

// Close implements io.Closer for the transport.
// It closes stdin of the subprocess, then stdout and stderr pipes, waits for it to exit, and cleans up resources.
func (spt *SubprocessTransport) Close() error {
	spt.once.Do(func() {
		log.Println("SubprocessTransport: Closing...")

		// Close stdin first to signal EOF to the subprocess.
		if spt.stdin != nil {
			if err := spt.stdin.Close(); err != nil {
				log.Printf("SubprocessTransport: failed to close subprocess stdin: %v", err)
				if spt.closeErr == nil {
					spt.closeErr = err
				}
			}
		}

		// Close the stdout pipe that the transport was reading from.
		if spt.stdoutPipe != nil {
			if err := spt.stdoutPipe.Close(); err != nil {
				log.Printf("SubprocessTransport: error closing stdout pipe: %v", err)
				if spt.closeErr == nil {
					spt.closeErr = err
				}
			}
		}

		// Close the stderr pipe that the transport was using.
		if spt.stderrPipe != nil {
			if err := spt.stderrPipe.Close(); err != nil {
				log.Printf("SubprocessTransport: error closing stderr pipe: %v", err)
				if spt.closeErr == nil {
					spt.closeErr = err
				}
			}
		}

		// Wait for the command to exit.
		waitErr := make(chan error, 1)
		go func() {
			waitErr <- spt.cmd.Wait()
		}()

		select {
		case err := <-waitErr:
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					errStr := fmt.Sprintf("SubprocessTransport: subprocess exited with error: %v. Stderr may have more.", exitErr)
					log.Println(errStr)
					if spt.closeErr == nil {
						spt.closeErr = fmt.Errorf(errStr)
					}
				} else {
					errStr := fmt.Sprintf("SubprocessTransport: failed to wait for subprocess: %v", err)
					log.Println(errStr)
					if spt.closeErr == nil {
						spt.closeErr = fmt.Errorf(errStr)
					}
				}
			} else {
				log.Printf("SubprocessTransport: Subprocess (PID: %d) exited cleanly.", spt.cmd.ProcessState.Pid())
			}
		case <-time.After(5 * time.Second):
			log.Println("SubprocessTransport: Timeout waiting for subprocess to exit. Attempting to kill.")
			if err := spt.cmd.Process.Kill(); err != nil {
				log.Printf("SubprocessTransport: Failed to kill subprocess: %v", err)
				if spt.closeErr == nil {
					spt.closeErr = fmt.Errorf("failed to kill subprocess: %w", err)
				}
			} else {
				log.Println("SubprocessTransport: Subprocess killed.")
				if spt.closeErr == nil {
					spt.closeErr = fmt.Errorf("subprocess timed out and was killed")
				}
			}
			<-waitErr
		}
		close(spt.closed)
		log.Println("SubprocessTransport: Closed.")
	})
	return spt.closeErr
}

func main() {
	log.Println("Starting MCP tester with jsonrpc2 client...")

	transport, err := NewSubprocessTransport("uvx", "mcp-server-fetch")
	if err != nil {
		log.Fatalf("Failed to create subprocess transport: %v", err)
	}

	client := jsonrpc2.NewClient(transport)
	clientCtx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()

	go func() {
		log.Println("Client listener starting...")
		listenErr := client.Listen()
		if listenErr != nil && listenErr != context.Canceled && listenErr != io.EOF && listenErr.Error() != "context canceled" {
			log.Printf("Client Listen error: %v", listenErr)
		}
		log.Println("Client listener stopped.")
	}()

	defer func() {
		log.Println("Closing RPC client (which will also close transport)...")
		if err := client.Close(); err != nil {
			log.Printf("Error closing client: %v", err)
		}
		log.Println("Client and transport closed.")
	}()

	mainOpCtx, mainOpCancel := context.WithTimeout(clientCtx, 30*time.Second)
	defer mainOpCancel()

	log.Println("Dispatching RPC request to 'initialize'...")
	initParams := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "smolcode-tester-jsonrpc2",
			"version": "1.0.2",
		},
	}
	var initReply interface{}
	callCtx, callCancel := context.WithTimeout(mainOpCtx, 10*time.Second)
	err = client.Call(callCtx, jsonrpc2.ClientCallArgs{Method: "initialize", Params: initParams}, &initReply)
	callCancel()
	if err != nil {
		log.Fatalf("RPC call 'initialize' failed: %v", err)
	}
	log.Printf("[SUCCESS] 'initialize' call successful. Reply: %+v\n", initReply)

	log.Println("Sending RPC notification to 'notifications/initialized'...")
	notifyCtx, notifyCancel := context.WithTimeout(mainOpCtx, 5*time.Second)
	err = client.Notify(notifyCtx, jsonrpc2.ClientNotifyArgs{Method: "notifications/initialized", Params: nil})
	notifyCancel()
	if err != nil {
		log.Printf("RPC notification 'notifications/initialized' failed: %v", err)
	} else {
		log.Println("[SUCCESS] RPC notification 'notifications/initialized' sent successfully.")
	}

	log.Println("Sending RPC request to 'tools/list'...")
	listParams := make(map[string]interface{})
	var listReply interface{}
	callCtx, callCancel = context.WithTimeout(mainOpCtx, 10*time.Second)
	err = client.Call(callCtx, jsonrpc2.ClientCallArgs{Method: "tools/list", Params: listParams}, &listReply)
	callCancel()
	if err != nil {
		log.Fatalf("RPC call 'tools/list' failed: %v", err)
	}
	log.Println("[SUCCESS] RPC call 'tools/list' successful.")
	fmt.Printf("[RESULT] Response from 'tools/list':\n%+v\n", listReply)

	log.Println("Sending RPC request to 'tools/call' for 'fetch' tool...")
	toolCallParams := map[string]interface{}{
		"name": "fetch",
		"arguments": map[string]interface{}{
			"url": "https://example.com",
		},
	}
	var toolCallReply interface{}
	callCtx, callCancel = context.WithTimeout(mainOpCtx, 15*time.Second)
	err = client.Call(callCtx, jsonrpc2.ClientCallArgs{Method: "tools/call", Params: toolCallParams}, &toolCallReply)
	callCancel()
	if err != nil {
		log.Fatalf("RPC call 'tools/call' for 'fetch' failed: %v", err)
	}
	log.Println("[SUCCESS] RPC call 'tools/call' for 'fetch' successful.")
	fmt.Printf("[RESULT] Response from 'tools/call' (fetch):\n%+v\n", toolCallReply)

	log.Println("[SUCCESS] MCP tester finished successfully using jsonrpc2 client.")
}

// trimSpace removes leading and trailing ASCII white space from a byte slice.
func trimSpace(s []byte) []byte {
	start := 0
	for start < len(s) && isSpace(s[start]) {
		start++
	}
	end := len(s)
	for end > start && isSpace(s[end-1]) {
		end--
	}
	if start == end {
		return s[0:0]
	}
	return s[start:end]
}

// isSpace reports whether b is an ASCII space character.
func isSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}
