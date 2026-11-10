// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package tui

import (
	"strings"
	"time"
	"unicode"
)

// Line is the terminal: one byte at a time, within a time, and bytes out.
type Line interface {
	ReadOne(time.Duration) (byte, error)
	WriteRaw(string) error
}

// Desk is the full screen on a terminal: what it shows now, and how.
type Desk struct {
	Line    Line
	Rows    int
	Cols    int
	Charset Charset
	Mono    bool   // no colors: bright as bold, light bars reversed (a VT)
	Name    string // at the right of the menu bar
	// the desk shaded (░), not plain blue: plain is cleared in a few
	// bytes, for a slow line
	Shade bool
	Menus []Menu // the menu bar's pull-downs; none: a bar with Name
	// how long a key may take where the screens wait without a limit of
	// their own (0: a day, < 0: no limit); ErrTimeout after
	Idle time.Duration
	// a time no wait goes past (zero: none); ErrTimeout at it
	Deadline time.Time

	on      *Screen // what the terminal shows (nil: not known)
	last    *Screen // what was shown last
	pending []byte
	lastCR  bool
}

// Menu is a pull-down menu of the menu bar.
type Menu struct {
	Label string
	Items []MenuItem
}

// MenuItem is a menu's line: an ID it gives back, or a heading line.
type MenuItem struct {
	Label   string
	ID      string
	Heading bool
}

// Hint is a key on the bar at the foot, and what it does.
type Hint struct{ Key, Label string }

// OK is whether a terminal of rows and cols has room for the desk.
func OK(rows, cols int) bool { return rows >= 14 && cols >= 40 }

// Show sends what changed between the terminal's screen and scr.
func (d *Desk) Show(scr *Screen) error {
	old, out := d.on, ""
	if old == nil || old.Cols != scr.Cols || old.Rows != scr.Rows {
		// the screen cleared in the desk's colors: its blank cells need
		// nothing more
		out = "\x1b[0m" + sgr(Attr{LightGray, Blue}, d.Mono) + "\x1b[2J"
		old = NewScreen(scr.Cols, scr.Rows)
		old.Fill(0, 0, scr.Rows, scr.Cols, ' ', Attr{LightGray, Blue})
	}
	out += scr.Render(old, d.Mono) + "\x1b[?25l"
	d.on = scr.Clone()
	d.last = d.on
	return d.Line.WriteRaw(string(Encode(out, d.Charset)))
}

// Redraw: the next Show sends the whole screen (the terminal's is not
// known: a program ran on it, or the caller asked with ^L).
func (d *Desk) Redraw() { d.on = nil }

// Close takes the desk down: colors reset, the screen cleared, the cursor
// shown at the top.
func (d *Desk) Close() error {
	d.on = nil
	return d.Line.WriteRaw("\x1b[0m\x1b[2J\x1b[H\x1b[?25h")
}

// Key is the next key within wait (< 0: Idle; ErrTimeout without one); alt: a letter
// or digit typed with Alt.  ^L redraws the screen at the next Show and is
// given back too.
func (d *Desk) Key(wait time.Duration) (rune, bool, error) {
	k, alt, err := d.readKey(wait)
	if err == nil && k == 0x0c {
		d.Redraw()
	}
	return k, alt, err
}

// Blank is the bare desk: the shading, the menu bar (open: the menu shown
// pulled down, -1 none), the key bar's row empty.
func (d *Desk) Blank(open int) *Screen {
	scr := NewScreen(d.Cols, d.Rows)
	if d.Shade {
		scr.Fill(1, 0, d.Rows-2, d.Cols, '░', Attr{LightGray, Blue})
	} else {
		scr.Fill(1, 0, d.Rows-2, d.Cols, ' ', Attr{LightGray, Blue})
	}
	bar := Attr{Black, LightGray}
	scr.Fill(0, 0, 1, d.Cols, ' ', bar)
	scr.Fill(d.Rows-1, 0, 1, d.Cols, ' ', bar)
	bm := barMenus(d.Menus)
	for i, m := range bm {
		a, hot := bar, Attr{Red, LightGray}
		if i == open {
			a, hot = Attr{Black, Green}, Attr{Red, Green}
		}
		scr.Put(0, m.col-1, " "+m.label+" ", a)
		if m.hot >= 0 {
			scr.Put(0, m.col+m.hot, string([]rune(m.label)[m.hot]), hot)
		}
	}
	end := 0
	if len(bm) > 0 {
		l := bm[len(bm)-1]
		end = l.col + Width(l.label) + 1
	}
	if name := " " + d.Name + " "; d.Name != "" && end+Width(name)+2 < d.Cols {
		if len(bm) == 0 {
			scr.Put(0, 1, name, bar)
		} else {
			scr.Put(0, d.Cols-Width(name)-1, name, bar)
		}
	}
	return scr
}

// Status puts s at the right of the menu bar (over Name).
func (d *Desk) Status(scr *Screen, s string) {
	if s == "" {
		return
	}
	s = " " + Trunc(s, d.Cols/2) + " "
	scr.Put(0, d.Cols-Width(s)-1, s, Attr{Black, LightGray})
}

// KeyBar draws the bar of keys at the foot, and status at its right.
func (d *Desk) KeyBar(scr *Screen, keys []Hint, status string) {
	cols, row := d.Cols, d.Rows-1
	bar, hot := Attr{Black, LightGray}, Attr{Red, LightGray}
	scr.Fill(row, 0, 1, cols, ' ', bar)
	room := cols - 1
	if status != "" {
		room -= min(Width(status)+3, cols/2)
	}
	x := 1
	for _, k := range keys {
		n := Width(k.Key) + 1 + Width(k.Label)
		if x+n > room {
			break
		}
		scr.Put(row, x, k.Key, hot)
		scr.Put(row, x+Width(k.Key)+1, k.Label, bar)
		x += n + 2
	}
	if status != "" {
		st := Trunc(status, cols/2-2)
		scr.Put(row, cols-Width(st)-2, st, Attr{Blue, LightGray})
	}
}

// HasHelp is whether the screen has rows for the help box.
func (d *Desk) HasHelp() bool { return d.Rows >= 20 }

// Bottom is the last row a window may use (above the help box, if there
// is one, and the window's shadow).
func (d *Desk) Bottom() int {
	if d.HasHelp() {
		return d.Rows - 7
	}
	return d.Rows - 3
}

// HelpBox draws the box of help above the key bar: a title and text (two
// lines of it; more is cut).  tone: 0 plain, 1 a warning.
func (d *Desk) HelpBox(scr *Screen, title, text string, warn bool) {
	if !d.HasHelp() {
		return
	}
	row, cols := d.Rows-5, d.Cols
	scr.Box(row, 0, 4, cols, Single, Attr{LightCyan, Blue}, Attr{White, Blue})
	if title != "" {
		scr.Title(row, 0, cols, title, Attr{Yellow, Blue})
	}
	a := Attr{White, Blue}
	if warn {
		a = Attr{LightRed, Blue}
	}
	lines := Wrap(text, cols-4)
	if len(lines) > 2 {
		lines[1] = Trunc(lines[1], cols-14) + "  (F1: more)"
		lines = lines[:2]
	}
	for i, l := range lines {
		scr.Put(row+1+i, 2, l, a)
	}
}

// barMenu is a menu's place on the bar.
type barMenu struct {
	label string
	col   int // where its label begins
	hot   int // its hot key's rune in label (-1: none)
	key   rune
}

// barMenus are the bar's menus placed, each with a hot key no other has.
func barMenus(menus []Menu) []barMenu {
	var out []barMenu
	used := map[rune]bool{}
	col := 3
	for _, m := range menus {
		b := barMenu{label: m.Label, col: col, hot: -1}
		for i, r := range []rune(m.Label) {
			u := unicode.ToUpper(r)
			if unicode.IsLetter(r) && !used[u] {
				b.hot, b.key, used[u] = i, u, true
				break
			}
		}
		out = append(out, b)
		col += Width(m.Label) + 2
	}
	return out
}

// MenuKey is the menu whose hot key (with Alt) k is; -1 none.
func (d *Desk) MenuKey(k rune) int {
	for i, m := range barMenus(d.Menus) {
		if m.key == unicode.ToUpper(k) {
			return i
		}
	}
	return -1
}

// PullDown shows the menu bar's menu i pulled down over base (the screen
// with that menu open: base(i)), until an item is picked (its ID) or the
// menus are left ("").
func (d *Desk) PullDown(base func(open int) *Screen, i int) (string, error) {
	bm := barMenus(d.Menus)
	if i < 0 || i >= len(bm) {
		return "", nil
	}
	first := func(m Menu) int {
		for j, it := range m.Items {
			if !it.Heading {
				return j
			}
		}
		return -1
	}
	cur, top := first(d.Menus[i]), 0
	for {
		scr := base(i)
		m := d.Menus[i]
		width := 14
		for _, it := range m.Items {
			width = max(width, Width(it.Label)+4)
		}
		width = min(width, d.Cols-2)
		col := max(min(bm[i].col-2, d.Cols-width-2), 0)
		n := min(len(m.Items), d.Rows-6)
		if cur >= 0 {
			top = min(top, cur)
			if cur >= top+n {
				top = cur - n + 1
			}
		}
		box := Attr{Black, LightGray}
		scr.Box(1, col, n+2, width, Single, box, box)
		scr.Shadow(1, col, n+2, width)
		for r := 0; r < n; r++ {
			j := top + r
			it := m.Items[j]
			row := 2 + r
			if it.Heading {
				scr.Rule(row, col, width, Single, box)
				if it.Label != "" {
					scr.Put(row, col+2, " "+Trunc(it.Label, width-6)+" ", Attr{DarkGray, LightGray})
				}
				continue
			}
			a := box
			if j == cur {
				a = Attr{Black, Green}
			}
			scr.PutW(row, col+1, width-2, " "+Trunc(it.Label, width-3), a)
		}
		if top > 0 {
			scr.Put(2, col+width-1, "▲", box)
		}
		if top+n < len(m.Items) {
			scr.Put(1+n, col+width-1, "▼", box)
		}
		d.KeyBar(scr, Hints("Enter", "Select", "Esc", "Close", "←→", "Menus", "↑↓", "Items"), "")
		if err := d.Show(scr); err != nil {
			return "", err
		}
		k, alt, err := d.Key(-1)
		if err != nil {
			return "", err
		}
		move := func(dir int) {
			for j := cur + dir; j >= 0 && j < len(m.Items); j += dir {
				if !m.Items[j].Heading {
					cur = j
					return
				}
			}
		}
		switch {
		case alt:
			if j := d.MenuKey(k); j >= 0 {
				i, cur, top = j, first(d.Menus[j]), 0
			}
		case k == KeyEsc || k == '/' || k == F(12) || k == F(10):
			return "", nil
		case k == KeyLeft:
			i = (i + len(d.Menus) - 1) % len(d.Menus)
			cur, top = first(d.Menus[i]), 0
		case k == KeyRight:
			i = (i + 1) % len(d.Menus)
			cur, top = first(d.Menus[i]), 0
		case k == KeyUp:
			move(-1)
		case k == KeyDown || k == KeyTab:
			move(1)
		case k == KeyHome:
			cur = -1
			move(1)
		case k == KeyEnd:
			cur = len(m.Items)
			move(-1)
		case k == KeyEnter || k == ' ':
			if cur >= 0 {
				return m.Items[cur].ID, nil
			}
		default:
			// a letter: the next item beginning with it
			for o := 1; o <= len(m.Items); o++ {
				j := (cur + o + len(m.Items)) % len(m.Items)
				it := m.Items[j]
				if !it.Heading && strings.HasPrefix(strings.ToUpper(it.Label), string(unicode.ToUpper(k))) {
					cur = j
					break
				}
			}
		}
	}
}

// Hints is keys and what they do, in pairs: Hints("Enter", "Select", ...).
func Hints(kv ...string) []Hint {
	var out []Hint
	for i := 0; i+1 < len(kv); i += 2 {
		out = append(out, Hint{Key: kv[i], Label: kv[i+1]})
	}
	return out
}

// Screen is the screen last shown (nil: none yet).
func (d *Desk) Screen() *Screen { return d.last }
