package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// hashes of "pw123" by mkpasswd (libxcrypt)
var systemHashes = map[string]string{
	"yescrypt": "$y$j9T$/tDn3yJx1xmozsDcSKtLG/$XDHKBcewLul2hgpgzjTXbw.Sz1doE9PcZ6HVbCZU2r.",
	"sha512":   "$6$WGDfVGNbOnBz0Fxg$o3y6ojWIAQnc6G0BjyDk0BlveuiqsT2zq/8eGJD4Xf449xkYMvGUa5jnuizLjMgzU/nd7hKEdSCNEPwx6l.pK.",
	"sha256":   "$5$WY1xA9Dk8LzlgmFH$y.zVbdfZtzglOCpp8SWerkXrXlqsaT..m5y1c6YsbT4",
	"md5":      "$1$5/Jxb5gD$yQFclV1oPQB4IoIT7ul0A0",
	"bcrypt":   "$2b$05$wuaPobJ7LD3K59VdM1lH7OQz58/HjNgUJvsW.IKEZz6X0s.hggr52",
}

func TestCryptCompare(t *testing.T) {
	for name, h := range systemHashes {
		for _, pw := range []string{"pw123", "pw124"} {
			ok, err := cryptCompare(h, pw)
			if err != nil || ok != (pw == "pw123") {
				t.Errorf("%s %s: %v %v", name, pw, ok, err)
			}
		}
	}
	if _, err := cryptCompare("$7$CU..../....abc", "pw"); err == nil {
		t.Error("an unsupported hash: no error")
	}
}

func TestSystemShadow(t *testing.T) {
	dir := t.TempDir()
	w := func(name, s string) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(s), 0o600)
		return p
	}
	// today: day 20000 (2024-10-04)
	passwd := w("passwd", strings.Join([]string{
		"root:x:0:0:root:/root:/bin/bash",
		"daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin",
		"alice:x:1000:1000:Alice:/home/alice:/bin/bash",
		"bob:x:1001:1001:Bob:/home/bob:/bin/sh",
		"carol:x:1002:100:Carol:/home/carol:/bin/sh",
		"locked:x:1003:1003::/home/locked:/bin/sh",
		"expired:x:1004:1004::/home/expired:/bin/sh",
		"oldpw:x:1005:1005::/home/oldpw:/bin/sh",
		"legacy:" + systemHashes["md5"] + ":1006:1006::/home/legacy:/bin/sh",
	}, "\n")+"\n")
	shadow := w("shadow", strings.Join([]string{
		"root:" + systemHashes["sha512"] + ":19000:0:99999:7:::",
		"daemon:*:19000:0:99999:7:::",
		"alice:" + systemHashes["yescrypt"] + ":19990:0:99999:7:::",
		"bob:" + systemHashes["sha512"] + ":19990:0:99999:7:::",
		"carol:" + systemHashes["sha256"] + ":19990::::::",
		"locked:!" + systemHashes["sha512"] + ":19990:0:99999:7:::",
		"expired:" + systemHashes["sha512"] + ":19990:0:99999:7::19999:",
		"oldpw:" + systemHashes["sha512"] + ":19000:0:90:7:30::",
	}, "\n")+"\n")
	group := w("group", "users:x:100:\ndialup:x:20:bob,alice\nalice:x:1000:\n")

	newAuth := func(vals map[string]string) *systemAuth {
		vals["passwd_file"], vals["shadow_file"], vals["group_file"] = passwd, shadow, group
		a, err := newSystemAuth(&section{name: "system", vals: vals})
		if err != nil {
			t.Fatal(err)
		}
		sa := a.(*systemAuth)
		sa.now = func() time.Time { return time.Unix(20000*86400+3600, 0) }
		return sa
	}
	ctx := context.Background()
	a := newAuth(map[string]string{})
	for _, c := range []struct {
		user, pw string
		err      error
	}{
		{"alice", "pw123", nil}, // yescrypt
		{"alice", "wrong", errBadPassword},
		{"bob", "pw123", nil},        // sha512
		{"carol", "pw123", nil},      // sha256, no ageing
		{"legacy", "pw123", nil},     // a hash in passwd itself
		{"root", "pw123", errNoUser}, // not without allow_root
		{"daemon", "pw123", errNoUser},
		{"nobody", "pw123", errNoUser},
		{"locked", "pw123", errBadPassword},
		{"expired", "pw123", errBadPassword},
		{"oldpw", "pw123", errBadPassword},
	} {
		err := a.authenticate(ctx, c.user, c.pw)
		if c.err == nil && err != nil || c.err != nil && !errors.Is(err, c.err) {
			t.Errorf("%s/%s: %v, want %v", c.user, c.pw, err, c.err)
		}
	}
	if err := newAuth(map[string]string{"allow_root": "yes"}).authenticate(ctx, "root", "pw123"); err != nil {
		t.Errorf("allow_root: %v", err)
	}

	// groups: dialup by membership, users by the primary group
	g := newAuth(map[string]string{"groups": "dialup users"})
	for user, want := range map[string]error{"alice": nil, "bob": nil, "carol": nil, "legacy": errNoUser} {
		if err := g.authenticate(ctx, user, "pw123"); want == nil && err != nil || want != nil && !errors.Is(err, want) {
			t.Errorf("groups, %s: %v, want %v", user, err, want)
		}
	}
	if _, err := newSystemAuth(&section{name: "system", vals: map[string]string{"method": "kerberos"}}); err == nil {
		t.Error("method = kerberos: no error")
	}
}
