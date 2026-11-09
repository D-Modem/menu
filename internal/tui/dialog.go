// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package tui

import (
	"strings"
	"unicode"
)

// Dialog kinds: the colors of a dialog.
const (
	Info  = iota // black on gray
	Alert        // white on red: an error, a question that loses something
)

// Message shows a dialog over base: a title, text (wrapped) and buttons
// (none: "OK").  The button chosen, -1 for Esc.  A button's first letter
// chooses it too.
func (d *Desk) Message(base *Screen, kind int, title, text string, buttons ...string) (int, error) {
	if len(buttons) == 0 {
		buttons = []string{"OK"}
	}
	frame, inside, btn, btnOn := Attr{White, LightGray}, Attr{Black, LightGray}, Attr{Black, Cyan}, Attr{White, Green}
	if kind == Alert {
		frame, inside, btn, btnOn = Attr{White, Red}, Attr{White, Red}, Attr{Black, LightGray}, Attr{White, Green}
	}
	bw := 0
	for _, b := range buttons {
		bw += Width(b) + 6
	}
	w := max(bw+4, Width(title)+10, 36)
	if Width(text) > 60 {
		w = max(w, 64)
	}
	w = min(w, d.Cols-4)
	lines := []string{}
	for _, para := range strings.Split(text, "\n") {
		lines = append(lines, Wrap(para, w-6)...)
	}
	for _, l := range lines {
		w = max(w, min(Width(l)+6, d.Cols-4))
	}
	maxLines := d.Rows - 8
	if len(lines) > maxLines {
		lines = append(lines[:maxLines-1], "…")
	}
	h := len(lines) + 5
	row := max(1, (d.Rows-h)/2)
	col := (d.Cols - w) / 2
	cur := 0
	for {
		scr := base.Clone()
		scr.Box(row, col, h, w, Double, frame, inside)
		scr.Shadow(row, col, h, w)
		scr.Title(row, col, w, title, frame)
		for i, l := range lines {
			scr.Put(row+2+i, col+3, l, inside)
		}
		x := col + (w-bw)/2 + 1
		for i, b := range buttons {
			a := btn
			if i == cur {
				a = btnOn
			}
			label := "  " + b + "  "
			scr.Put(row+h-2, x, label, a)
			if i == cur {
				scr.Put(row+h-2, x, "►", a)
			}
			x += Width(label) + 2
		}
		if err := d.Show(scr); err != nil {
			return -1, err
		}
		k, _, err := d.Key(-1)
		if err != nil {
			return -1, err
		}
		switch k {
		case KeyEsc:
			return -1, nil
		case KeyEnter, ' ':
			return cur, nil
		case KeyLeft, KeyUp:
			cur = (cur + len(buttons) - 1) % len(buttons)
		case KeyRight, KeyDown, KeyTab:
			cur = (cur + 1) % len(buttons)
		default:
			for i, b := range buttons {
				if k < KeyUp && strings.HasPrefix(strings.ToUpper(b), string(unicode.ToUpper(k))) {
					return i, nil
				}
			}
		}
	}
}

// Confirm asks a yes-or-no question: whether the answer is yes.
func (d *Desk) Confirm(base *Screen, kind int, title, text string) (bool, error) {
	i, err := d.Message(base, kind, title, text, "Yes", "No")
	return i == 0, err
}

// Input is a box over base to type a line in: prompt above it, value to
// begin with.  The line, and whether it was taken (Enter) or not (Esc).
// Mask hides what is typed.  Cursor keys, Home, End, Del, BS and ^U edit.
func (d *Desk) Input(base *Screen, title, prompt, value string, limit int, mask bool) (string, bool, error) {
	if limit <= 0 {
		limit = 255
	}
	w := min(d.Cols-4, 72)
	fw := w - 6 // the field's width
	plines := Wrap(prompt, w-6)
	if prompt == "" {
		plines = nil
	}
	plines = plines[:min(len(plines), 4)]
	h := 5 + len(plines)
	row := max(1, (d.Rows-h)/2-1)
	col := (d.Cols - w) / 2
	line := []rune(value)
	pos, off := len(line), 0
	frame, inside := Attr{White, Blue}, Attr{White, Blue}
	field := Attr{Black, Cyan}
	for {
		if pos < off {
			off = pos
		}
		if pos >= off+fw {
			off = pos - fw + 1
		}
		scr := base.Clone()
		scr.Box(row, col, h, w, Double, frame, inside)
		scr.Shadow(row, col, h, w)
		scr.Title(row, col, w, title, Attr{Yellow, Blue})
		for i, l := range plines {
			scr.Put(row+1+i, col+3, l, inside)
		}
		fr := row + 2 + len(plines)
		shown := line[off:min(len(line), off+fw)]
		s := string(shown)
		if mask {
			s = strings.Repeat("*", len(shown))
		}
		scr.PutW(fr, col+3, fw, s, field)
		if off > 0 {
			scr.Put(fr, col+2, "◄", Attr{Yellow, Blue})
		}
		if off+fw < len(line) {
			scr.Put(fr, col+3+fw, "►", Attr{Yellow, Blue})
		}
		d.KeyBar(scr, Hints("Enter", "Done", "Esc", "Cancel", "^U", "Clear", "←→", "Move"), "")
		if err := d.Show(scr); err != nil {
			return value, false, err
		}
		// the cursor where the next character goes, and shown
		cx := col + 3 + pos - off + 1
		if err := d.Line.WriteRaw("\x1b[" + itoa(fr+1) + ";" + itoa(cx) + "H\x1b[?25h"); err != nil {
			return value, false, err
		}
		k, _, err := d.Key(-1)
		if err != nil {
			return value, false, err
		}
		switch {
		case k == KeyEnter:
			return string(line), true, nil
		case k == KeyEsc:
			return value, false, nil
		case k == KeyLeft:
			pos = max(pos-1, 0)
		case k == KeyRight:
			pos = min(pos+1, len(line))
		case k == KeyHome || k == 0x01:
			pos = 0
		case k == KeyEnd || k == 0x05:
			pos = len(line)
		case k == KeyBackspace:
			if pos > 0 {
				line = append(line[:pos-1], line[pos:]...)
				pos--
			}
		case k == KeyDel || k == 0x04:
			if pos < len(line) {
				line = append(line[:pos], line[pos+1:]...)
			}
		case k == 0x15:
			line, pos, off = nil, 0, 0
		case k == 0x0b: // ^K: to the end
			line = line[:pos]
		case k >= ' ' && k < KeyUp && k != 0x7f && len(line) < limit:
			line = append(line[:pos], append([]rune{k}, line[pos:]...)...)
			pos++
		}
	}
}

// Choose is a list over base to pick one of choices from (cur: the one
// the bar starts on): its index, and whether one was picked.  notes are
// shown beside the choices (may be nil).
func (d *Desk) Choose(base *Screen, title string, choices, notes []string, cur int) (int, bool, error) {
	l := &List{Title: title, Center: true, Cur: cur}
	for i, c := range choices {
		e := Entry{Label: c}
		if i < len(notes) && notes[i] != "" {
			e.Value = notes[i]
			e.Unset = true
		}
		l.Items = append(l.Items, e)
	}
	l.Fix()
	for {
		scr := base.Clone()
		l.Draw(d, scr, true)
		d.KeyBar(scr, Hints("Enter", "Pick", "Esc", "Cancel", "↑↓", "Move"), "")
		if err := d.Show(scr); err != nil {
			return cur, false, err
		}
		k, _, err := d.Key(-1)
		if err != nil {
			return cur, false, err
		}
		switch k {
		case KeyEnter, ' ':
			return l.Cur, true, nil
		case KeyEsc, KeyLeft:
			return cur, false, nil
		default:
			l.Move(d, k)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
