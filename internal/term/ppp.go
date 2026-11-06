// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package term

// IsPPPFrame: whether what starts with the flag 0x7E is a PPP frame: the
// address and control (0xFF 0x03, the 0x03 escaped as 0x7D 0x23), the
// address escaped (0x7D 0xDF), or both compressed away (LCP's 0xC0 0x21)
func IsPPPFrame(b []byte) bool {
	if len(b) < 3 || b[0] != 0x7E {
		return false
	}
	switch {
	case b[1] == 0xFF && (b[2] == 0x03 || b[2] == 0x7D):
		return true
	case b[1] == 0x7D && b[2] == 0xDF:
		return true
	case b[1] == 0xC0 && b[2] == 0x21:
		return true
	}
	return false
}
