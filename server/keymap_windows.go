package main

type keyDef struct {
	sc  uint16 // set-1 scancode
	ext bool   // extended (E0) key
	vk  uint16 // if set, send as virtual key instead of scancode
}

// keyMap maps DOM KeyboardEvent.code values to Windows scancodes. Scancodes are
// layout-independent, so the remote PC's keyboard layout decides the character.
var keyMap = map[string]keyDef{
	"Escape": {sc: 0x01}, "Digit1": {sc: 0x02}, "Digit2": {sc: 0x03}, "Digit3": {sc: 0x04},
	"Digit4": {sc: 0x05}, "Digit5": {sc: 0x06}, "Digit6": {sc: 0x07}, "Digit7": {sc: 0x08},
	"Digit8": {sc: 0x09}, "Digit9": {sc: 0x0A}, "Digit0": {sc: 0x0B}, "Minus": {sc: 0x0C},
	"Equal": {sc: 0x0D}, "Backspace": {sc: 0x0E}, "Tab": {sc: 0x0F},
	"KeyQ": {sc: 0x10}, "KeyW": {sc: 0x11}, "KeyE": {sc: 0x12}, "KeyR": {sc: 0x13}, "KeyT": {sc: 0x14},
	"KeyY": {sc: 0x15}, "KeyU": {sc: 0x16}, "KeyI": {sc: 0x17}, "KeyO": {sc: 0x18}, "KeyP": {sc: 0x19},
	"BracketLeft": {sc: 0x1A}, "BracketRight": {sc: 0x1B}, "Enter": {sc: 0x1C}, "ControlLeft": {sc: 0x1D},
	"KeyA": {sc: 0x1E}, "KeyS": {sc: 0x1F}, "KeyD": {sc: 0x20}, "KeyF": {sc: 0x21}, "KeyG": {sc: 0x22},
	"KeyH": {sc: 0x23}, "KeyJ": {sc: 0x24}, "KeyK": {sc: 0x25}, "KeyL": {sc: 0x26},
	"Semicolon": {sc: 0x27}, "Quote": {sc: 0x28}, "Backquote": {sc: 0x29}, "ShiftLeft": {sc: 0x2A},
	"Backslash": {sc: 0x2B}, "KeyZ": {sc: 0x2C}, "KeyX": {sc: 0x2D}, "KeyC": {sc: 0x2E}, "KeyV": {sc: 0x2F},
	"KeyB": {sc: 0x30}, "KeyN": {sc: 0x31}, "KeyM": {sc: 0x32}, "Comma": {sc: 0x33}, "Period": {sc: 0x34},
	"Slash": {sc: 0x35}, "ShiftRight": {sc: 0x36}, "NumpadMultiply": {sc: 0x37}, "AltLeft": {sc: 0x38},
	"Space": {sc: 0x39}, "CapsLock": {sc: 0x3A},
	"F1": {sc: 0x3B}, "F2": {sc: 0x3C}, "F3": {sc: 0x3D}, "F4": {sc: 0x3E}, "F5": {sc: 0x3F},
	"F6": {sc: 0x40}, "F7": {sc: 0x41}, "F8": {sc: 0x42}, "F9": {sc: 0x43}, "F10": {sc: 0x44},
	"F11": {sc: 0x57}, "F12": {sc: 0x58},
	"ScrollLock": {sc: 0x46}, "Numpad7": {sc: 0x47}, "Numpad8": {sc: 0x48}, "Numpad9": {sc: 0x49},
	"NumpadSubtract": {sc: 0x4A}, "Numpad4": {sc: 0x4B}, "Numpad5": {sc: 0x4C}, "Numpad6": {sc: 0x4D},
	"NumpadAdd": {sc: 0x4E}, "Numpad1": {sc: 0x4F}, "Numpad2": {sc: 0x50}, "Numpad3": {sc: 0x51},
	"Numpad0": {sc: 0x52}, "NumpadDecimal": {sc: 0x53}, "IntlBackslash": {sc: 0x56},
	"NumpadEnter": {sc: 0x1C, ext: true}, "ControlRight": {sc: 0x1D, ext: true},
	"NumpadDivide": {sc: 0x35, ext: true}, "PrintScreen": {sc: 0x37, ext: true},
	"AltRight": {sc: 0x38, ext: true}, "Home": {sc: 0x47, ext: true}, "ArrowUp": {sc: 0x48, ext: true},
	"PageUp": {sc: 0x49, ext: true}, "ArrowLeft": {sc: 0x4B, ext: true}, "ArrowRight": {sc: 0x4D, ext: true},
	"End": {sc: 0x4F, ext: true}, "ArrowDown": {sc: 0x50, ext: true}, "PageDown": {sc: 0x51, ext: true},
	"Insert": {sc: 0x52, ext: true}, "Delete": {sc: 0x53, ext: true}, "MetaLeft": {sc: 0x5B, ext: true},
	"MetaRight": {sc: 0x5C, ext: true}, "ContextMenu": {sc: 0x5D, ext: true},
	// keys that behave better as virtual keys
	"NumLock": {vk: 0x90, sc: 0x45, ext: true}, "Pause": {vk: 0x13, sc: 0x45},
	"AudioVolumeMute": {vk: 0xAD, ext: true}, "AudioVolumeDown": {vk: 0xAE, ext: true},
	"AudioVolumeUp": {vk: 0xAF, ext: true}, "MediaTrackNext": {vk: 0xB0, ext: true},
	"MediaTrackPrevious": {vk: 0xB1, ext: true}, "MediaStop": {vk: 0xB2, ext: true},
	"MediaPlayPause": {vk: 0xB3, ext: true}, "BrowserBack": {vk: 0xA6, ext: true},
	"BrowserForward": {vk: 0xA7, ext: true},
}
