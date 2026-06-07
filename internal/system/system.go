package system

import (
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/atotto/clipboard"
	"github.com/gen2brain/beeep"

	"voice-to-clipboard/internal/logger"
)

const (
	// WindowClass is the X11/Hyprland window class identifier
	WindowClass = "voice-to-clipboard"
)

var (
	isHyprlandCached  *bool
	hyprlandCheckOnce sync.Once
)

// CopyToClipboard copies text to the system clipboard
func CopyToClipboard(text string) error {
	if err := clipboard.WriteAll(text); err != nil {
		logger.Error("Failed to copy to clipboard", "error", err)
		return err
	}
	logger.Debug("Copied to clipboard", "length", len(text))
	return nil
}

// ShowNotification displays a desktop notification
// Errors are logged but not returned since notifications are non-critical
func ShowNotification(title, message string) {
	if err := beeep.Notify(title, message, ""); err != nil {
		logger.Debug("Notification not shown", "error", err)
	}
}

// HideWindow hides the window using compositor-specific commands
func HideWindow(windowTitle string) error {
	// Try Hyprland first
	if isHyprland() {
		// Use class name instead of title for more reliable matching
		cmd := exec.Command("hyprctl", "dispatch", "movetoworkspacesilent", "special:hidden,class:"+WindowClass)
		if err := cmd.Run(); err != nil {
			logger.Debug("hyprctl hide failed", "error", err)
			return err
		}
		logger.Debug("Window hidden via hyprctl")
		return nil
	}

	// Fallback: try wmctrl for X11
	cmd := exec.Command("wmctrl", "-r", windowTitle, "-b", "add,hidden")
	if err := cmd.Run(); err != nil {
		logger.Debug("wmctrl hide not available", "error", err)
		return err
	}
	return nil
}

// ShowWindow shows the window using compositor-specific commands
func ShowWindow(windowTitle string) error {
	// Try Hyprland first
	if isHyprland() {
		classArg := "class:" + WindowClass

		// Move the window from special:hidden back to the active workspace.
		// Resolve the active workspace id explicitly — the "current" selector is
		// rejected ("Invalid workspace") by Hyprland 0.5x, which left the window
		// stuck hidden and made the later focuswindow misbehave (the window would
		// tile fullscreen or never reappear).
		if ws := activeWorkspaceID(); ws != "" {
			cmd := exec.Command("hyprctl", "dispatch", "movetoworkspacesilent", ws+","+classArg)
			if err := cmd.Run(); err != nil {
				logger.Debug("hyprctl movetoworkspacesilent failed", "error", err)
			}
		}

		// Force floating deterministically with setfloating (idempotent): a
		// frameless fixed-size window otherwise tiles and fills the screen.
		if err := exec.Command("hyprctl", "dispatch", "setfloating", classArg).Run(); err != nil {
			logger.Debug("hyprctl setfloating failed", "error", err)
		}

		// Focus the window (non-critical).
		if err := exec.Command("hyprctl", "dispatch", "focuswindow", classArg).Run(); err != nil {
			logger.Debug("hyprctl focuswindow failed", "error", err)
		}
		logger.Debug("Window shown via hyprctl")
		return nil
	}

	// Fallback: try wmctrl for X11
	cmd := exec.Command("wmctrl", "-r", windowTitle, "-b", "remove,hidden")
	if err := cmd.Run(); err != nil {
		logger.Debug("wmctrl show not available", "error", err)
		return err
	}
	// Try to activate/focus (non-critical)
	cmd = exec.Command("wmctrl", "-a", windowTitle)
	if err := cmd.Run(); err != nil {
		logger.Debug("wmctrl activate failed", "error", err)
	}
	return nil
}

// activeWorkspaceID returns the id of the currently active Hyprland workspace as
// a string, or "" if it can't be determined.
func activeWorkspaceID() string {
	out, err := exec.Command("hyprctl", "activeworkspace", "-j").Output()
	if err != nil {
		return ""
	}
	var ws struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(out, &ws); err != nil {
		return ""
	}
	return strconv.Itoa(ws.ID)
}

// EnsureFloating forces the app window to float on Hyprland. A frameless,
// fixed-size window otherwise tiles and fills the whole workspace (the
// "fullscreen overlay" bug). Idempotent and a no-op on other compositors, so it
// is safe to call repeatedly while the window is mapping at startup.
func EnsureFloating() {
	if !isHyprland() {
		return
	}
	cmd := exec.Command("hyprctl", "dispatch", "setfloating", "class:"+WindowClass)
	if err := cmd.Run(); err != nil {
		logger.Debug("hyprctl setfloating (startup) failed", "error", err)
	}
}

// isHyprland checks if running under Hyprland compositor (cached)
func isHyprland() bool {
	hyprlandCheckOnce.Do(func() {
		cmd := exec.Command("hyprctl", "version")
		output, err := cmd.Output()
		result := err == nil && strings.Contains(string(output), "Hyprland")
		isHyprlandCached = &result
	})
	return *isHyprlandCached
}
