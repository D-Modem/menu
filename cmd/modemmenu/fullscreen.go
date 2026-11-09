// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/D-Modem/menu/internal/config"
	"github.com/D-Modem/menu/internal/term"
	"github.com/D-Modem/menu/internal/tui"
)

// The menu full screen, on a terminal that can have it (ANSI, or a VT
// that said what it is, of 40x14 at least): the items in a window on the
// desk, a bar moved with the cursor keys, an item's key picking it at
// once.  Others get the menu of lines.  The login stays lines, for the
// dialers and AOL clients that read and answer it.

// deskFor is the full screen for the caller's terminal, nil when it
// cannot have one (or [menu] fullscreen = no).
func deskFor(t *term.Term, s *settings, sess *session) *tui.Desk {
	if !s.fullscreen || t.PETSCII || (sess.Terminal != "ansi" && sess.Terminal != "vt") {
		return nil
	}
	rows, _ := strconv.Atoi(sess.Rows)
	cols, _ := strconv.Atoi(sess.Cols)
	if rows == 0 || cols == 0 {
		rows, cols = 24, 80
	}
	if !tui.OK(rows, cols) {
		return nil
	}
	d := &tui.Desk{Line: t, Rows: rows, Cols: cols, Idle: s.idleTimeout}
	// the shaded desk costs a byte or three a cell: not below 9600 bit/s
	if rate, err := strconv.Atoi(sess.TxRate); err != nil || rate >= 9600 {
		d.Shade = true
	}
	// a DEC terminal itself has no colors; the emulators do
	d.Mono = sess.hardware
	// the lines: UTF-8 where it showed é as one character; else a VT's
	// own set for what said it is a VT or xterm, code page 437 for the
	// ANSI terminals of BBSs (which ask nothing of them) and SyncTERM
	cs, ok := tui.ParseCharset(s.terminal.charset)
	switch {
	case ok:
	case sess.charset == "utf8", sess.charset == "" && sess.Term == "xterm":
		cs = tui.UTF8
	case sess.charset != "cp437" && (strings.HasPrefix(sess.Term, "vt") || sess.Term == "xterm"):
		cs = tui.DEC
	default:
		cs = tui.CP437
	}
	d.Charset = cs
	return d
}

// escapes: what a template's text has that is not to be read
var escapes = regexp.MustCompile(`\x1b\[[0-9;?]*[@-~]|\x1b.|[\x00-\x08\x0b-\x1f\x7f]`)

// plainLines: a template's text as lines to put in a window, without its
// escapes and empty lines
func plainLines(s string) []string {
	s = escapes.ReplaceAllString(strings.ReplaceAll(s, "\r", ""), "")
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// fullMenu: the menu full screen until the caller hangs up, says goodbye,
// the time is up or a selection ends with after = hangup; first (if not
// nil) starts at once.  The desk is down when it returns.
func fullMenu(t *term.Term, s *settings, sess *session, lim *limiter, first *menuItem, d *tui.Desk, motd string) error {
	defer d.Close()
	items := s.visible(sess)
	title := "Main menu"
	if l := plainLines(config.Expand(s.menuTitle, sess.vars(false), nil)); len(l) > 0 {
		title = l[0]
	}
	w := &tui.List{Title: title, Center: true, MinW: 44}
	if l := plainLines(config.Expand(s.welcome, sess.vars(false), nil)); len(l) > 0 {
		w.Notes = l
	}
	for _, it := range items {
		w.Items = append(w.Items, tui.Entry{ID: it.name, Key: it.key, Label: it.label, Help: it.help})
	}
	w.Items = append(w.Items, tui.Entry{Key: s.hangup[0], Label: "Goodbye (hang up)", Help: "Hang up the line."})
	w.Fix()
	d.Name = sess.User
	if sp := sess.Speed(); sp != "" {
		d.Name += " at " + sp
	}
	// a box to show on the desk: the motd first, then what launch said
	// went wrong
	notice, noticeTitle, noticeKind := "", "", tui.Info
	if m := strings.TrimSpace(escapes.ReplaceAllString(strings.ReplaceAll(motd, "\r", ""), "")); m != "" {
		notice, noticeTitle = m, "Message of the day"
	}
	for {
		lim.mark(sess, time.Now())
		left, limited := lim.left(time.Now())
		if limited && left <= 0 {
			return errTimeLimit
		}
		idle := time.Now().Add(s.idleTimeout)
		if limited && lim.deadline.Before(idle) {
			idle = lim.deadline
		}
		it := first
		first = nil
		for it == nil {
			now := time.Now()
			left, limited := lim.left(now)
			frame := func() *tui.Screen {
				scr := d.Blank(-1)
				d.Status(scr, now.Format("15:04"))
				w.Draw(d, scr, true)
				if e := w.Selected(); e != nil {
					help := e.Help
					if help == "" {
						help = "Enter starts it, or press its key: " + e.Key + "."
					}
					d.HelpBox(scr, e.Label, help, false)
				}
				status := ""
				if limited {
					status = fmt.Sprintf("%d min left", int((left+30*time.Second)/time.Minute))
				}
				d.KeyBar(scr, tui.Hints("↑↓", "Choose", "Enter", "Select", "Esc", "Goodbye", "F1", "Help"), status)
				return scr
			}
			scr := frame()
			if notice != "" {
				if _, err := d.Message(scr, noticeKind, noticeTitle, notice); err != nil {
					return err
				}
				notice = ""
				continue
			}
			if err := d.Show(scr); err != nil {
				return err
			}
			// a key until idle, the clock redrawn each minute
			wait := min(time.Until(idle), time.Until(now.Truncate(time.Minute).Add(time.Minute)))
			k, _, err := d.Key(max(wait, time.Millisecond))
			if errors.Is(err, term.ErrTimeout) {
				if time.Now().Before(idle) {
					continue
				}
				if limited && !time.Now().Before(lim.deadline) {
					return errTimeLimit
				}
				return term.ErrTimeout
			}
			if err != nil {
				return err
			}
			if terminated.Load() {
				return term.ErrHangup
			}
			idle = time.Now().Add(s.idleTimeout)
			if limited && lim.deadline.Before(idle) {
				idle = lim.deadline
			}
			pick := false
			switch k {
			case tui.KeyEnter, tui.KeyRight:
				pick = true
			case 0x04:
				return io.EOF
			case tui.KeyEsc:
				yes, err := d.Confirm(frame(), tui.Info, "Goodbye", "Hang up the line?")
				if err != nil {
					return err
				}
				if yes {
					return nil
				}
			case tui.F(1), '?':
				if _, err := d.Message(frame(), tui.Info, "Help", menuHelp); err != nil {
					return err
				}
			default:
				_, pick = w.Move(d, k)
			}
			if !pick {
				continue
			}
			e := w.Selected()
			if e == nil {
				continue
			}
			if e.ID == "" {
				return nil // goodbye
			}
			it = selectFrom(items, strings.ToUpper(e.Key))
		}

		// the selection has the line: the desk down, the screen clear
		d.Close()
		sess.Selection = it.name
		lim.mark(sess, time.Now())
		logf("start %s user=%s mode=%s", it.name, sess.User, it.mode)
		err := launch(t, s, it, sess, lim.deadline, logf)
		if errors.Is(err, term.ErrHangup) || terminated.Load() {
			return term.ErrHangup
		}
		if errors.Is(err, errTimeLimit) {
			logf("%s: time limit", it.name)
			return errTimeLimit
		}
		if err != nil {
			logf("%s: %v", it.name, err)
			notice, noticeTitle, noticeKind = it.label+" is not available now.", "Not available", tui.Alert
			continue
		}
		logf("end %s user=%s", it.name, sess.User)
		if it.after == "hangup" {
			return nil
		}
		if pausesAfter(it) {
			// what it wrote stays until the caller has read it
			if err := pause(t, d, s, "Press any key for the menu"); err != nil {
				return err
			}
		}
	}
}

// menuHelp: F1 in the full-screen menu
const menuHelp = "Move the bar with the cursor keys (or the first letter of an item) " +
	"and press Enter, or press an item's key in [ ] to start it at once.\n\n" +
	"Esc hangs up, after asking.  ^L draws the screen again."

// pausesAfter: whether what an item wrote is kept on the screen until a
// key (programs here; not a server that drew its own screens)
func pausesAfter(it *menuItem) bool {
	switch it.mode {
	case "exec", "passwd", "callers":
		return true
	}
	return false
}

// pause: "press a key" under what is on the screen, until one or idle
func pause(t *term.Term, d *tui.Desk, s *settings, say string) error {
	if err := t.Write("\r\n\x1b[0;7m " + say + " \x1b[0m"); err != nil {
		return err
	}
	_, _, err := d.Key(s.idleTimeout)
	return err
}
