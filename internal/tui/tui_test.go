// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// fakeLine: keys queued, what is written kept
type fakeLine struct {
	in  []byte
	out bytes.Buffer
}

func (f *fakeLine) ReadOne(time.Duration) (byte, error) {
	if len(f.in) == 0 {
		return 0, ErrTimeout
	}
	b := f.in[0]
	f.in = f.in[1:]
	return b, nil
}

func (f *fakeLine) WriteRaw(s string) error { f.out.WriteString(s); return nil }

// TestRender: the whole screen at first, then only what changed
func TestRender(t *testing.T) {
	a := NewScreen(20, 5)
	a.Put(1, 2, "hello", Attr{White, Blue})
	full := a.Render(nil, false)
	if !strings.HasPrefix(full, "\x1b[0m\x1b[2J") || !strings.Contains(full, "hello") {
		t.Fatalf("full: %q", full)
	}
	b := a.Clone()
	b.Put(1, 3, "a", Attr{White, Blue})
	if d := b.Render(a, false); d != "\x1b[2;4H\x1b[0;1;37;44ma" {
		t.Errorf("one cell: %q", d)
	}
	if d := b.Render(b, false); d != "" {
		t.Errorf("nothing changed: %q", d)
	}
	// two changes close together: the cells between written, not a move
	c := b.Clone()
	c.Put(1, 2, "J", Attr{White, Blue})
	c.Put(1, 5, "y", Attr{White, Blue})
	if d := c.Render(b, false); d != "\x1b[2;3H\x1b[0;1;37;44mJaly" {
		t.Errorf("a run: %q", d)
	}
}

// TestDeskPlain: a plain desk is cleared in its colors, its blank cells
// not sent
func TestDeskPlain(t *testing.T) {
	f := &fakeLine{}
	d := &Desk{Line: f, Rows: 24, Cols: 80, Charset: CP437}
	scr := d.Blank(-1)
	d.Show(scr)
	plain := f.out.Len()
	f.out.Reset()
	d.Shade = true
	d.Redraw()
	d.Show(d.Blank(-1))
	if shaded := f.out.Len(); plain > 600 || shaded < 1800 {
		t.Errorf("a plain desk took %d bytes, a shaded one %d", plain, shaded)
	}
	// a moved bar: a few bytes
	l := &List{Title: "Menu", Items: []Entry{{Key: "A", Label: "One"}, {Key: "B", Label: "Two"}}}
	s1 := d.Blank(-1)
	l.Draw(d, s1, true)
	d.Show(s1)
	f.out.Reset()
	l.Move(d, KeyDown)
	s2 := d.Blank(-1)
	l.Draw(d, s2, true)
	d.Show(s2)
	if n := f.out.Len(); n > 200 {
		t.Errorf("moving the bar took %d bytes: %q", n, f.out.String())
	}
}

// TestEncode: the lines in each charset
func TestEncode(t *testing.T) {
	s := "\x1b[1;1H╔═╗║░► x"
	for _, c := range []struct {
		cs   Charset
		want string
	}{
		{UTF8, s},
		{CP437, "\x1b[1;1H\xc9\xcd\xbb\xba\xb0\xaf x"},
		{DEC, "\x1b[1;1H\x1b(0lqkxa\x1b(B> x"},
		{ASCII, "\x1b[1;1H+=+| > x"},
	} {
		if got := string(Encode(s, c.cs)); got != c.want {
			t.Errorf("%v: %q, want %q", c.cs, got, c.want)
		}
	}
	// nothing of the control range: some terminals take it as controls
	for r := range cp437 {
		if b := cp437[r]; b < 0x20 {
			t.Errorf("%c is sent as control %#x", r, b)
		}
	}
}

// TestKeys: the keys of VTs, xterm and ANSI terminals, and Alt
func TestKeys(t *testing.T) {
	f := &fakeLine{in: []byte("\x1b[A\x1bOB\x1b[5~\x1b[11~\x1bOP\x1b[21~\r\nx\x1bf\x1b\x7f\xc3\xa9")}
	d := &Desk{Line: f, Charset: UTF8}
	want := []struct {
		k   rune
		alt bool
	}{{KeyUp, false}, {KeyDown, false}, {KeyPgUp, false}, {F(1), false}, {F(1), false}, {F(10), false},
		{KeyEnter, false}, {'x', false}, {'F', true}, {KeyEsc, false}, {KeyBackspace, false}, {'é', false}}
	for i, w := range want {
		k, alt, err := d.Key(time.Second)
		if err != nil || k != w.k || alt != w.alt {
			t.Errorf("key %d: %q %v %v, want %q %v", i, k, alt, err, w.k, w.alt)
		}
	}
	if _, _, err := d.Key(time.Second); err != ErrTimeout {
		t.Errorf("no more keys: %v", err)
	}
}

// TestDialogs: a question answered, a line typed and edited, a choice
func TestDialogs(t *testing.T) {
	f := &fakeLine{}
	d := &Desk{Line: f, Rows: 24, Cols: 80}
	base := d.Blank(-1)
	f.in = []byte("\x1b[C\r")
	if yes, err := d.Confirm(base, Info, "Q", "Sure?"); yes || err != nil {
		t.Errorf("right, Enter: %v %v", yes, err)
	}
	f.in = []byte("n")
	if i, _ := d.Message(base, Info, "Q", "Save?", "Save", "No", "Cancel"); i != 1 {
		t.Errorf("n: %d", i)
	}
	f.in = []byte("\x1b[Dxy\x7f\x1b[H>\r")
	if v, ok, _ := d.Input(base, "T", "type", "abc", 0, false); !ok || v != ">abxc" {
		t.Errorf("edited: %q %v", v, ok)
	}
	f.in = []byte("\x1b")
	if v, ok, _ := d.Input(base, "T", "type", "abc", 0, false); ok || v != "abc" {
		t.Errorf("Esc: %q %v", v, ok)
	}
	f.in = []byte("\x1b[B\x1b[B\r")
	if i, ok, _ := d.Choose(base, "Pick", []string{"a", "b", "c"}, nil, 0); !ok || i != 2 {
		t.Errorf("choice %d %v", i, ok)
	}
}

// TestPullDown: a menu pulled down, moved across, an item picked
func TestPullDown(t *testing.T) {
	f := &fakeLine{in: []byte("\x1b[C\x1b[B\r")}
	d := &Desk{Line: f, Rows: 24, Cols: 80, Menus: []Menu{
		{Label: "File", Items: []MenuItem{{Label: "Save", ID: "save"}}},
		{Label: "Setup", Items: []MenuItem{{Label: "One", ID: "one"}, {Heading: true}, {Label: "Two", ID: "two"}}},
	}}
	id, err := d.PullDown(d.Blank, 0)
	if err != nil || id != "two" {
		t.Errorf("picked %q %v", id, err)
	}
	if d.MenuKey('s') != 1 || d.MenuKey('f') != 0 || d.MenuKey('q') != -1 {
		t.Errorf("menu keys")
	}
}

// slowLine: times out a few times before its one key
type slowLine struct {
	quiet int
	waits []time.Duration
}

func (l *slowLine) ReadOne(d time.Duration) (byte, error) {
	l.waits = append(l.waits, d)
	if l.quiet > 0 {
		l.quiet--
		return 0, ErrTimeout
	}
	return 'x', nil
}

func (l *slowLine) WriteRaw(string) error { return nil }

// TestNoIdle: Idle < 0 waits through the line's timeouts for a key;
// a Deadline still ends the wait
func TestNoIdle(t *testing.T) {
	l := &slowLine{quiet: 3}
	d := &Desk{Line: l, Idle: -1}
	if k, _, err := d.Key(-1); err != nil || k != 'x' {
		t.Fatalf("Key = %q, %v; want 'x'", k, err)
	}
	if len(l.waits) != 4 {
		t.Fatalf("%d reads, want 4", len(l.waits))
	}
	l = &slowLine{quiet: 1 << 30}
	d = &Desk{Line: l, Idle: -1, Deadline: time.Now().Add(-time.Second)}
	if _, _, err := d.Key(-1); err != ErrTimeout {
		t.Fatalf("past the deadline: %v, want ErrTimeout", err)
	}
	d = &Desk{Line: &slowLine{quiet: 1}, Idle: time.Minute}
	if _, _, err := d.Key(-1); err != ErrTimeout {
		t.Fatalf("Idle a minute: %v, want ErrTimeout", err)
	}
}
