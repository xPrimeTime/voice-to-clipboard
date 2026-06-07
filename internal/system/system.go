package system

import (
	"os/exec"
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
		// Move from special workspace to current workspace using class name
		classArg := "class:" + WindowClass
		cmd := exec.Command("hyprctl", "dispatch", "movetoworkspace", "current,"+classArg)
		if err := cmd.Run(); err != nil {
			logger.Debug("hyprctl show failed", "error", err)
			return err
		}

		// Moving a window out of the special workspace can drop its floating
		// state. Force it floating deterministically with setfloating: a blind
		// togglefloating would tile an already-floating window, which on Hyprland
		// expands to fill the whole screen (the "fullscreen overlay" bug).
		cmd = exec.Command("hyprctl", "dispatch", "setfloating", classArg)
		if err := cmd.Run(); err != nil {
			logger.Debug("hyprctl setfloating failed", "error", err)
		}

		// Focus the window (non-critical, log but don't fail)
		cmd = exec.Command("hyprctl", "dispatch", "focuswindow", classArg)
		if err := cmd.Run(); err != nil {
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
