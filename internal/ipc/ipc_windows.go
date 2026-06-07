//go:build windows

package ipc

import (
	"net"
	"os"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
)

// pipeName returns a per-user named-pipe path. Including the username keeps
// instances for different users on the same machine separate; the pipe's
// default security descriptor already restricts access to the creator.
func pipeName() string {
	user := os.Getenv("USERNAME")
	if user == "" {
		user = "default"
	}
	// Pipe names can't contain backslashes beyond the prefix; sanitize.
	user = strings.NewReplacer("\\", "_", "/", "_", " ", "_").Replace(user)
	return `\\.\pipe\voice-to-clipboard-` + user
}

// Address returns a human-readable identifier for the IPC endpoint (for logs).
func Address() string {
	return pipeName()
}

// Listen creates the named-pipe listener.
func Listen() (net.Listener, error) {
	return winio.ListenPipe(pipeName(), nil)
}

// Dial connects to a running instance's named pipe. A short timeout makes the
// "no running instance" case fail fast.
func Dial() (net.Conn, error) {
	timeout := 2 * time.Second
	return winio.DialPipe(pipeName(), &timeout)
}

// CleanupStale is a no-op on Windows: a named pipe disappears when its last
// listener handle is closed, so there is nothing to clean up between runs.
func CleanupStale() error {
	return nil
}
