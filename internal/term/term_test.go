// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package term

import "testing"

// TestParseVT: names, TERMs and levels from real terminals' answers
func TestParseVT(t *testing.T) {
	for _, c := range []struct {
		what, reply, name, term string
		level                   int
	}{
		{"xterm", "\x1b[?64;1;2;6;9;15;18;21;22c\x1b[>41;390;0c\x1bP>|XTerm(390)\x1b\\\x1b[50;132R", "XTerm(390)", "xterm", 4},
		{"xterm without XTVERSION", "\x1b[?65;1;9c\x1b[>41;390;0c\x1b[24;80R", "xterm (patch 390)", "xterm", 5},
		{"a VT220", "\x1b[?62;1;2;6;8;9;15c\x1b[>1;10;0c\x1b[24;80R", "VT220", "vt220", 2},
		{"a VT520", "\x1b[?65;1;2;6;7;8;9;12;18;19;21;22;23;24;42;44;45;46c\x1b[>64;20;0c\x1b[24;80R", "VT520", "vt520", 5},
		{"a VT100", "\x1b[?1;2c\x1b[24;80R", "VT100", "vt100", 1},
		{"a VT102", "\x1b[?6c\x1b[24;80R", "VT102", "vt102", 1},
		{"SyncTERM", "\x1b[=67;84;101;114;109;1;316c\x1b[24;80R", "SyncTERM (CTerm 1.316)", "syncterm", 0},
		{"VTE", "\x1b[?65;1;9c\x1b[>65;6800;1c\x1b[24;80R", "VTE", "xterm", 5},
		{"an answerback", "MYTERM\x1b[?62;1c\x1b[24;80R", "MYTERM", "vt220", 2},
		{"nothing said", "\x1b[24;80R", "", "ansi", 0},
	} {
		in := ParseVT([]byte(c.reply))
		if in.Name() != c.name || in.Term() != c.term || in.Level() != c.level {
			t.Errorf("%s: %q %q level %d, want %q %q %d", c.what, in.Name(), in.Term(), in.Level(), c.name, c.term, c.level)
		}
	}
}

// TestPETSCIIText: the menu's text as PETSCII, and a Commodore's keys
// as ASCII
func TestPETSCIIText(t *testing.T) {
	if got := string(PETSCIIOut("Hi you\r\n\x1b[1;33mX\b \b\n")); got != "\xc8I YOU\r\xd8\x9d \x9d\r" {
		t.Errorf("out %q", got)
	}
	var in []byte
	for _, c := range []byte("HI\xc8\x14\r") {
		in = append(in, PETSCIIIn(c))
	}
	if string(in) != "hiH\x7f\r" {
		t.Errorf("in %q", in)
	}
}
