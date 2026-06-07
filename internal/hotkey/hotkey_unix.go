//go:build linux || darwin

package hotkey

import (
	"os"
	"runtime"
	"strings"

	"voice-to-clipboard/internal/logger"

	hook "github.com/robotn/gohook"
)

// X11 key codes (from xev output)
const (
	x11LeftCtrl   = 37
	x11RightCtrl  = 105
	x11LeftShift  = 50
	x11RightShift = 62
	x11KeyR       = 19 // Note: gohook uses different codes than standard X11
)

func (h *Manager) start() error {
	h.stopChan = make(chan struct{})

	logger.Info("Starting global hotkey listener", "key", "Ctrl+Shift+R")

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
					if ev.Rawcode == x11LeftCtrl || ev.Rawcode == x11RightCtrl {
						ctrlPressed = true
					}
					if ev.Rawcode == x11LeftShift || ev.Rawcode == x11RightShift {
						shiftPressed = true
					}

					// Check for R key with both modifiers
					if ev.Rawcode == x11KeyR && ctrlPressed && shiftPressed {
						logger.Info("Global hotkey triggered", "key", "Ctrl+Shift+R")
						if h.onToggle != nil {
							h.onToggle()
						}
					}
				} else if ev.Kind == hook.KeyUp {
					if ev.Rawcode == x11LeftCtrl || ev.Rawcode == x11RightCtrl {
						ctrlPressed = false
					}
					if ev.Rawcode == x11LeftShift || ev.Rawcode == x11RightShift {
						shiftPressed = false
					}
				}
			}
		}
	}()

	return nil
}

func isSupported() bool {
	if runtime.GOOS == "darwin" {
		return true
	}

	// Linux support is X11-only. Wayland sessions should use compositor keybinds.
	sessionType := strings.ToLower(os.Getenv("XDG_SESSION_TYPE"))
	if sessionType == "wayland" {
		return false
	}

	waylandDisplay := os.Getenv("WAYLAND_DISPLAY")
	display := os.Getenv("DISPLAY")
	if waylandDisplay != "" && display == "" {
		return false
	}

	return display != ""
}
