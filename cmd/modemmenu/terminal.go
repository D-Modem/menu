// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/D-Modem/menu/internal/config"
	"github.com/D-Modem/menu/internal/term"
	"golang.org/x/sys/unix"
)

// terminalSettings: [terminal]
type terminalSettings struct {
	detect  bool   // ask: ANSI, VT52, an ANSI terminal's identity
	petscii string // auto (a terminal that is not ANSI may be one), yes (every caller), no
	timeout time.Duration
}

// loadTerminal: [terminal], checked
func loadTerminal(sec *config.Section) (terminalSettings, error) {
	var err error
	ts := terminalSettings{
		petscii: strings.ToLower(sec.Get("petscii", "auto")),
	}
	if ts.detect, err = sec.Bool("detect", true); err != nil {
		return ts, err
	}
	if ts.timeout, err = sec.Duration("timeout", 2*time.Second); err != nil {
		return ts, err
	}
	switch ts.petscii {
	case "auto", "yes", "no":
	default:
		return ts, fmt.Errorf("[terminal] petscii: %q (auto, yes or no)", ts.petscii)
	}
	return ts, nil
}

// detectTerminal: what the caller's terminal is, in sess (Terminal, Term,
// TermName, Rows, Cols), the line set for it
func detectTerminal(t *term.Term, s *settings, sess *session) error {
	ts := s.terminal
	defer func() {
		if sess.Terminal == "petscii" {
			t.SetPETSCII()
		}
		if sess.Rows != "" && sess.Cols != "" {
			rows, _ := strconv.Atoi(sess.Rows)
			cols, _ := strconv.Atoi(sess.Cols)
			// the line's size, for what a selection runs on it
			unix.IoctlSetWinsize(t.In, unix.TIOCSWINSZ, &unix.Winsize{Row: uint16(rows), Col: uint16(cols)})
		}
	}()
	switch {
	case sess.Terminal != "":
		// an upstream menu said
		return nil
	case ts.petscii == "yes":
		sess.Terminal, sess.Term, sess.Rows, sess.Cols = "petscii", "dumb", "25", "40"
		return nil
	}
	p := &term.Prober{T: t}
	defer p.Done()
	if !ts.detect || p.PPP {
		// not asked: perhaps a Commodore, whose keys will tell
		t.WatchPETSCII = ts.petscii == "auto" && !p.PPP
		return nil
	}
	ansi, rows, cols, err := p.ANSI(ts.timeout)
	if err != nil || p.PPP {
		return err
	}
	if !ansi {
		is52, err := p.VT52(ts.timeout / 2)
		if err != nil || p.PPP {
			return err
		}
		if is52 {
			sess.Terminal, sess.Term, sess.TermName, sess.Rows, sess.Cols = "vt52", "vt52", "VT52", "24", "80"
			return nil
		}
		sess.Terminal, sess.Term = "ascii", "dumb"
		// perhaps a Commodore: its keys will tell
		t.WatchPETSCII = ts.petscii == "auto"
		return nil
	}
	sess.Terminal, sess.Term = "ansi", "ansi"
	if rows > 0 && cols > 0 {
		sess.Rows, sess.Cols = strconv.Itoa(rows), strconv.Itoa(cols)
	}
	in, err := p.Identify(ts.timeout)
	if err != nil {
		return err
	}
	if n := in.Name(); n != "" {
		sess.TermName = n
	}
	sess.Term = in.Term()
	if in.Level() > 0 || in.Emulator() {
		sess.Terminal = "vt"
	}
	return nil
}

// becomePETSCII: the caller has a Commodore after all
func becomePETSCII(t *term.Term, sess *session) {
	t.WatchPETSCII = false
	sess.Terminal, sess.Term, sess.Rows, sess.Cols = "petscii", "dumb", "25", "40"
	unix.IoctlSetWinsize(t.In, unix.TIOCSWINSZ, &unix.Winsize{Row: 25, Col: 40})
	t.SetPETSCII()
	logf("terminal: a Commodore (PETSCII) %v", sess)
}

// settlePETSCII: a name typed all in capitals, while it is not known
// whether the terminal is a Commodore: a Commodore's unshifted keys, or
// ASCII in capitals, and DEL tells which.  The name as the caller meant
// it.
func settlePETSCII(t *term.Term, s *settings, sess *session, name string, deadline time.Time) (string, error) {
	t.WatchPETSCII = false
	if strings.ToUpper(name) != name || strings.ToLower(name) == name {
		return name, nil // lower case, or no letters
	}
	if err := t.Write("PRESS DEL (OR BACKSPACE): "); err != nil {
		return "", err
	}
	wait := min(30*time.Second, time.Until(deadline))
	c, err := t.ReadOne(wait)
	if err != nil && !errors.Is(err, term.ErrTimeout) {
		return "", err
	}
	t.Write("\r\n")
	if c != 0x14 {
		return name, nil
	}
	becomePETSCII(t, sess)
	// its unshifted letters are lower case
	name = strings.ToLower(name)
	t.Write(s.loginPrompt + name + "\r\n")
	return name, nil
}

// describeTerminal: for the log
func (s *session) describeTerminal() string {
	d := s.Terminal
	if s.TermName != "" {
		d += " (" + s.TermName + ")"
	}
	if s.Rows != "" {
		d += " " + s.Cols + "x" + s.Rows
	}
	return d
}
