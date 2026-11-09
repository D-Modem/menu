// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package tui

import (
	"strings"
	"unicode"
)

// Entry is a line of a List: a label and its value, or a heading.
type Entry struct {
	ID      string
	Key     string // shown before the label, as [B]; typing it picks the entry
	Label   string
	Value   string // at the right of the labels
	Unset   bool   // Value is a default, not set: shown dim
	Note    string // after the value, dim
	Help    string // in the help box while the bar is on it
	Mark    string // at the window's right edge: ► a window, ↕ a choice
	Heading bool   // a line across the window, Label in it
	Off     bool   // shown, not chosen
	Changed bool   // a * before it
}

// choosable is whether the bar may rest on e.
func (e *Entry) choosable() bool { return !e.Heading && !e.Off }

// List is a window of entries with a bar on one of them.
type List struct {
	Title string
	Notes []string // lines above the entries (three at most)
	Items []Entry
	Cur   int
	top   int
	// where it goes: Depth cascades it down and right of the first window
	// (0); Center puts it in the middle of the desk
	Depth  int
	Center bool
	MinW   int
}

// colors of a window, as it is up or under another
type winColors struct {
	frame, title, label, value, unset, note, mark, off, head, key Attr
	bar, barValue, barKey                                         Attr
	box                                                           BoxStyle
}

var (
	activeWin = winColors{
		frame: Attr{White, Blue}, title: Attr{Yellow, Blue}, label: Attr{White, Blue},
		value: Attr{Yellow, Blue}, unset: Attr{LightGray, Blue}, note: Attr{Cyan, Blue},
		mark: Attr{LightRed, Blue}, off: Attr{DarkGray, Blue}, head: Attr{LightCyan, Blue},
		key: Attr{LightCyan, Blue}, bar: Attr{White, Cyan}, barValue: Attr{Yellow, Cyan},
		barKey: Attr{Yellow, Cyan}, box: Double,
	}
	inactiveWin = winColors{
		frame: Attr{LightGray, Blue}, title: Attr{LightGray, Blue}, label: Attr{LightGray, Blue},
		value: Attr{LightGray, Blue}, unset: Attr{LightGray, Blue}, note: Attr{LightGray, Blue},
		mark: Attr{LightGray, Blue}, off: Attr{DarkGray, Blue}, head: Attr{LightGray, Blue},
		key: Attr{LightGray, Blue}, bar: Attr{Black, LightGray}, barValue: Attr{Black, LightGray},
		barKey: Attr{Black, LightGray}, box: Single,
	}
)

// geometry is a window's place: top row and left column, height and width
// (frame and all), and the rows of notes and of entries.
type geometry struct{ row, col, h, w, notes, list int }

// widths: the labels' column (with keys) and the values'.
func (l *List) widths() (kw, lw, vw int) {
	for _, e := range l.Items {
		if e.Heading {
			lw = max(lw, Width(e.Label)+4-12)
			continue
		}
		if e.Key != "" {
			kw = max(kw, Width(e.Key)+3)
		}
		lw = max(lw, Width(e.Label))
		v := e.Value
		if e.Note != "" {
			v += "  " + e.Note
		}
		vw = max(vw, Width(v))
	}
	return
}

func (l *List) geom(d *Desk) geometry {
	first, last := 2, d.Bottom()
	kw, lw, vw := l.widths()
	for _, n := range l.Notes {
		vw = max(vw, Width(n)-lw-kw-4)
	}
	maxW := d.Cols - 4
	width := 2 + 2 + kw + lw + 3
	if vw > 0 {
		width += 2 + min(max(vw, 12), 40)
	}
	width = max(width, Width(l.Title)+10, min(l.MinW, maxW), 30)
	width = min(width, maxW)
	off := min(l.Depth, 5)
	g := geometry{w: width, col: 2 + 3*off, row: first + off}
	g.notes = min(len(l.Notes), 3)
	if g.notes > 0 {
		g.notes++ // the rule under them
	}
	g.list = max(len(l.Items), 1)
	g.h = 2 + g.notes + g.list
	if l.Center {
		g.col = (d.Cols - g.w) / 2
		g.row = max(first, first+(last-first+1-g.h)/2)
	}
	if g.col+g.w+2 > d.Cols {
		g.col = max(d.Cols-2-g.w, 0)
	}
	room := last - g.row + 1
	if g.h > room {
		if room < 2+g.notes+5 { // too far down: up as far as needed
			g.row = max(first, last+1-(2+g.notes+5))
			room = last - g.row + 1
		}
		g.h = min(g.h, room)
		g.list = max(g.h-2-g.notes, 1)
	}
	return g
}

// Fix puts the bar on an entry it may rest on (the first after Cur).
func (l *List) Fix() {
	if l.Cur >= 0 && l.Cur < len(l.Items) && l.Items[l.Cur].choosable() {
		return
	}
	if j := l.step(l.Cur, 1); j >= 0 {
		l.Cur = j
	} else if j := l.step(l.Cur, -1); j >= 0 {
		l.Cur = j
	}
}

// step is the next entry the bar may rest on from i, by dir (-1 if none).
func (l *List) step(i, dir int) int {
	for j := i + dir; j >= 0 && j < len(l.Items); j += dir {
		if l.Items[j].choosable() {
			return j
		}
	}
	return -1
}

// Selected is the entry under the bar (nil: none).
func (l *List) Selected() *Entry {
	if l.Cur < 0 || l.Cur >= len(l.Items) || !l.Items[l.Cur].choosable() {
		return nil
	}
	return &l.Items[l.Cur]
}

// Move handles the keys that move the bar: whether k was one.  A letter
// moves to the next entry it begins (or whose Key it is, picked: true).
func (l *List) Move(d *Desk, k rune) (moved, picked bool) {
	g := l.geom(d)
	to := func(j int) {
		if j >= 0 {
			l.Cur = j
		}
	}
	switch k {
	case KeyUp:
		to(l.step(l.Cur, -1))
	case KeyDown, KeyTab:
		to(l.step(l.Cur, 1))
	case KeyHome:
		to(l.step(-1, 1))
	case KeyEnd:
		to(l.step(len(l.Items), -1))
	case KeyPgUp, KeyPgDn:
		dir := 1
		if k == KeyPgUp {
			dir = -1
		}
		j := min(max(l.Cur+dir*(g.list-1), 0), len(l.Items)-1)
		if n := l.step(j-dir, dir); n >= 0 {
			to(n)
		} else {
			to(l.step(j, -dir))
		}
	default:
		if k < ' ' || k >= KeyUp || k == ' ' {
			return false, false
		}
		u := strings.ToUpper(string(k))
		for i, e := range l.Items {
			if e.choosable() && e.Key != "" && strings.ToUpper(e.Key) == u {
				l.Cur = i
				return true, true
			}
		}
		n := len(l.Items)
		for o := 1; o <= n; o++ {
			j := (l.Cur + o + n) % n
			e := l.Items[j]
			if e.choosable() && strings.HasPrefix(strings.ToUpper(e.Label), u) {
				l.Cur = j
				return true, false
			}
		}
		return false, false
	}
	return true, false
}

// Draw draws the window on scr, active (on top: a double frame, the bar
// bright) or under another.
func (l *List) Draw(d *Desk, scr *Screen, active bool) {
	c := inactiveWin
	if active {
		c = activeWin
	}
	g := l.geom(d)
	scr.Box(g.row, g.col, g.h, g.w, c.box, c.frame, c.label)
	scr.Shadow(g.row, g.col, g.h, g.w)
	if l.Title != "" {
		scr.Title(g.row, g.col, g.w, l.Title, c.title)
	}
	inner := g.w - 2
	for i := 0; i < min(len(l.Notes), 3); i++ {
		scr.PutW(g.row+1+i, g.col+1, inner, " "+Trunc(l.Notes[i], inner-2), c.note)
	}
	if g.notes > 0 {
		scr.Rule(g.row+g.notes, g.col, g.w, c.box, c.frame)
	}
	cur := l.Cur
	if cur >= 0 && cur < l.top {
		l.top = cur
		if l.top > 0 && l.Items[l.top-1].Heading {
			l.top-- // a heading just above comes too
		}
	}
	if cur >= l.top+g.list {
		l.top = cur - g.list + 1
	}
	l.top = max(min(l.top, len(l.Items)-g.list), 0)
	kw, lw, vmax := l.widths()
	_ = vmax
	if vmax > 0 {
		lw = min(lw, max(inner*11/20-kw, inner-6-kw-max(vmax, 8)))
	} else {
		lw = inner - 4 - kw
	}
	vw := inner - 2 - kw - lw - 2 - 2
	first := g.row + 1 + g.notes
	for r := 0; r < g.list; r++ {
		i := l.top + r
		row := first + r
		if i >= len(l.Items) {
			break
		}
		e := l.Items[i]
		if e.Heading {
			scr.Put(row, g.col+1, strings.Repeat("─", inner), c.off)
			if e.Label != "" {
				scr.Put(row, g.col+2, " "+Trunc(e.Label, inner-4)+" ", c.head)
			}
			continue
		}
		label, value, unset, note, mark, key := c.label, c.value, c.unset, c.note, c.mark, c.key
		if e.Off {
			label, value, unset, note, key = c.off, c.off, c.off, c.off, c.off
		}
		if i == cur && active || i == cur && !e.Off {
			scr.Fill(row, g.col+1, 1, inner, ' ', c.bar)
			label, unset, note, mark, value, key = c.bar, c.bar, c.bar, c.barValue, c.barValue, c.barKey
		}
		if e.Changed {
			scr.Put(row, g.col+1, "*", mark)
		}
		x := g.col + 2
		if kw > 0 {
			if e.Key != "" {
				scr.Put(row, x, "[", label)
				scr.Put(row, x+1, e.Key, key)
				scr.Put(row, x+1+Width(e.Key), "]", label)
			}
			x += kw
		}
		scr.PutW(row, x, lw, Trunc(e.Label, lw), label)
		if vw > 0 {
			va := value
			if e.Unset {
				va = unset
			}
			x += lw + 2
			n := scr.Put(row, x, Trunc(e.Value, vw), va)
			if e.Note != "" && n+3 < vw && vw >= 20 {
				scr.Put(row, x+n+2, Trunc(e.Note, vw-n-2), note)
			}
		}
		if e.Mark != "" {
			scr.Put(row, g.col+g.w-3, e.Mark, value)
		}
	}
	if l.top > 0 {
		scr.Put(first, g.col+g.w-1, "▲", c.frame)
	}
	if l.top+g.list < len(l.Items) {
		scr.Put(first+g.list-1, g.col+g.w-1, "▼", c.frame)
	}
}

// capFirst is s with its first letter a capital.
func capFirst(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}
