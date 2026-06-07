package hotkey

import (
	"voice-to-clipboard/internal/logger"

	hook "github.com/robotn/gohook"
)

// keyCodes holds the platform-specific gohook raw key codes for the
// Ctrl+Shift+R hotkey. Each platform file provides platformKeyCodes.
type keyCodes struct {
	leftCtrl, rightCtrl   uint16
	leftShift, rightShift uint16
	keyR                  uint16
}

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

// stop signals the listener goroutine to exit
func (h *Manager) stop() {
	if h.stopChan != nil {
		close(h.stopChan)
		h.stopChan = nil
	}
}

// start launches the listener goroutine. The event loop is shared across
// platforms; only the raw key codes (platformKeyCodes) differ.
func (h *Manager) start() error {
	h.stopChan = make(chan struct{})

	logger.Info("Starting global hotkey listener", "key", "Ctrl+Shift+R")

	codes := platformKeyCodes
	go func() {
		evChan := hook.Start()
		defer hook.End()

		ctrlPressed := false
		shiftPressed := false

		for {
			select {
			case <-h.stopChan:
				return
			case ev := <-evChan:
				switch ev.Kind {
				case hook.KeyDown:
					switch ev.Rawcode {
					case codes.leftCtrl, codes.rightCtrl:
						ctrlPressed = true
					case codes.leftShift, codes.rightShift:
						shiftPressed = true
					case codes.keyR:
						if ctrlPressed && shiftPressed {
							logger.Info("Global hotkey triggered", "key", "Ctrl+Shift+R")
							if h.onToggle != nil {
								h.onToggle()
							}
						}
					}
				case hook.KeyUp:
					switch ev.Rawcode {
					case codes.leftCtrl, codes.rightCtrl:
						ctrlPressed = false
					case codes.leftShift, codes.rightShift:
						shiftPressed = false
					}
				}
			}
		}
	}()

	return nil
}

// IsSupported returns whether global hotkeys are supported on this platform
func IsSupported() bool {
	return isSupported()
}
