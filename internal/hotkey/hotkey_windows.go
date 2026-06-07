//go:build windows

package hotkey

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
