// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package term

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// What the caller's terminal is, asked as bbs-sec asks it, before the
// banner:
//
//   - ANSI: the cursor moved as far as it goes and its position asked for
//     (CSI 6n).  An ANSI terminal answers, and the answer is its size.
//   - VT52, when not ANSI: ESC Z, which a VT52 answers with ESC / and a
//     letter.
//   - What an ANSI terminal is: DEC's questions (the answerback, DA1, DA2),
//     xterm's version (XTVERSION) and SyncTERM's CTerm attributes.
//   - PETSCII, when not ANSI: a Commodore asks nothing and answers nothing;
//     its keys tell (DEL, the cursor keys, its shifted letters), or a name
//     typed all in capitals and the DEL key pressed when asked.

// the questions, and the patterns of their answers
const (
	// the cursor saved, as far down and right as it goes, its position
	// asked, restored: the answer is the screen's size
	queryANSI = "\x1b7\x1b[999;999H\x1b[6n\x1b8"
	// the answerback (ENQ), DA1, DA2, XTVERSION, and last the cursor's
	// position, which every ANSI terminal answers: once it comes, whatever
	// else would has
	queryVT   = "\x05\x1b[c\x1b[>c\x1b[>0q\x1b[6n"
	queryVT52 = "\x1bZ"
)

var (
	da1Pattern  = regexp.MustCompile(`\x1b\[\?([0-9;]*)c`)
	da2Pattern  = regexp.MustCompile(`\x1b\[>([0-9;]*)c`)
	ctermPat    = regexp.MustCompile(`\x1b\[=([0-9;]*)c`)
	xtverPat    = regexp.MustCompile(`\x1bP>\|([^\x1b\x07]{0,80})\x1b\\`)
	cprPattern  = regexp.MustCompile(`\x1b\[(\d+);(\d+)R`)
	vt52Pattern = regexp.MustCompile(`\x1b/([A-Za-z])`)
	// all the answers, to take them out of what the caller typed
	answers = []*regexp.Regexp{da1Pattern, da2Pattern, ctermPat, xtverPat, cprPattern, vt52Pattern}
)

// VTInfo: what an ANSI terminal said of itself
type VTInfo struct {
	answerback string // ENQ's answer
	da1        []int  // the class (1, 6, 62-65) and options
	da2        []int  // the model, its firmware version, options
	version    string // XTVERSION's "name(version)"
	cterm      string // SyncTERM's CTerm version
}

// ParseVT: what an ANSI terminal answered to queryVT
func ParseVT(reply []byte) VTInfo {
	var in VTInfo
	// the answerback is what came before the first escape sequence
	ab := reply
	if i := bytes.IndexByte(ab, 0x1b); i >= 0 {
		ab = ab[:i]
	}
	in.answerback = printable(string(ab), 40)
	if m := da1Pattern.FindSubmatch(reply); m != nil {
		in.da1 = numbers(string(m[1]))
	}
	if m := da2Pattern.FindSubmatch(reply); m != nil {
		in.da2 = numbers(string(m[1]))
	}
	if m := xtverPat.FindSubmatch(reply); m != nil {
		in.version = printable(string(m[1]), 60)
	}
	if m := ctermPat.FindSubmatch(reply); m != nil {
		// "CTerm" in decimal character codes, then the version
		ns := numbers(string(m[1]))
		if len(ns) >= 5 && string([]byte{byte(ns[0]), byte(ns[1]), byte(ns[2]), byte(ns[3]), byte(ns[4])}) == "CTerm" {
			var v []string
			for _, n := range ns[5:] {
				v = append(v, strconv.Itoa(n))
			}
			if in.cterm = strings.Join(v, "."); in.cterm == "" {
				in.cterm = "?"
			}
		}
	}
	return in
}

// numbers: a sequence's ;-separated parameters (32 at most)
func numbers(s string) []int {
	var out []int
	for _, f := range strings.Split(s, ";") {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 || n > 1<<20 {
			continue
		}
		if out = append(out, n); len(out) == 32 {
			break
		}
	}
	return out
}

// printable: s's printable ASCII, at most max bytes, trimmed
func printable(s string, max int) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r < 0x7f && b.Len() < max {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// Level: the VT Level: 1 a VT100 or VT102, 2 a VT220, 3 VT320, 4 VT420,
// 5 VT510 and later; 0 if it did not say
func (in VTInfo) Level() int {
	if len(in.da1) == 0 {
		return 0
	}
	switch c := in.da1[0]; {
	case c >= 61 && c <= 69:
		return c - 60
	case c == 1 || c == 6 || c == 4 || c == 7 || c == 12:
		return 1
	}
	return 0
}

// DA2's model numbers of DEC's own terminals, and those emulators give
var (
	decModels = map[int]string{
		0: "VT100", 1: "VT220", 2: "VT240", 18: "VT330", 19: "VT340", 24: "VT320",
		28: "DECterm", 32: "VT382", 41: "VT420", 61: "VT510", 64: "VT520", 65: "VT525",
	}
	emulators = map[int]string{77: "mintty", 82: "rxvt", 83: "screen", 84: "tmux", 85: "rxvt-unicode"}
)

// Hardware: a DEC terminal by its DA2 (a model of DEC's, a small firmware
// version), not an emulator that says its name otherwise
func (in VTInfo) Hardware() bool {
	if in.version != "" || in.cterm != "" || len(in.da2) < 2 {
		return false
	}
	_, dec := decModels[in.da2[0]]
	return dec && in.da2[1] < 95
}

// Emulator: a program on a computer (xterm, VTE, SyncTERM...), not a VT
func (in VTInfo) Emulator() bool {
	if in.version != "" || in.cterm != "" {
		return true
	}
	if len(in.da2) >= 2 && !in.Hardware() {
		p, v := in.da2[0], in.da2[1]
		return emulators[p] != "" || p == 41 && v >= 95 || (p == 1 || p == 65) && v >= 1000
	}
	return false
}

// Name: what the terminal is, as best it said
func (in VTInfo) Name() string {
	switch {
	case in.version != "":
		return in.version
	case in.cterm != "":
		return "SyncTERM (CTerm " + in.cterm + ")"
	case in.Hardware():
		return decModels[in.da2[0]]
	}
	if len(in.da2) >= 2 {
		p, v := in.da2[0], in.da2[1]
		switch {
		case emulators[p] != "":
			return emulators[p]
		case p == 41 && v >= 95:
			return fmt.Sprintf("xterm (patch %d)", v)
		case (p == 1 || p == 65) && v >= 1000:
			return "VTE"
		}
	}
	if in.answerback != "" {
		return in.answerback
	}
	switch l := in.Level(); {
	case l == 1 && in.da1[0] == 6:
		return "VT102"
	case l == 1:
		return "VT100"
	case l >= 2:
		return fmt.Sprintf("VT%d00 class", l)
	}
	return ""
}

// Term: a TERM for it: syncterm, xterm for the other emulators, a VT's
// own (vt100, vt220...), ansi when it did not say
func (in VTInfo) Term() string {
	switch {
	case in.cterm != "":
		return "syncterm"
	case in.Emulator():
		return "xterm"
	case in.Hardware():
		switch m := in.da2[0]; {
		case m == 0:
			return "vt100"
		case m == 1 || m == 2:
			return "vt220"
		case m >= 18 && m <= 32:
			return "vt320"
		case m == 41:
			return "vt420"
		default:
			return "vt520"
		}
	}
	switch l := in.Level(); {
	case l == 1 && in.da1[0] == 6:
		return "vt102"
	case l == 1:
		return "vt100"
	case l == 2:
		return "vt220"
	case l == 3:
		return "vt320"
	case l == 4:
		return "vt420"
	case l >= 5:
		return "vt520"
	}
	return "ansi"
}

// PETSCIITelltale: a byte only a Commodore's keys send: DEL, the cursor
// keys, CLR, the function keys, a shifted letter
func PETSCIITelltale(b byte) bool {
	switch {
	case b == 0x14, b == 0x91, b == 0x1D, b == 0x9D, b == 0x93:
		return true
	case b >= 0x85 && b <= 0x8C:
		return true
	case b >= 0xC1 && b <= 0xDA:
		return true
	}
	return false
}

// Prober: the questions of one call, and what came that was no answer
// (what the caller typed, a PPP frame) to be read again after
type Prober struct {
	T     *Term
	extra []byte
	PPP   bool // a PPP frame came: ask nothing more
}

// collect what comes until done, max bytes or timeout
func (p *Prober) collect(timeout time.Duration, max int, done func([]byte) bool) ([]byte, error) {
	var got []byte
	end := time.Now().Add(timeout)
	for len(got) < max {
		left := time.Until(end)
		if left <= 0 {
			break
		}
		c, err := p.T.ReadOne(left)
		if errors.Is(err, ErrTimeout) {
			break
		}
		if err != nil {
			return got, err
		}
		got = append(got, c)
		if done(got) {
			break
		}
		if n := len(got); n >= 3 && IsPPPFrame(got[n-3:]) {
			break // a dialer's PPP: no answer will come
		}
	}
	return got, nil
}

// keep what is no answer to the questions
func (p *Prober) keep(b []byte) {
	for _, re := range answers {
		b = re.ReplaceAll(b, nil)
	}
	p.extra = append(p.extra, b...)
	for i := range p.extra {
		if IsPPPFrame(p.extra[i:]) {
			p.PPP = true
		}
	}
}

// Done: what was no answer, read again by the login
func (p *Prober) Done() { p.T.Unread(p.extra...) }

// ANSI: the position after the cursor went as far as it goes: whether the
// terminal answered, and its size
func (p *Prober) ANSI(timeout time.Duration) (bool, int, int, error) {
	if err := p.T.WriteRaw(queryANSI); err != nil {
		return false, 0, 0, err
	}
	got, err := p.collect(timeout, 64, cprPattern.Match)
	if err != nil {
		return false, 0, 0, err
	}
	m := cprPattern.FindSubmatch(got)
	p.keep(got)
	if m == nil {
		// a plain terminal shows the question as text: wiped
		return false, 0, 0, p.T.WriteRaw("\r" + strings.Repeat(" ", 20) + "\r")
	}
	rows, _ := strconv.Atoi(string(m[1]))
	cols, _ := strconv.Atoi(string(m[2]))
	if rows < 10 || rows > 200 {
		rows = 0
	}
	if cols < 40 || cols > 300 {
		cols = 0
	}
	return true, rows, cols, nil
}

// VT52: whether ESC Z is answered as a VT52 answers it; any other shows
// a Z, which is wiped
func (p *Prober) VT52(timeout time.Duration) (bool, error) {
	if err := p.T.WriteRaw(queryVT52); err != nil {
		return false, err
	}
	got, err := p.collect(timeout, 16, vt52Pattern.Match)
	if err != nil {
		return false, err
	}
	is := vt52Pattern.Match(got)
	p.keep(got)
	if !is {
		return false, p.T.WriteRaw("\r \r")
	}
	return true, nil
}

// Identify: what an ANSI terminal says it is
func (p *Prober) Identify(timeout time.Duration) (VTInfo, error) {
	if err := p.T.WriteRaw(queryVT); err != nil {
		return VTInfo{}, err
	}
	got, err := p.collect(timeout, 1024, cprPattern.Match)
	if err != nil {
		return VTInfo{}, err
	}
	in := ParseVT(got)
	// the answerback is no key the caller typed
	if i := bytes.IndexByte(got, 0x1b); i > 0 && in.answerback != "" {
		got = got[i:]
	}
	p.keep(got)
	return in, nil
}
