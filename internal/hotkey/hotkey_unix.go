//go:build linux || darwin

package hotkey

import (
	"os"
	"runtime"
	"strings"
)

// platformKeyCodes are the gohook raw key codes for Ctrl+Shift+R on X11/macOS.
// Note: gohook reports codes that differ from raw X11 keycodes for some keys.
var platformKeyCodes = keyCodes{
	leftCtrl:   37,
	rightCtrl:  105,
	leftShift:  50,
	rightShift: 62,
	keyR:       19,
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
