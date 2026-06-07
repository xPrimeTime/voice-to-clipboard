//go:build linux || darwin

// Package ipc provides the single-instance control transport. On Unix it uses a
// per-user Unix domain socket; on Windows a named pipe (see ipc_windows.go).
package ipc

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
)

// socketDir returns the per-user directory holding the IPC socket.
func socketDir() string {
	// Prefer the per-user runtime dir on Linux (typically /run/user/<uid>).
	if runtime.GOOS == "linux" {
		if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
			return filepath.Join(runtimeDir, "voice-to-clipboard")
		}
	}

	// Cross-platform per-user cache dir fallback.
	if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
		return filepath.Join(cacheDir, "voice-to-clipboard")
	}

	// Last-resort fallback for unusual environments.
	return filepath.Join(os.TempDir(), "voice-to-clipboard")
}

func socketPath() string {
	return filepath.Join(socketDir(), "ipc.sock")
}

// Address returns a human-readable identifier for the IPC endpoint (for logs).
func Address() string {
	return socketPath()
}

// Listen creates the IPC listener, removing any stale socket first and
// restricting the socket to the current user.
func Listen() (net.Listener, error) {
	if err := os.MkdirAll(socketDir(), 0700); err != nil {
		return nil, fmt.Errorf("failed to create IPC directory: %w", err)
	}
	if err := CleanupStale(); err != nil {
		return nil, err
	}

	listener, err := net.Listen("unix", socketPath())
	if err != nil {
		return nil, fmt.Errorf("failed to create IPC socket: %w", err)
	}

	if _, err := os.Stat(socketPath()); err != nil {
		listener.Close()
		return nil, fmt.Errorf("socket created but not accessible: %w", err)
	}
	if err := os.Chmod(socketPath(), 0600); err != nil {
		listener.Close()
		return nil, fmt.Errorf("failed to secure IPC socket permissions: %w", err)
	}

	return listener, nil
}

// Dial connects to a running instance's IPC endpoint.
func Dial() (net.Conn, error) {
	return net.Dial("unix", socketPath())
}

// CleanupStale removes a leftover socket file from a previous run. It refuses to
// remove a non-socket file at the path as a safety check.
func CleanupStale() error {
	path := socketPath()
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to check socket path: %w", err)
	}

	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to remove non-socket IPC path: %s", path)
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to remove stale socket: %w", err)
	}
	return nil
}
