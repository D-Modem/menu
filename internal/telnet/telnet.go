// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

// Package telnet is just enough of a telnet client for a BBS: binary,
// suppress go ahead and the server's echo, and the terminal type.
package telnet

import (
	"io"
	"sync"
)

// Client: just enough of a Client client for a BBS: binary, suppress go
// ahead and the server's echo, and the terminal type
type Client struct {
	mu     sync.Mutex
	conn   io.Writer
	term   string
	state  int
	cmd    byte
	sb     []byte
	local  map[byte]bool // options we do
	remote map[byte]bool // options the server does
}

const (
	iac  = 255
	dont = 254
	do   = 253
	wont = 252
	will = 251
	sb   = 250
	se   = 240

	optBinary = 0
	optEcho   = 1
	optSGA    = 3
	optTType  = 24
)

// Pass: b written to w, not in the middle of a reply to the server
// (a nil telnet writes as is)
func (tn *Client) Pass(w io.Writer, b []byte) (int, error) {
	if tn == nil {
		return w.Write(b)
	}
	tn.mu.Lock()
	defer tn.mu.Unlock()
	return w.Write(b)
}

// send: bytes to the server
func (tn *Client) send(b ...byte) {
	tn.Pass(tn.conn, b)
}

// Encode the caller's bytes: IAC doubled, CR as CR NUL unless binary
func (tn *Client) Encode(b []byte) []byte {
	tn.mu.Lock()
	bin := tn.local[optBinary]
	tn.mu.Unlock()
	out := make([]byte, 0, len(b)+8)
	for _, c := range b {
		out = append(out, c)
		if c == iac {
			out = append(out, iac)
		} else if c == '\r' && !bin {
			out = append(out, 0)
		}
	}
	return out
}

// Decode the server's bytes: its data, the commands answered
func (tn *Client) Decode(b []byte) []byte {
	out := b[:0:0]
	for _, c := range b {
		switch tn.state {
		case 0:
			if c == iac {
				tn.state = 1
			} else {
				out = append(out, c)
			}
		case 1:
			switch c {
			case iac:
				out = append(out, iac)
				tn.state = 0
			case will, wont, do, dont:
				tn.cmd, tn.state = c, 2
			case sb:
				tn.sb, tn.state = tn.sb[:0], 3
			default:
				tn.state = 0
			}
		case 2:
			tn.option(tn.cmd, c)
			tn.state = 0
		case 3:
			if c == iac {
				tn.state = 4
			} else if len(tn.sb) < 256 {
				tn.sb = append(tn.sb, c)
			}
		case 4:
			if c == se {
				tn.subneg()
				tn.state = 0
			} else {
				if c == iac && len(tn.sb) < 256 {
					tn.sb = append(tn.sb, iac)
				}
				tn.state = 3
			}
		}
	}
	return out
}

// option: the answer to the server's WILL, WONT, DO or DONT
func (tn *Client) option(cmd, opt byte) {
	switch cmd {
	case will:
		ok := opt == optEcho || opt == optSGA || opt == optBinary
		if ok && !tn.remote[opt] {
			tn.remote[opt] = true
			tn.send(iac, do, opt)
		} else if !ok {
			tn.send(iac, dont, opt)
		}
	case wont:
		if tn.remote[opt] {
			tn.remote[opt] = false
			tn.send(iac, dont, opt)
		}
	case do:
		ok := opt == optSGA || opt == optBinary || opt == optTType
		if ok && !tn.local[opt] {
			tn.mu.Lock()
			tn.local[opt] = true
			tn.mu.Unlock()
			tn.send(iac, will, opt)
		} else if !ok {
			tn.send(iac, wont, opt)
		}
	case dont:
		if tn.local[opt] {
			tn.mu.Lock()
			tn.local[opt] = false
			tn.mu.Unlock()
			tn.send(iac, wont, opt)
		}
	}
}

// subneg: the answer to a subnegotiation: the terminal type when asked
func (tn *Client) subneg() {
	// TTYPE SEND: TTYPE IS <terminal>
	if len(tn.sb) >= 2 && tn.sb[0] == optTType && tn.sb[1] == 1 {
		msg := []byte{iac, sb, optTType, 0}
		msg = append(msg, []byte(tn.term)...)
		tn.send(append(msg, iac, se)...)
	}
}

// New: a client on conn that says it is a term terminal
func New(conn io.Writer, term string) *Client {
	return &Client{conn: conn, term: term, local: map[byte]bool{}, remote: map[byte]bool{}}
}
