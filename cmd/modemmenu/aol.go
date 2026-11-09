// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/D-Modem/menu/internal/config"
	"github.com/D-Modem/menu/internal/term"
	"golang.org/x/sys/unix"
)

// AOL: the classic AOL client (2.5 to 4.0) dials a network (AOLnet,
// SprintNet, Tymnet), whose prompts its CCL script answers, and then
// speaks P3 to the AOL host.  modemmenu plays the network: the script's
// answer at the login prompt tells which one it expects, and once the
// network part is done the line goes to the AOL host ([aol] host, the BBS's
// AOL front end), where the caller signs on with their BBS account.
//
//	AOLnet     modemmenu says UQKT1 (before the first login prompt), the
//	           script answers "aol", modemmenu says CONNECTED
//	SprintNet  the script sends "@D" until it is asked TERMINAL=, answers
//	           D1, is given the "@" prompt, asks "par? 11" (the line's
//	           speed) and calls the host with "C <address>": CONNECTED
//	Tymnet     the script sends "A" (no line end) until it is asked to log
//	           in, logs in, is given "+", names the host: CALL CONNECTED
//	P3         a script of one's own that starts P3 at once: its first
//	           packet (0x5A) where the name goes
//
// The strings are those AOL 3.0's own AOLNET.CCL, SPRINT.CCL and
// TYMNET.CCL wait for.

// errAOL: an AOL client's network script, or its P3, instead of a name
type errAOL struct{ network string }

func (e errAOL) Error() string { return "AOL client (" + e.network + ") at the login prompt" }

// The networks modemmenu can play.
const (
	netAOLnet    = "aolnet"
	netSprintNet = "sprintnet"
	netTymnet    = "tymnet"
	netP3        = "p3"
)

// aolSettings: [aol]
type aolSettings struct {
	enabled  bool
	host     string          // the AOL host, host:port
	networks map[string]bool // the networks played
	prompt   string          // AOLnet's greeting, before the first login prompt
	speed    string          // SprintNet's answer to par? 11
}

// loadAOL: [aol]
func loadAOL(sec *config.Section) (aolSettings, error) {
	var err error
	a := aolSettings{host: sec.Get("host", "localhost:5190"), prompt: config.Unescape(sec.Get("aolnet_prompt", "UQKT1")),
		speed: sec.Get("sprintnet_speed", "11:14"), networks: map[string]bool{}}
	if a.enabled, err = sec.Bool("enabled", false); err != nil {
		return a, err
	}
	for _, n := range config.Fields(strings.ToLower(sec.Get("networks", "aolnet sprintnet tymnet p3"))) {
		switch n {
		case netAOLnet, netSprintNet, netTymnet, netP3:
			a.networks[n] = true
		default:
			return a, fmt.Errorf("[aol] networks: no network %q (aolnet, sprintnet, tymnet, p3)", n)
		}
	}
	if _, _, err := net.SplitHostPort(a.host); a.enabled && err != nil {
		return a, fmt.Errorf("[aol] host: %v", err)
	}
	return a, nil
}

// sniffAOL: at the login prompt, a Tymnet script's lone "A" or a P3
// packet; what is neither is put back for the name
func sniffAOL(t *term.Term, a aolSettings, deadline time.Time) (string, error) {
	c, err := t.ReadOne(time.Until(deadline))
	if err != nil {
		return "", err
	}
	switch {
	case c == 0x5A && a.networks[netP3]:
		t.Unread(c)
		return netP3, nil
	case c == 'A' && a.networks[netTymnet]:
		// a caller types on; the script waits seconds for an answer
		next, err := t.ReadOne(1500 * time.Millisecond)
		if errors.Is(err, term.ErrTimeout) {
			return netTymnet, nil
		}
		if err != nil {
			return "", err
		}
		t.Unread(c, next)
		return "", nil
	}
	t.Unread(c)
	return "", nil
}

// aolName: the network whose script answered the login prompt so, or ""
func aolName(a aolSettings, name string) string {
	switch n := strings.ToUpper(strings.TrimSpace(name)); {
	case n == "AOL" && a.networks[netAOLnet]:
		return netAOLnet
	case (n == "@D" || n == "D") && a.networks[netSprintNet]:
		return netSprintNet
	}
	return ""
}

// aolCall: the rest of the network's part, then the line to the AOL host
// until either ends; the exit status
func aolCall(t *term.Term, s *settings, sess *session, network string) int {
	logf("AOL client (%s) at the login prompt %v", network, sess)
	if err := playNetwork(t, s.aol, network); err != nil {
		logf("aol: %s: %v %v", network, err, sess)
		return 1
	}
	c, err := net.DialTimeout("tcp", s.aol.host, 10*time.Second)
	if err != nil {
		logf("aol: %s: %v", s.aol.host, err)
		t.WriteRaw("\r\nNOT AVAILABLE\r\n") // the scripts' word for a host that is down
		return 1
	}
	defer c.Close()
	logf("aol: to %s %v", s.aol.host, sess)
	start := time.Now()
	n := pipe(t, c)
	logf("aol: ended after %s, %d bytes from the caller %v", time.Since(start).Round(time.Second), n, sess)
	return 0
}

// playNetwork: the network's prompts, as its script waits for them
func playNetwork(t *term.Term, a aolSettings, network string) error {
	deadline := time.Now().Add(time.Minute)
	line := func() (string, error) {
		l, err := t.ReadLine(deadline, 80, true, "")
		return strings.TrimSpace(l), err
	}
	switch network {
	case netAOLnet:
		return t.WriteRaw("\r\nCONNECTED\r\n")
	case netSprintNet:
		if err := t.WriteRaw("\r\nTERMINAL="); err != nil {
			return err
		}
		if _, err := line(); err != nil { // D1
			return err
		}
		if err := t.WriteRaw("\r\n@"); err != nil {
			return err
		}
		for {
			l, err := line()
			if err != nil {
				return err
			}
			switch u := strings.ToUpper(l); {
			case strings.HasPrefix(u, "PAR?"):
				// the line's speed, as X.3 parameter 11 (14: 9600 and up)
				if err := t.WriteRaw("\r\nPAR " + a.speed + "\r\n\r\n@"); err != nil {
					return err
				}
			case strings.HasPrefix(u, "C "):
				return t.WriteRaw("\r\n" + strings.TrimSpace(l[2:]) + " CONNECTED\r\n")
			default:
				if err := t.WriteRaw("\r\n@"); err != nil {
					return err
				}
			}
		}
	case netTymnet:
		if err := t.WriteRaw("\r\nPLEASE LOG IN: "); err != nil {
			return err
		}
		if _, err := line(); err != nil { // name;password
			return err
		}
		if err := t.WriteRaw("\r\n+"); err != nil {
			return err
		}
		if _, err := line(); err != nil { // the host's name
			return err
		}
		return t.WriteRaw("\r\nCALL CONNECTED\r\n")
	}
	return nil // P3: nothing to play
}

// pipe: the line and c joined, what was read and put back going first,
// until either ends; the bytes from the caller
func pipe(t *term.Term, c net.Conn) int64 {
	var n int64
	if p := t.TakePending(); len(p) > 0 {
		if _, err := c.Write(p); err != nil {
			return 0
		}
		n += int64(len(p))
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			k, err := c.Read(buf)
			if k > 0 && t.WriteRaw(string(buf[:k])) != nil {
				return
			}
			if err != nil {
				return
			}
		}
	}()
	buf := make([]byte, 4096)
	for {
		select {
		case <-done:
			return n
		default:
		}
		ok, err := term.PollOne(t.In, unix.POLLIN, 500*time.Millisecond)
		if err != nil {
			c.Close()
			return n
		}
		if !ok {
			continue
		}
		k, err := unix.Read(t.In, buf)
		if err == unix.EINTR || err == unix.EAGAIN {
			continue
		}
		if k <= 0 || err != nil {
			c.Close() // the caller hung up
			<-done
			return n
		}
		n += int64(k)
		if _, err := c.Write(buf[:k]); err != nil {
			return n
		}
	}
}
