// Package hotkey provides an optional built-in global hotkey (Ctrl+Shift+R).
//
// The gohook-based listener is compiled in on Windows only. On Linux and
// macOS the package reports unsupported: gohook is X11-only there (useless
// on Wayland sessions), and its C constructor crashes the whole process at
// load time when no X display is available (headless invocations, Wayland
// without XWayland), so merely linking it is a liability. Users bind
// `voice-to-clipboard --toggle` to a system keybind instead (see README).
package hotkey

// Manager handles global hotkey registration
type Manager struct {
	onToggle func()
	stopChan chan struct{}
}

// New creates a new hotkey manager
func New(onToggle func()) *Manager {
	return &Manager{
		onToggle: onToggle,
	}
}

// Start begins listening for the global hotkey
func (h *Manager) Start() error {
	return h.start()
}

// Stop stops listening for hotkeys
func (h *Manager) Stop() {
	h.stop()
}

// IsSupported returns whether a built-in global hotkey is supported on this
// platform. When false, the IPC --toggle command bound to a system keybind
// is the supported alternative.
func IsSupported() bool {
	return isSupported()
}
