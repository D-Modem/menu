// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

var (
	errTimeout = errors.New("timeout")
	errHangup  = errors.New("hangup")
)

// term: the caller's line on stdin and stdout.  app_dmodem's pty is raw
// (no echo, no line editing, no CR/LF translation): the menu echoes and
// edits lines itself.  Reads poll stdin and take no more than they need,
// so that what the caller types after the menu goes to the selection.
type term struct {
	in, out int
	saved   *unix.Termios
	lastCR  bool
	pending []byte // read and put back (unread), read again first
}

// unread: b read again, before what the line has
func (t *term) unread(b ...byte) {
	t.pending = append(append([]byte{}, b...), t.pending...)
}

func newTerm(in, out *os.File) *term {
	t := &term{in: int(in.Fd()), out: int(out.Fd())}
	// a terminal of our own (a getty, a test from a console): raw as the
	// pty of app_dmodem is
	if tio, err := unix.IoctlGetTermios(t.in, unix.TCGETS); err == nil {
		saved := *tio
		t.saved = &saved
		makeRaw(tio)
		unix.IoctlSetTermios(t.in, unix.TCSETS, tio)
	}
	return t
}

func makeRaw(t *unix.Termios) {
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB
	t.Cflag |= unix.CS8
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
}

// makeSane: a cooked line for programs that expect one (a shell, login)
func makeSane(t *unix.Termios) {
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

// setMode: the line raw or sane for a selection; the mode before, to
// restore (nil when stdin is no terminal)
func (t *term) setMode(mode string) *unix.Termios {
	tio, err := unix.IoctlGetTermios(t.in, unix.TCGETS)
	if err != nil {
		return nil
	}
	before := *tio
	if mode == "sane" {
		makeSane(tio)
	} else {
		makeRaw(tio)
	}
	unix.IoctlSetTermios(t.in, unix.TCSETS, tio)
	return &before
}

func (t *term) restoreMode(tio *unix.Termios) {
	if tio != nil {
		unix.IoctlSetTermios(t.in, unix.TCSETS, tio)
	}
}

func (t *term) close() {
	if t.saved != nil {
		unix.IoctlSetTermios(t.in, unix.TCSETSW, t.saved)
	}
}

func (t *term) write(s string) error {
	b := []byte(s)
	for len(b) > 0 {
		n, err := unix.Write(t.out, b)
		if err == unix.EINTR || err == unix.EAGAIN {
			pollOne(t.out, unix.POLLOUT, time.Second)
			continue
		}
		if err != nil {
			return errHangup
		}
		b = b[n:]
	}
	return nil
}

// pollOne: whether fd is ready for ev within d (d < 0: no limit)
func pollOne(fd int, ev int16, d time.Duration) (bool, error) {
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

// readByte within d, or errTimeout; errHangup when the line is gone
func (t *term) readByte(d time.Duration) (byte, error) {
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
			return 0, errTimeout
		}
		ok, err := pollOne(t.in, unix.POLLIN, left)
		if err != nil {
			return 0, errHangup
		}
		if !ok {
			return 0, errTimeout
		}
		n, err := unix.Read(t.in, b[:])
		if err == unix.EINTR || err == unix.EAGAIN {
			continue
		}
		if n <= 0 || err != nil {
			return 0, errHangup
		}
		return b[0], nil
	}
}

// readLine of at most max bytes until deadline, echoing it (or mask for
// each byte; "" for nothing).  CR, LF or CR LF end it; BS and DEL erase,
// ^U the line, ^D or ^C on an empty line hang up.
func (t *term) readLine(deadline time.Time, max int, echo bool, mask string) (string, error) {
	var line []byte
	for {
		c, err := t.readByte(time.Until(deadline))
		if err != nil {
			return "", err
		}
		if c == '\n' && t.lastCR {
			t.lastCR = false
			continue
		}
		t.lastCR = c == '\r'
		switch {
		case c == '\r' || c == '\n':
			return string(line), t.write("\r\n")
		case c == 0x08 || c == 0x7f:
			if len(line) > 0 {
				line = line[:len(line)-1]
				if echo || mask != "" {
					t.write("\b \b")
				}
			}
		case c == 0x15:
			for range line {
				if echo || mask != "" {
					t.write("\b \b")
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
				t.write(string(c))
			case mask != "":
				t.write(mask)
			}
		}
	}
}

// readKey: one key, upper case, until deadline
func (t *term) readKey(deadline time.Time) (string, error) {
	for {
		c, err := t.readByte(time.Until(deadline))
		if err != nil {
			return "", err
		}
		if c == '\n' && t.lastCR {
			t.lastCR = false
			continue
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
