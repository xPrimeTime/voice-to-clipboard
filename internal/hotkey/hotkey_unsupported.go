//go:build !windows

package hotkey

import "errors"

// No built-in global hotkey off Windows — see the package comment. The
// supported path is a system keybind running `voice-to-clipboard --toggle`.

func isSupported() bool {
	return false
}

func (h *Manager) start() error {
	return errors.New("built-in global hotkey not supported on this platform; bind 'voice-to-clipboard --toggle' to a system keybind")
}

func (h *Manager) stop() {}
