// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package tui

import (
	"errors"
	"time"
	"unicode/utf8"

	"github.com/D-Modem/menu/internal/term"
)

// Keys that are no character, as runes past Unicode's private area.
const (
	KeyUp rune = 0x10ff00 + iota
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPgUp
	KeyPgDn
	KeyIns
	KeyDel
	KeyF1 // F1 to F12: KeyF1+0 to KeyF1+11
)

const (
	KeyEnter     rune = '\r'
	KeyEsc       rune = 0x1b
	KeyTab       rune = '\t'
	KeyBackspace rune = 0x08
)

// F is function key n (1-12).
func F(n int) rune { return KeyF1 + rune(n-1) }

// ErrTimeout: no key came in time (the line's own).
var ErrTimeout = term.ErrTimeout

// escWait is how long the rest of an escape sequence may take after its
// ESC: a byte at 300 bit/s takes 33 ms, and modems send in bursts.
const escWait = 400 * time.Millisecond

// a VT's ESC [ n ~ keys
var tildeKeys = map[int]rune{1: KeyHome, 2: KeyIns, 3: KeyDel, 4: KeyEnd, 5: KeyPgUp, 6: KeyPgDn,
	7: KeyHome, 8: KeyEnd, 11: F(1), 12: F(2), 13: F(3), 14: F(4), 15: F(5), 17: F(6), 18: F(7),
	19: F(8), 20: F(9), 21: F(10), 23: F(11), 24: F(12)}

// readKey is a key from read (which returns ErrTimeout itself, or any
// error); alt: a letter typed with Alt (ESC and it at once).
func (d *Desk) readKey(wait time.Duration) (k rune, alt bool, err error) {
	for {
		b, err := d.next(wait)
		if err != nil {
			return 0, false, err
		}
		switch {
		case b == '\n' && d.lastCR:
			d.lastCR = false
			continue
		case b == '\r' || b == '\n':
			d.lastCR = b == '\r'
			return KeyEnter, false, nil
		case b == 0x7f || b == 0x08:
			d.lastCR = false
			return KeyBackspace, false, nil
		case b == 0x1b:
			d.lastCR = false
			return d.escape()
		case b >= 0xc0 && d.Charset == UTF8:
			d.lastCR = false
			buf := []byte{b}
			for len(buf) < 4 && !utf8.FullRune(buf) {
				c, err := d.next(escWait)
				if err != nil {
					break
				}
				buf = append(buf, c)
			}
			r, _ := utf8.DecodeRune(buf)
			return r, false, nil
		case b >= 0x80:
			continue // not a key the screens take
		}
		d.lastCR = false
		return rune(b), false, nil
	}
}

// escape is the key whose ESC came.
func (d *Desk) escape() (rune, bool, error) {
	b, err := d.next(escWait)
	if errors.Is(err, ErrTimeout) {
		return KeyEsc, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	switch b {
	case '[':
	case 'O':
		c, err := d.next(escWait)
		if err != nil {
			return KeyEsc, false, nil
		}
		switch c {
		case 'A', 'B', 'C', 'D', 'H', 'F':
			return cursorKey(c), false, nil
		case 'P', 'Q', 'R', 'S':
			return F(int(c-'P') + 1), false, nil
		}
		return KeyEsc, false, nil
	case 0x1b:
		d.unread(b)
		return KeyEsc, false, nil
	default:
		if b >= 'a' && b <= 'z' {
			b -= 'a' - 'A'
		}
		if b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' {
			return rune(b), true, nil
		}
		d.unread(b) // a key of its own, after Esc
		return KeyEsc, false, nil
	}
	// CSI: parameters, then the final byte
	n, params := 0, 0
	for i := 0; i < 16; i++ {
		c, err := d.next(escWait)
		if err != nil {
			return KeyEsc, false, nil
		}
		switch {
		case c >= '0' && c <= '9':
			if params == 0 {
				n = n*10 + int(c-'0')
			}
		case c == ';':
			params++
		case c == '~':
			if k, ok := tildeKeys[n]; ok {
				return k, false, nil
			}
			return KeyEsc, false, nil
		case c >= 0x40 && c <= 0x7e:
			if k := cursorKey(c); k != 0 {
				return k, false, nil
			}
			return KeyEsc, false, nil
		}
	}
	return KeyEsc, false, nil
}

// cursorKey is ESC [ c's key (0: none).
func cursorKey(c byte) rune {
	switch c {
	case 'A':
		return KeyUp
	case 'B':
		return KeyDown
	case 'C':
		return KeyRight
	case 'D':
		return KeyLeft
	case 'H':
		return KeyHome
	case 'F':
		return KeyEnd
	case 'P', 'Q', 'R', 'S':
		return F(int(c-'P') + 1)
	}
	return 0
}

// next is the next byte (what was put back first).
func (d *Desk) next(wait time.Duration) (byte, error) {
	if len(d.pending) > 0 {
		b := d.pending[0]
		d.pending = d.pending[1:]
		return b, nil
	}
	if wait < 0 {
		wait = d.Idle
		if wait == 0 {
			wait = 24 * time.Hour
		}
	}
	forever := wait < 0 // Idle < 0: until a key, a hangup or the Deadline
	for {
		w := wait
		if forever {
			w = 24 * time.Hour
		}
		if !d.Deadline.IsZero() {
			left := time.Until(d.Deadline)
			if left <= 0 {
				return 0, ErrTimeout
			}
			w = min(w, left)
		}
		if w == 0 {
			return 0, ErrTimeout
		}
		b, err := d.Line.ReadOne(w)
		if forever && errors.Is(err, ErrTimeout) {
			continue
		}
		return b, err
	}
}

func (d *Desk) unread(b byte) { d.pending = append([]byte{b}, d.pending...) }
