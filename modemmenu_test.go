// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// TestCheckPassword: each stored hash scheme accepts the right password
// and refuses others
func TestCheckPassword(t *testing.T) {
	bc, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	s256 := sha256.Sum256([]byte("pw"))
	s512 := sha512.Sum512([]byte("pw"))
	salted := sha256.Sum256([]byte("pwSALT"))
	ssha := base64.StdEncoding.EncodeToString(append(salted[:], "SALT"...))
	for _, c := range []struct {
		stored, scheme string
		ok             bool
	}{
		{string(bc), "auto", true},
		{string(bc), "bcrypt", true},
		{"{SHA256}" + hex.EncodeToString(s256[:]), "auto", true},
		{"{SHA256}" + base64.StdEncoding.EncodeToString(s256[:]), "auto", true},
		{"{SHA512}" + hex.EncodeToString(s512[:]), "auto", true},
		{"{SSHA256}" + ssha, "auto", true},
		{"{PLAIN}pw", "auto", true},
		{"{PLAIN}px", "auto", false},
		{"pw", "plain", true},
		{hex.EncodeToString(s256[:]), "sha256", true},
	} {
		for _, pw := range []string{"pw", "wrong"} {
			ok, err := checkPassword(c.stored, pw, c.scheme)
			if err != nil {
				t.Errorf("%s (%s): %v", c.stored, c.scheme, err)
			}
			if want := c.ok && pw == "pw"; ok != want {
				t.Errorf("%s (%s) %s: %v, want %v", c.stored, c.scheme, pw, ok, want)
			}
		}
	}
	if _, err := checkPassword("pw", "pw", "auto"); err == nil {
		t.Error("a bare password with hash = auto: no error")
	}
}

// TestExpand: {placeholders} replaced and shell-quoted; unknown ones
// kept
func TestExpand(t *testing.T) {
	v := map[string]string{"user": "o'neil; rm -rf /", "baud": "33600"}
	got := expand("login {user} {baud} {unknown} {", v, shellQuote)
	want := `login 'o'\''neil; rm -rf /' '33600' {unknown} {`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := expand("{USER}/{baud}", v, nil); got != v["user"]+"/33600" {
		t.Errorf("got %q", got)
	}
	if got := unescape(`a\r\n\x41\\b`); got != "a\r\nA\\b" {
		t.Errorf("unescape: %q", got)
	}
}

// TestSession: the call's details in the environment
func TestSession(t *testing.T) {
	now := time.Date(2026, 10, 10, 21, 5, 9, 0, time.UTC)
	s := newSession("50666/31200", "5551000", "5559999", now)
	s.User, s.Password = "alice", "secret"
	env := strings.Join(s.environ(true), "\n")
	for _, w := range []string{"MODEM_USER=alice", "MODEM_PASSWORD=secret", "MODEM_BAUD=50666",
		"MODEM_RX_RATE=31200", "MODEM_DIALED=5551000", "MODEM_CALLERID=5559999",
		"MODEM_DATE=2026-10-10", "MODEM_TIME=21:05:09", "MODEM_EPOCH=" + "1791666309"} {
		if !strings.Contains(env, w+"\n") && !strings.HasSuffix(env, w) {
			t.Errorf("no %s in\n%s", w, env)
		}
	}
	if strings.Contains(strings.Join(s.environ(false), "\n"), "secret") {
		t.Error("the password without pass_password")
	}
}

// TestTelnetDecode: the telnet client's data, its replies to the
// server, and the caller's bytes encoded
func TestTelnetDecode(t *testing.T) {
	var sent []byte
	tn := &telnet{conn: nil, term: "ANSI", local: map[byte]bool{}, remote: map[byte]bool{}}
	tn.conn = &recorder{b: &sent}
	in := []byte{'h', iac, will, optEcho, 'i', iac, iac, iac, do, optTType,
		iac, sb, optTType, 1, iac, se, iac, do, 31, '!'}
	out := tn.decode(in)
	if string(out) != "hi\xff!" {
		t.Errorf("data %q", out)
	}
	want := []byte{iac, do, optEcho, iac, will, optTType, iac, sb, optTType, 0, 'A', 'N', 'S', 'I', iac, se, iac, wont, 31}
	if string(sent) != string(want) {
		t.Errorf("replies % x, want % x", sent, want)
	}
	if got := tn.encode([]byte{'a', '\r', iac}); string(got) != "a\r\x00\xff\xff" {
		t.Errorf("encode % x", got)
	}
}

// TestINI: the configuration file's sections, keys in lower case and
// quoted values
func TestINI(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.conf")
	os.WriteFile(p, []byte("; comment\n[general]\nauth = file, sql\nbanner = \"  hi  \"\n[item  bbs]\nKey = b\n"), 0o600)
	c, err := parseINI(p)
	if err != nil {
		t.Fatal(err)
	}
	if g := c.section("general"); g.get("banner", "") != "  hi  " || len(fields(g.get("auth", ""))) != 2 {
		t.Errorf("general: %v", g.vals)
	}
	if c.section("item bbs").get("key", "") != "b" {
		t.Errorf("item: %v", c.section("item bbs").vals)
	}
}

// TestBackends: the file and SQL backends, in order, accept and refuse
// as they should
func TestBackends(t *testing.T) {
	dir := t.TempDir()
	users := filepath.Join(dir, "users")
	bc, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	os.WriteFile(users, []byte("# users\nalice:"+string(bc)+":Alice\nbob:{PLAIN}a:b\n"), 0o600)
	fa, err := newFileAuth(&section{name: "file", vals: map[string]string{"path": users}})
	if err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(dir, "users.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE users (username TEXT, password TEXT);
		INSERT INTO users VALUES ('carol', '{PLAIN}pw'), ('alice', '{PLAIN}other');`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	sa, err := newSQLAuth(&section{name: "sql", vals: map[string]string{
		"driver": "sqlite", "dsn": dbPath,
		"query": "SELECT password FROM users WHERE username = ?"}})
	if err != nil {
		t.Fatal(err)
	}
	sn, err := newSQLAuth(&section{name: "sql", vals: map[string]string{
		"driver": "sqlite", "dsn": dbPath, "args": "user password", "hash": "none",
		"query": "SELECT 1 FROM users WHERE username = ? AND password = '{PLAIN}' || ?"}})
	if err != nil {
		t.Fatal(err)
	}

	backends := []authenticator{fa, sa}
	nolog := func(string, ...any) {}
	for _, c := range []struct {
		user, pw, auth string
		err            error
	}{
		{"alice", "secret", "file", nil},
		{"alice", "other", "", errBadPassword}, // the file knows alice: not asked further
		{"bob", "a:b", "file", nil},
		{"carol", "pw", "sql", nil},
		{"carol", "px", "", errBadPassword},
		{"dave", "pw", "", errNoUser},
	} {
		auth, err := authenticate(context.Background(), backends, c.user, c.pw, nolog)
		if auth != c.auth || !errors.Is(err, c.err) && !(err == nil && c.err == nil) {
			t.Errorf("%s/%s: %q %v, want %q %v", c.user, c.pw, auth, err, c.auth, c.err)
		}
	}
	if err := sn.authenticate(context.Background(), "carol", "pw"); err != nil {
		t.Errorf("hash = none: %v", err)
	}
	if err := sn.authenticate(context.Background(), "carol", "px"); !errors.Is(err, errNoUser) {
		t.Errorf("hash = none, wrong password: %v", err)
	}
}

type recorder struct{ b *[]byte }

// Write: p kept
func (r *recorder) Write(p []byte) (int, error) {
	*r.b = append(*r.b, p...)
	return len(p), nil
}

// TestMenuSettings: what keys, names and aliases select, and the menu
// settings that are refused
func TestMenuSettings(t *testing.T) {
	dir := t.TempDir()
	users := filepath.Join(dir, "users")
	os.WriteFile(users, nil, 0o600)
	load := func(menu, items string) (*settings, error) {
		p := filepath.Join(dir, "c.conf")
		os.WriteFile(p, []byte("[general]\nauth = file\n[file]\npath = "+users+"\n[menu]\n"+menu+"\n"+items), 0o600)
		return loadSettings(p)
	}
	ppp := "[item ppp]\nkey = P\naliases = dialup net\ncommand = true\n"
	s, err := load("", ppp+"[item bbs]\nkey = BBS\ncommand = true\n")
	if err != nil {
		t.Fatal(err)
	}
	for choice, want := range map[string]string{"P": "ppp", "PPP": "ppp", "NET": "ppp", "BBS": "bbs", "B": ""} {
		it := s.selects(choice)
		if want == "" && it != nil || want != "" && (it == nil || it.name != want) {
			t.Errorf("%s selects %v, want %q", choice, it, want)
		}
	}
	if !contains(s.hangup, "GOODBYE") {
		t.Errorf("hangup %v", s.hangup)
	}
	for _, c := range []struct{ menu, items string }{
		{"", ppp + "[item net]\ncommand = true\n"}, // net: an alias of ppp
		{"hangup = P", ppp},                        // P hangs up and selects ppp
		{"hotkeys = yes", ppp + "[item bbs]\nkey = BBS\ncommand = true\n"},
	} {
		if _, err := load(c.menu, c.items); err == nil {
			t.Errorf("%q %q: no error", c.menu, c.items)
		}
	}
	if _, err := load("hotkeys = yes", ppp); err != nil {
		t.Errorf("hotkeys with one-character keys: %v", err)
	}
}

// runGo: the go command with args, its output
func runGo(args ...string) ([]byte, error) {
	return exec.Command("go", args...).CombinedOutput()
}
