//go:build windows

package hotkey

import (
	"voice-to-clipboard/internal/logger"

	hook "github.com/robotn/gohook"
)

// keyCodes holds the gohook raw key codes for the Ctrl+Shift+R hotkey.
type keyCodes struct {
	leftCtrl, rightCtrl   uint16
	leftShift, rightShift uint16
	keyR                  uint16
}

// platformKeyCodes are the Windows virtual-key codes for Ctrl+Shift+R.
var platformKeyCodes = keyCodes{
	leftCtrl:   0xA2,
	rightCtrl:  0xA3,
	leftShift:  0xA0,
	rightShift: 0xA1,
	keyR:       0x52,
}

func isSupported() bool {
	return true
}

// stop signals the listener goroutine to exit
func (h *Manager) stop() {
	if h.stopChan != nil {
		close(h.stopChan)
		h.stopChan = nil
	}
}

// start launches the gohook listener goroutine.
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
