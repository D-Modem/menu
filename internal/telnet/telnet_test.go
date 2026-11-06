// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package telnet

import "testing"

// TestTelnetDecode: the telnet client's data, its replies to the
// server, and the caller's bytes encoded
func TestTelnetDecode(t *testing.T) {
	var sent []byte
	tn := &Client{conn: nil, term: "ANSI", local: map[byte]bool{}, remote: map[byte]bool{}}
	tn.conn = &recorder{b: &sent}
	in := []byte{'h', iac, will, optEcho, 'i', iac, iac, iac, do, optTType,
		iac, sb, optTType, 1, iac, se, iac, do, 31, '!'}
	out := tn.Decode(in)
	if string(out) != "hi\xff!" {
		t.Errorf("data %q", out)
	}
	want := []byte{iac, do, optEcho, iac, will, optTType, iac, sb, optTType, 0, 'A', 'N', 'S', 'I', iac, se, iac, wont, 31}
	if string(sent) != string(want) {
		t.Errorf("replies % x, want % x", sent, want)
	}
	if got := tn.Encode([]byte{'a', '\r', iac}); string(got) != "a\r\x00\xff\xff" {
		t.Errorf("encode % x", got)
	}
}

type recorder struct{ b *[]byte }

// Write: p kept
func (r *recorder) Write(p []byte) (int, error) {
	*r.b = append(*r.b, p...)
	return len(p), nil
}
