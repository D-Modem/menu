// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPasswdLine: on a line, a mode = passwd item changes the password
func TestPasswdLine(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "modemmenu")
	if out, err := runGo("build", "-o", bin, "."); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("alice:{PLAIN}old-password\n"), 0o600)
	conf := filepath.Join(dir, "c.conf")
	os.WriteFile(conf, []byte(fmt.Sprintf(`
[general]
log = %s
auth = file
fail_delay = 0
[file]
path = %s
[guest]
enabled = yes
[item passwd]
key = PW
label = Change your password
mode = passwd
guest = yes
[item info]
command = echo "PW=$MODEM_PASSWORD"
after = menu
`, filepath.Join(dir, "log"), users)), 0o600)

	c, cmd := callAs(t, bin, conf)
	c.expect("login: ")
	c.send("alice\r")
	c.expect("Password: ")
	c.send("old-password\r")
	c.expect("Selection: ")
	c.send("pw\r")
	c.expect("Current password: ")
	c.send("wrong\r")
	c.expect("That is not your password.")
	c.expect("Selection: ")
	c.send("passwd\r")
	c.expect("Current password: ")
	c.send("old-password\r")
	c.expect("New password: ")
	c.send("short\r")
	c.expect("At least 8")
	c.expect("New password: ")
	c.send("old-password\r")
	c.expect("another one")
	c.expect("New password: ")
	c.send("new-password\r")
	c.expect("Again: ")
	c.send("new-passwork\r")
	c.expect("They differ.")
	// three tries
	c.expect("Your password is as it was.")
	c.expect("Selection: ")
	c.send("pw\r")
	c.expect("Current password: ")
	c.send("old-password\r")
	c.expect("New password: ")
	c.send("new-password\r")
	c.expect("Again: ")
	c.send("new-password\r")
	c.expect("Your password is changed.")
	c.expect("Selection: ")
	// the selections after it get the new one
	c.send("info\r")
	c.expect("PW=new-password")
	c.send("g\r")
	waitExit(t, cmd)

	c, cmd = callAs(t, bin, conf)
	c.expect("login: ")
	c.send("alice\r")
	c.expect("Password: ")
	c.send("old-password\r")
	c.expect("Login incorrect")
	c.expect("login: ")
	c.send("alice\r")
	c.expect("Password: ")
	c.send("new-password\r")
	c.expect("Selection: ")
	c.send("g\r")
	waitExit(t, cmd)

	// a guest has none
	c, cmd = callAs(t, bin, conf)
	c.expect("login: ")
	c.send("guest\r")
	c.expect("Selection: ")
	c.send("pw\r")
	c.expect("A guest has no password")
	c.send("g\r")
	waitExit(t, cmd)

	log, _ := os.ReadFile(filepath.Join(dir, "log"))
	for _, w := range []string{"passwd user=alice: current password wrong", "passwd user=alice auth=file: changed"} {
		if !strings.Contains(string(log), w) {
			t.Errorf("no %q in the log:\n%s", w, log)
		}
	}
	if strings.Contains(string(log), "new-password") {
		t.Errorf("a password in the log:\n%s", log)
	}
}
