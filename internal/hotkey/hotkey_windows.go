//go:build windows

package hotkey

import (
	"voice-to-clipboard/internal/logger"

	hook "github.com/robotn/gohook"
)

// Windows Virtual Key codes
const (
	vkLeftCtrl   = 0xA2
	vkRightCtrl  = 0xA3
	vkLeftShift  = 0xA0
	vkRightShift = 0xA1
	vkKeyR       = 0x52
)

func (h *Manager) start() error {
	h.stopChan = make(chan struct{})

	logger.Info("Starting global hotkey listener (Windows)", "key", "Ctrl+Shift+R")

	go func() {
		// Register Ctrl+Shift+R as the hotkey
		evChan := hook.Start()
		defer hook.End()

		ctrlPressed := false
		shiftPressed := false

		for {
			select {
			case <-h.stopChan:
				return
			case ev := <-evChan:
				// Track modifier key states
				if ev.Kind == hook.KeyDown {
					if ev.Rawcode == vkLeftCtrl || ev.Rawcode == vkRightCtrl {
						ctrlPressed = true
					}
					if ev.Rawcode == vkLeftShift || ev.Rawcode == vkRightShift {
						shiftPressed = true
					}

					// Check for R key with both modifiers
					if ev.Rawcode == vkKeyR && ctrlPressed && shiftPressed {
						logger.Info("Global hotkey triggered", "key", "Ctrl+Shift+R")
						if h.onToggle != nil {
							h.onToggle()
						}
					}
				} else if ev.Kind == hook.KeyUp {
					if ev.Rawcode == vkLeftCtrl || ev.Rawcode == vkRightCtrl {
						ctrlPressed = false
					}
					if ev.Rawcode == vkLeftShift || ev.Rawcode == vkRightShift {
						shiftPressed = false
					}
				}
			}
		}
	}()

	return nil
}

func isSupported() bool {
	return true
}
