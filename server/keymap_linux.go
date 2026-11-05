package main

// linKeys maps DOM KeyboardEvent.code to Linux evdev KEY_* codes (layout-independent positions).
var linKeys = map[string]int{
	"Escape": 1, "Digit1": 2, "Digit2": 3, "Digit3": 4, "Digit4": 5, "Digit5": 6, "Digit6": 7,
	"Digit7": 8, "Digit8": 9, "Digit9": 10, "Digit0": 11, "Minus": 12, "Equal": 13, "Backspace": 14,
	"Tab": 15, "KeyQ": 16, "KeyW": 17, "KeyE": 18, "KeyR": 19, "KeyT": 20, "KeyY": 21, "KeyU": 22,
	"KeyI": 23, "KeyO": 24, "KeyP": 25, "BracketLeft": 26, "BracketRight": 27, "Enter": 28,
	"ControlLeft": 29, "KeyA": 30, "KeyS": 31, "KeyD": 32, "KeyF": 33, "KeyG": 34, "KeyH": 35,
	"KeyJ": 36, "KeyK": 37, "KeyL": 38, "Semicolon": 39, "Quote": 40, "Backquote": 41,
	"ShiftLeft": 42, "Backslash": 43, "KeyZ": 44, "KeyX": 45, "KeyC": 46, "KeyV": 47, "KeyB": 48,
	"KeyN": 49, "KeyM": 50, "Comma": 51, "Period": 52, "Slash": 53, "ShiftRight": 54,
	"NumpadMultiply": 55, "AltLeft": 56, "Space": 57, "CapsLock": 58,
	"F1": 59, "F2": 60, "F3": 61, "F4": 62, "F5": 63, "F6": 64, "F7": 65, "F8": 66, "F9": 67, "F10": 68,
	"NumLock": 69, "ScrollLock": 70, "Numpad7": 71, "Numpad8": 72, "Numpad9": 73, "NumpadSubtract": 74,
	"Numpad4": 75, "Numpad5": 76, "Numpad6": 77, "NumpadAdd": 78, "Numpad1": 79, "Numpad2": 80,
	"Numpad3": 81, "Numpad0": 82, "NumpadDecimal": 83, "IntlBackslash": 86, "F11": 87, "F12": 88,
	"NumpadEnter": 96, "ControlRight": 97, "NumpadDivide": 98, "PrintScreen": 99, "AltRight": 100,
	"Home": 102, "ArrowUp": 103, "PageUp": 104, "ArrowLeft": 105, "ArrowRight": 106, "End": 107,
	"ArrowDown": 108, "PageDown": 109, "Insert": 110, "Delete": 111, "AudioVolumeMute": 113,
	"AudioVolumeDown": 114, "AudioVolumeUp": 115, "Pause": 119, "MetaLeft": 125, "MetaRight": 126,
	"ContextMenu": 127, "BrowserBack": 158, "BrowserForward": 159, "MediaTrackNext": 163,
	"MediaPlayPause": 164, "MediaTrackPrevious": 165, "MediaStop": 166,
}

type asciiKey struct {
	code  int
	shift bool
}

// asciiKeys: characters on a US layout → key + shift.
var asciiKeys = func() map[rune]asciiKey {
	m := map[rune]asciiKey{' ': {57, false}, '\n': {28, false}, '\t': {15, false}}
	lower := "abcdefghijklmnopqrstuvwxyz"
	codes := []int{30, 48, 46, 32, 18, 33, 34, 35, 23, 36, 37, 38, 50, 49, 24, 25, 16, 19, 31, 20, 22, 47, 17, 45, 21, 44}
	for i, c := range lower {
		m[c] = asciiKey{codes[i], false}
		m[c-32] = asciiKey{codes[i], true}
	}
	digits := "1234567890"
	shifted := "!@#$%^&*()"
	for i, c := range digits {
		m[c] = asciiKey{2 + i, false}
		m[rune(shifted[i])] = asciiKey{2 + i, true}
	}
	pairs := []struct {
		plain, shift rune
		code         int
	}{{'-', '_', 12}, {'=', '+', 13}, {'[', '{', 26}, {']', '}', 27}, {';', ':', 39}, {'\'', '"', 40},
		{'`', '~', 41}, {'\\', '|', 43}, {',', '<', 51}, {'.', '>', 52}, {'/', '?', 53}}
	for _, p := range pairs {
		m[p.plain] = asciiKey{p.code, false}
		m[p.shift] = asciiKey{p.code, true}
	}
	return m
}()
