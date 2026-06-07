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

// Start begins listening for hotkeys (implementation is platform-specific)
func (h *Manager) Start() error {
	return h.start()
}

// Stop stops listening for hotkeys
func (h *Manager) Stop() {
	h.stop()
}

// Common stop implementation used by both platforms
func (h *Manager) stop() {
	if h.stopChan != nil {
		close(h.stopChan)
		h.stopChan = nil
	}
}

// IsSupported returns whether global hotkeys are supported on this platform
func IsSupported() bool {
	return isSupported()
}
