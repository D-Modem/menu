// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

// The handoff: the caller's line itself, a terminal, passed from one
// program to the next over a unix socket (SCM_RIGHTS) instead of its
// bytes copied.  The descriptor comes with the session header; the
// connection then carries nothing more of the caller's (but for what the
// sender had read of the line and not used, right after the header) and
// stays open for as long as the receiver has the line.  The receiver
// closing it gives the line back; the sender closing it takes it away.
//
// app_dmodem (handoff = yes) hands modemmenu -serve the modem's line, on
// which the caller's session runs as on a serial port; an item with
// handoff = yes hands the session's line to a server (the BBS) and has
// it back when the server is done.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/D-Modem/menu/internal/config"
	"github.com/D-Modem/menu/internal/term"
	"golang.org/x/sys/unix"
)

// readHeaderFD: readHeader on a unix socket, and the descriptor that came
// with it (nil: none came); a second descriptor is an error
func readHeaderFD(conn *net.UnixConn, timeout time.Duration) (map[string]string, *os.File, error) {
	conn.SetReadDeadline(time.Now().Add(timeout))
	defer conn.SetReadDeadline(time.Time{})
	var got *os.File
	fail := func(err error) (map[string]string, *os.File, error) {
		if got != nil {
			got.Close()
		}
		return nil, nil, err
	}
	h := map[string]string{}
	var line []byte
	total := 0
	b := make([]byte, 1)
	oob := make([]byte, unix.CmsgSpace(4*4))
	for {
		n, oobn, _, _, err := conn.ReadMsgUnix(b, oob)
		if oobn > 0 {
			fds, perr := rightsIn(oob[:oobn])
			if perr != nil {
				return fail(perr)
			}
			for _, fd := range fds {
				if got != nil {
					unix.Close(fd)
					return fail(errors.New("header: more than one descriptor"))
				}
				unix.CloseOnExec(fd)
				got = os.NewFile(uintptr(fd), "handoff")
			}
		}
		if err != nil {
			return fail(fmt.Errorf("header: %v", err))
		}
		if n == 0 {
			return fail(errors.New("header: end of file"))
		}
		if total++; total > headerMaxBytes {
			return fail(errors.New("header: too long"))
		}
		if b[0] != '\n' {
			line = append(line, b[0])
			continue
		}
		s := strings.TrimSuffix(string(line), "\r")
		line = line[:0]
		if s == "" {
			return h, got, nil
		}
		if len(h) >= headerMaxLines {
			return fail(errors.New("header: too many lines"))
		}
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			return fail(fmt.Errorf("header: no = in %q", s))
		}
		h[strings.ToUpper(strings.TrimSpace(k))] = v
	}
}

// rightsIn: the descriptors of SCM_RIGHTS messages in oob
func rightsIn(oob []byte) ([]int, error) {
	msgs, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return nil, err
	}
	var fds []int
	for i := range msgs {
		f, err := unix.ParseUnixRights(&msgs[i])
		if err == nil {
			fds = append(fds, f...)
		}
	}
	return fds, nil
}

// sendWithFD: b on conn, the descriptor fd attached to it
func sendWithFD(conn *net.UnixConn, b []byte, fd int) error {
	n, _, err := conn.WriteMsgUnix(b, unix.UnixRights(fd), nil)
	if err == nil && n < len(b) {
		_, err = conn.Write(b[n:])
	}
	return err
}

// runHandoff: the caller's line (the session's terminal) to the item's
// server on its unix socket, with the session header, until the server
// gives it back (closes the connection) or the deadline takes it away
func runHandoff(t *term.Term, it *menuItem, s *session, deadline time.Time) error {
	c, err := net.DialTimeout("unix", it.path, it.timeout)
	if err != nil {
		return err
	}
	conn := c.(*net.UnixConn)
	defer conn.Close()
	var b bytes.Buffer
	writeHeader(&b, s, it.password)
	// what the server is to have first: the item's send, then what the
	// caller typed that the menu read and did not take
	if it.send != "" {
		v := s.vars(it.password)
		b.WriteString(config.Expand(config.Unescape(it.send), v, nil))
	}
	b.Write(t.TakePending())
	// the server may change the line's flags (non-blocking, for one): the
	// menu's again once it is back
	flags, ferr := unix.FcntlInt(uintptr(t.In), unix.F_GETFL, 0)
	if err := sendWithFD(conn, b.Bytes(), t.In); err != nil {
		return err
	}
	defer func() {
		if ferr == nil {
			unix.FcntlInt(uintptr(t.In), unix.F_SETFL, flags)
		}
	}()
	conn.SetReadDeadline(deadline)
	// nothing comes but the end
	// (nothing comes but the end; the line gone meanwhile, the caller
	// hung up, is the menu's to find on its next read)
	io.Copy(io.Discard, conn)
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		return errTimeLimit
	}
	return nil
}
