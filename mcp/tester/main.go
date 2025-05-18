package main

import (
	"fmt"
	"io"
	"log"
	"net/rpc"
	"os"
	"os/exec"

	// IMPORTANT: Replace YOUR_MODULE_PATH with the actual Go module path
	// for your mcp package, e.g., "github.com/youruser/yourproject/mcp"
	"YOUR_MODULE_PATH/mcp"
)

// SubprocessReadWriteCloser wraps the stdin and stdout of an exec.Cmd
// to be used as an io.ReadWriteCloser for RPC.
// It also manages the lifecycle of the command.
type SubprocessReadWriteCloser struct {
	io.WriteCloser // Stdin of the subprocess
	io.ReadCloser  // Stdout of the subprocess
	cmd            *exec.Cmd
}

// NewSubprocessReadWriteCloser creates a new SubprocessReadWriteCloser.
// It requires the caller to have already started the command.
func NewSubprocessReadWriteCloser(cmd *exec.Cmd, stdin io.WriteCloser, stdout io.ReadCloser) *SubprocessReadWriteCloser {
	return &SubprocessReadWriteCloser{
		WriteCloser: stdin,
		ReadCloser:  stdout,
		cmd:         cmd,
	}
}

// Close closes the stdin and stdout pipes and waits for the subprocess to exit.
func (s *SubprocessReadWriteCloser) Close() error {
	var errs []error

	// Close stdin first to signal EOF to the subprocess
	if err := s.WriteCloser.Close(); err != nil {
		errString := fmt.Sprintf("failed to close subprocess stdin: %v", err)
		log.Println(errString)
		errs = append(errs, fmt.Errorf(errString))
	}

	// Closing stdout isn't strictly necessary for the subprocess to terminate based on stdin EOF,
	// but good practice for resource cleanup on our side.
	if err := s.ReadCloser.Close(); err != nil {
		errString := fmt.Sprintf("failed to close subprocess stdout: %v", err)
		log.Println(errString)
		errs = append(errs, fmt.Errorf(errString))
	}

	// Wait for the command to exit and release its resources.
	if err := s.cmd.Wait(); err != nil {
		// log.Printf("Subprocess wait error: %v (ExitCode: %d)", err, s.cmd.ProcessState.ExitCode())
		// Differentiate between non-zero exit and other errors if needed
		if exitErr, ok := err.(*exec.ExitError); ok {
			errString := fmt.Sprintf("subprocess exited with error: %v, stderr: %s", exitErr, string(exitErr.Stderr))
			log.Println(errString)
			errs = append(errs, fmt.Errorf(errString))
		} else {
			errString := fmt.Sprintf("failed to wait for subprocess: %v", err)
			log.Println(errString)
			errs = append(errs, fmt.Errorf(errString))
		}
	} else {
		log.Printf("Subprocess exited cleanly (PID: %d).", s.cmd.ProcessState.Pid())
	}

	if len(errs) > 0 {
		return fmt.Errorf("encountered %d error(s) during close: %v", len(errs), errs)
	}
	return nil
}

func main() {
	log.Println("Starting MCP tester...")

	// 1. Prepare the command
	cmd := exec.Command("uvx", "mcp-server-fetch")

	// 2. Get pipes for stdin and stdout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalf("Failed to get stdin pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}

	// Optional: Capture stderr for debugging the subprocess
	cmd.Stderr = os.Stderr // Or a bytes.Buffer if you want to capture it

	// 3. Start the command
	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start command: %v", err)
	}
	log.Printf("Subprocess started (PID: %d)", cmd.Process.Pid)

	// 4. Create the ReadWriteCloser
	spRwc := NewSubprocessReadWriteCloser(cmd, stdin, stdout)

	// 5. Create the JSON-RPC client
	// The mcp.NewJSONRPC2ClientCodec comes from the mcp package we wrote earlier.
	// Ensure YOUR_MODULE_PATH/mcp is correct in imports.
	client := rpc.NewClientWithCodec(mcp.NewJSONRPC2ClientCodec(spRwc))
	defer func() {
		log.Println("Closing RPC client and subprocess ReadWriteCloser...")
		if err := client.Close(); err != nil {
			log.Printf("Error closing client/subprocess: %v", err)
		}
		log.Println("Client closed.")
	}()

	// 6. Prepare the request
	// As per spec: id 1, method: "tools/list", and an empty params object.
	// The net/rpc client handles ID generation. We just provide method and params.
	params := make(map[string]interface{}) // Empty params object
	var reply interface{}                  // To store the generic JSON response

	log.Println("Sending RPC request to 'tools/list'...")
	// 7. Perform the RPC call
	err = client.Call("tools/list", params, &reply)
	if err != nil {
		// If the error is rpc.ErrShutdown, it means the client was closed, possibly due to subprocess exit.
		// The deferred Close() also calls cmd.Wait() which might log more details about subprocess exit.
		log.Fatalf("RPC call 'tools/list' failed: %v", err)
	}

	// 8. Output the response
	log.Println("RPC call successful.")
	fmt.Printf("Response from server: %+v\n", reply)

	log.Println("MCP tester finished successfully.")
}
