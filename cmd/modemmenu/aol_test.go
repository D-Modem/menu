// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// an AOL client's network scripts at the login prompt: modemmenu plays
// AOLnet, SprintNet and Tymnet as AOL 3.0's CCL files wait for them, and
// then joins the line to the AOL host
func TestAOLLine(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "modemmenu")
	if out, err := runGo("build", "-o", bin, "."); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	// the AOL host: answers what the line sends it
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				// a P3 packet's first 8 bytes (it may come in pieces)
				var got []byte
				buf := make([]byte, 64)
				for len(got) < 8 {
					n, err := c.Read(buf)
					got = append(got, buf[:n]...)
					if err != nil {
						break
					}
				}
				fmt.Fprintf(c, "host got %q", got)
			}()
		}
	}()
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("alice:{PLAIN}secret\n"), 0o600)
	log := filepath.Join(dir, "log")
	conf := filepath.Join(dir, "c.conf")
	os.WriteFile(conf, []byte(fmt.Sprintf(`
[general]
log = %s
auth = file
[file]
path = %s
[aol]
enabled = yes
host = %s
[item info]
command = true
`, log, users, ln.Addr())), 0o600)

	p3 := "\x5a\x00\x00\x00\x34\x7f\x7f\xa3"
	// AOLnet
	c, cmd := callAs(t, bin, conf)
	c.expect("UQKT1\r\n")
	c.expect("login: ")
	c.send("aol\r")
	c.expect("CONNECTED\r\n")
	c.send(p3)
	c.expect(fmt.Sprintf("host got %q", p3))
	waitExit(t, cmd)

	// SprintNet
	c, cmd = callAs(t, bin, conf)
	c.expect("login: ")
	c.send("@D\r")
	c.expect("TERMINAL=")
	c.send("D1\r")
	c.expect("\r\n@")
	c.send("par? 11\r")
	c.expect("11:14")
	c.send("C 31270\r")
	c.expect("31270 CONNECTED\r\n")
	c.send(p3)
	c.expect("host got")
	waitExit(t, cmd)

	// Tymnet: "A" with no line end
	c, cmd = callAs(t, bin, conf)
	c.expect("login: ")
	c.send("A")
	c.expect("LOG IN: ")
	c.send("aol;secret\r")
	c.expect("+")
	c.send("AOL\r")
	c.expect("CALL CONNECTED\r\n")
	c.send(p3)
	c.expect("host got")
	waitExit(t, cmd)

	// a CCL of one's own: P3 at once
	c, cmd = callAs(t, bin, conf)
	c.expect("login: ")
	c.send(p3)
	c.expect(fmt.Sprintf("host got %q", p3))
	waitExit(t, cmd)

	// a caller whose name starts with A is a caller
	c, cmd = callAs(t, bin, conf)
	c.expect("login: ")
	c.send("alice\r")
	c.expect("Password: ")
	c.send("secret\r")
	c.expect("Selection: ")
	cmd.Process.Kill()

	b, _ := os.ReadFile(log)
	for _, w := range []string{"AOL client (aolnet)", "AOL client (sprintnet)", "AOL client (tymnet)", "AOL client (p3)",
		"aol: to " + ln.Addr().String(), "login user=alice"} {
		if !strings.Contains(string(b), w) {
			t.Errorf("no %q in the log:\n%s", w, b)
		}
	}
}

// [aol] networks takes only those modemmenu plays
func TestAOLSettings(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "c.conf")
	users := filepath.Join(dir, "users")
	os.WriteFile(users, nil, 0o600)
	head := "[general]\nlog = none\nauth = file\n[file]\npath = " + users + "\n[item x]\ncommand = true\n"
	os.WriteFile(conf, []byte(head+"[aol]\nenabled = yes\nnetworks = aolnet compuserve\n"), 0o600)
	if _, err := loadSettings(conf); err == nil || !strings.Contains(err.Error(), "compuserve") {
		t.Errorf("compuserve: %v", err)
	}
	os.WriteFile(conf, []byte(head+"[aol]\nenabled = yes\nnetworks = tymnet\n"), 0o600)
	s, err := loadSettings(conf)
	if err != nil {
		t.Fatal(err)
	}
	if !s.aol.networks[netTymnet] || s.aol.networks[netAOLnet] || s.aol.host != "localhost:5190" {
		t.Errorf("%+v", s.aol)
	}
	if aolName(s.aol, "aol") != "" {
		t.Error("aol without AOLnet played")
	}
}
