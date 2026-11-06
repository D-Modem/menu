// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

// Package term is the caller's line: raw reads and writes with timeouts,
// line editing and echo, the questions that tell what terminal the caller
// has (ANSI, VT52, an ANSI terminal's identity), PETSCII for a Commodore,
// and a PPP frame told from typing.
package term

import (
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

var (
	ErrTimeout = errors.New("timeout")
	ErrHangup  = errors.New("hangup")
	// while WatchPETSCII: a key only a Commodore sends
	ErrPETSCII = errors.New("a Commodore's key")
)

// Term: the caller's line on stdin and stdout.  app_dmodem's pty is raw
// (no echo, no line editing, no CR/LF translation): the menu echoes and
// edits lines itself.  Reads poll stdin and take no more than they need,
// so that what the caller types after the menu goes to the selection.
type Term struct {
	In, Out int
	saved   *unix.Termios
	lastCR  bool
	pending []byte // read and put back (unread), read again first
	// a Commodore: what the menu writes and reads is PETSCII
	PETSCII bool
	// the terminal may be a Commodore's: a key only one sends ends a line
	// with ErrPETSCII
	WatchPETSCII bool
}

// SetPETSCII: the line a Commodore's from here on, in its lower/upper
// case set
func (t *Term) SetPETSCII() {
	if !t.PETSCII {
		t.PETSCII = true
		t.WriteRaw("\x0e")
	}
}

// Unread: b read again, before what the line has
func (t *Term) Unread(b ...byte) {
	t.pending = append(append([]byte{}, b...), t.pending...)
}

// TakePending: what was read and put back, taken off the line (for a
// selection to have it)
func (t *Term) TakePending() []byte {
	b := t.pending
	t.pending = nil
	return b
}

// New: the line on in and out, made raw if it is a terminal (its
// mode before kept for Close)
func New(in, out *os.File) *Term {
	t := &Term{In: int(in.Fd()), Out: int(out.Fd())}
	// a terminal of our own (a getty, a test from a console): raw as the
	// pty of app_dmodem is
	if tio, err := unix.IoctlGetTermios(t.In, unix.TCGETS); err == nil {
		saved := *tio
		t.saved = &saved
		MakeRaw(tio)
		unix.IoctlSetTermios(t.In, unix.TCSETS, tio)
	}
	return t
}

// MakeRaw: t as app_dmodem's pty is: no echo, no line editing, no
// translation, 8 bits
func MakeRaw(t *unix.Termios) {
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB
	t.Cflag |= unix.CS8
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
}

// MakeSane: a cooked line for programs that expect one (a shell, login)
func MakeSane(t *unix.Termios) {
	t.Iflag |= unix.BRKINT | unix.ICRNL | unix.IXON
	t.Iflag &^= unix.INLCR | unix.IGNCR
	t.Oflag |= unix.OPOST | unix.ONLCR
	t.Lflag |= unix.ECHO | unix.ECHOE | unix.ECHOK | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag |= unix.CS8
	t.Cc[unix.VERASE] = 0x7f
	t.Cc[unix.VKILL] = 'U' & 0x1f
	t.Cc[unix.VINTR] = 'C' & 0x1f
	t.Cc[unix.VEOF] = 'D' & 0x1f
}

// SetMode: the line raw or sane for a selection; the mode before, to
// restore (nil when stdin is no terminal)
func (t *Term) SetMode(mode string) *unix.Termios {
	tio, err := unix.IoctlGetTermios(t.In, unix.TCGETS)
	if err != nil {
		return nil
	}
	before := *tio
	if mode == "sane" {
		MakeSane(tio)
	} else {
		MakeRaw(tio)
	}
	unix.IoctlSetTermios(t.In, unix.TCSETS, tio)
	return &before
}

// RestoreMode: the line's mode as setMode found it
func (t *Term) RestoreMode(tio *unix.Termios) {
	if tio != nil {
		unix.IoctlSetTermios(t.In, unix.TCSETS, tio)
	}
}

// Close: the line's mode as newTerm found it
func (t *Term) Close() {
	if t.saved != nil {
		unix.IoctlSetTermios(t.In, unix.TCSETSW, t.saved)
	}
}

// Write what the menu says, as the terminal takes it
func (t *Term) Write(s string) error {
	if t.PETSCII {
		return t.WriteRaw(string(PETSCIIOut(s)))
	}
	return t.WriteRaw(s)
}

// WriteRaw: bytes as they are (a selection's, the terminal's questions)
func (t *Term) WriteRaw(s string) error {
	b := []byte(s)
	for len(b) > 0 {
		n, err := unix.Write(t.Out, b)
		if err == unix.EINTR || err == unix.EAGAIN {
			PollOne(t.Out, unix.POLLOUT, time.Second)
			continue
		}
		if err != nil {
			return ErrHangup
		}
		b = b[n:]
	}
	return nil
}

// PollOne: whether fd is ready for ev within d (d < 0: no limit)
func PollOne(fd int, ev int16, d time.Duration) (bool, error) {
	ms := -1
	if d >= 0 {
		ms = int(d / time.Millisecond)
	}
	fds := []unix.PollFd{{Fd: int32(fd), Events: ev}}
	for {
		n, err := unix.Poll(fds, ms)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return false, err
		}
		if n == 0 {
			return false, nil
		}
		return true, nil
	}
}

// ReadOne within d, or errTimeout; errHangup when the line is gone
func (t *Term) ReadOne(d time.Duration) (byte, error) {
	if len(t.pending) > 0 {
		b := t.pending[0]
		t.pending = t.pending[1:]
		return b, nil
	}
	deadline := time.Now().Add(d)
	var b [1]byte
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return 0, ErrTimeout
		}
		ok, err := PollOne(t.In, unix.POLLIN, left)
		if err != nil {
			return 0, ErrHangup
		}
		if !ok {
			return 0, ErrTimeout
		}
		n, err := unix.Read(t.In, b[:])
		if err == unix.EINTR || err == unix.EAGAIN {
			continue
		}
		if n <= 0 || err != nil {
			return 0, ErrHangup
		}
		return b[0], nil
	}
}

// ReadLine of at most max bytes until deadline, echoing it (or mask for
// each byte; "" for nothing).  CR, LF or CR LF end it; BS and DEL erase,
// ^U the line, ^D or ^C on an empty line hang up.
func (t *Term) ReadLine(deadline time.Time, max int, echo bool, mask string) (string, error) {
	var line []byte
	for {
		c, err := t.ReadOne(time.Until(deadline))
		if err != nil {
			return "", err
		}
		if t.WatchPETSCII && PETSCIITelltale(c) {
			if c < 0xc1 || !t.Continuation() {
				return "", ErrPETSCII
			}
			continue // a letter of UTF-8, not a shifted one
		}
		if t.PETSCII {
			c = PETSCIIIn(c)
		}
		if c == '\n' && t.lastCR {
			t.lastCR = false
			continue
		}
		t.lastCR = c == '\r'
		switch {
		case c == '\r' || c == '\n':
			return string(line), t.Write("\r\n")
		case c == 0x08 || c == 0x7f:
			if len(line) > 0 {
				line = line[:len(line)-1]
				if echo || mask != "" {
					t.Write("\b \b")
				}
			}
		case c == 0x15:
			for range line {
				if echo || mask != "" {
					t.Write("\b \b")
				}
			}
			line = line[:0]
		case (c == 0x03 || c == 0x04) && len(line) == 0:
			return "", io.EOF
		case c < 0x20 || c == 0xff:
			// other control bytes, telnet IAC: not part of a name
		case len(line) < max:
			line = append(line, c)
			switch {
			case echo:
				t.Write(string(c))
			case mask != "":
				t.Write(mask)
			}
		}
	}
}

// ReadKey: one key, upper case, until deadline
func (t *Term) ReadKey(deadline time.Time) (string, error) {
	for {
		c, err := t.ReadOne(time.Until(deadline))
		if err != nil {
			return "", err
		}
		if c == '\n' && t.lastCR {
			t.lastCR = false
			continue
		}
		if t.PETSCII {
			c = PETSCIIIn(c)
		}
		t.lastCR = c == '\r'
		if c == 0x04 {
			return "", io.EOF
		}
		if c > 0x20 && c < 0x7f {
			if c >= 'a' && c <= 'z' {
				c -= 'a' - 'A'
			}
			return string(c), nil
		}
	}
}

// Continuation: whether a UTF-8 Continuation byte follows at once (it is
// taken)
func (t *Term) Continuation() bool {
	c, err := t.ReadOne(200 * time.Millisecond)
	if err != nil {
		return false
	}
	if c >= 0x80 && c <= 0xbf {
		return true
	}
	t.Unread(c)
	return false
}

// CRLF: the line ends of a text file as the terminal wants them
func CRLF(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}
