// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

// Package authd is modemmenu's auth socket ([authd] in modemmenu.conf),
// for the programs that ask it: modemmenu's backends (its users file,
// LDAP, SQL, the system's accounts by PAM) answer whether a password is a
// user's, so that a BBS's other logins (telnet, SSH, the web) take the
// same one as the dial-up line.  One request a connection, a JSON line
// each way:
//
//	{"user": "alice", "password": "..."}
//	{"ok": true, "auth": "ldap"}      or      {"ok": false, "error": "denied"}
package authd

// Request is what a client sends.
type Request struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

// Reply is modemmenu's answer.
type Reply struct {
	OK    bool   `json:"ok"`
	Auth  string `json:"auth,omitempty"`  // the backend that took it
	Error string `json:"error,omitempty"` // denied, refused, bad request
}

// MaxRequest is the longest request line modemmenu reads.
const MaxRequest = 4096
