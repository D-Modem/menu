// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
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

// terminalSettings: [terminal]
type terminalSettings struct {
	detect  bool   // ask: ANSI, VT52, an ANSI terminal's identity
	petscii string // auto (a terminal that is not ANSI may be one), yes (every caller), no
	timeout time.Duration
}

func loadTerminal(sec *section) (terminalSettings, error) {
	var err error
	ts := terminalSettings{
		petscii: strings.ToLower(sec.get("petscii", "auto")),
	}
	if ts.detect, err = sec.bool("detect", true); err != nil {
		return ts, err
	}
	if ts.timeout, err = sec.duration("timeout", 2*time.Second); err != nil {
		return ts, err
	}
	switch ts.petscii {
	case "auto", "yes", "no":
	default:
		return ts, fmt.Errorf("[terminal] petscii: %q (auto, yes or no)", ts.petscii)
	}
	return ts, nil
}

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

// vtInfo: what an ANSI terminal said of itself
type vtInfo struct {
	answerback string // ENQ's answer
	da1        []int  // the class (1, 6, 62-65) and options
	da2        []int  // the model, its firmware version, options
	version    string // XTVERSION's "name(version)"
	cterm      string // SyncTERM's CTerm version
}

func parseVT(reply []byte) vtInfo {
	var in vtInfo
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

func printable(s string, max int) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r < 0x7f && b.Len() < max {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// level: the VT level: 1 a VT100 or VT102, 2 a VT220, 3 VT320, 4 VT420,
// 5 VT510 and later; 0 if it did not say
func (in vtInfo) level() int {
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

// hardware: a DEC terminal by its DA2 (a model of DEC's, a small firmware
// version), not an emulator that says its name otherwise
func (in vtInfo) hardware() bool {
	if in.version != "" || in.cterm != "" || len(in.da2) < 2 {
		return false
	}
	_, dec := decModels[in.da2[0]]
	return dec && in.da2[1] < 95
}

// emulator: a program on a computer (xterm, VTE, SyncTERM...), not a VT
func (in vtInfo) emulator() bool {
	if in.version != "" || in.cterm != "" {
		return true
	}
	if len(in.da2) >= 2 && !in.hardware() {
		p, v := in.da2[0], in.da2[1]
		return emulators[p] != "" || p == 41 && v >= 95 || (p == 1 || p == 65) && v >= 1000
	}
	return false
}

// name: what the terminal is, as best it said
func (in vtInfo) name() string {
	switch {
	case in.version != "":
		return in.version
	case in.cterm != "":
		return "SyncTERM (CTerm " + in.cterm + ")"
	case in.hardware():
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
	switch l := in.level(); {
	case l == 1 && in.da1[0] == 6:
		return "VT102"
	case l == 1:
		return "VT100"
	case l >= 2:
		return fmt.Sprintf("VT%d00 class", l)
	}
	return ""
}

// term: a TERM for it: syncterm, xterm for the other emulators, a VT's
// own (vt100, vt220...), ansi when it did not say
func (in vtInfo) term() string {
	switch {
	case in.cterm != "":
		return "syncterm"
	case in.emulator():
		return "xterm"
	case in.hardware():
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
	switch l := in.level(); {
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

// petsciiTelltale: a byte only a Commodore's keys send: DEL, the cursor
// keys, CLR, the function keys, a shifted letter
func petsciiTelltale(b byte) bool {
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

// prober: the questions of one call, and what came that was no answer
// (what the caller typed, a PPP frame) to be read again after
type prober struct {
	t     *term
	extra []byte
	ppp   bool // a PPP frame came: ask nothing more
}

// collect what comes until done, max bytes or timeout
func (p *prober) collect(timeout time.Duration, max int, done func([]byte) bool) ([]byte, error) {
	var got []byte
	end := time.Now().Add(timeout)
	for len(got) < max {
		left := time.Until(end)
		if left <= 0 {
			break
		}
		c, err := p.t.readByte(left)
		if errors.Is(err, errTimeout) {
			break
		}
		if err != nil {
			return got, err
		}
		got = append(got, c)
		if done(got) {
			break
		}
		if n := len(got); n >= 3 && pppFrame(got[n-3:]) {
			break // a dialer's PPP: no answer will come
		}
	}
	return got, nil
}

// keep what is no answer to the questions
func (p *prober) keep(b []byte) {
	for _, re := range answers {
		b = re.ReplaceAll(b, nil)
	}
	p.extra = append(p.extra, b...)
	for i := range p.extra {
		if pppFrame(p.extra[i:]) {
			p.ppp = true
		}
	}
}

// done: what was no answer, read again by the login
func (p *prober) done() { p.t.unread(p.extra...) }

// ansi: the position after the cursor went as far as it goes: whether the
// terminal answered, and its size
func (p *prober) ansi(timeout time.Duration) (bool, int, int, error) {
	if err := p.t.writeRaw(queryANSI); err != nil {
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
		return false, 0, 0, p.t.writeRaw("\r" + strings.Repeat(" ", 20) + "\r")
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

// vt52: whether ESC Z is answered as a VT52 answers it; any other shows
// a Z, which is wiped
func (p *prober) vt52(timeout time.Duration) (bool, error) {
	if err := p.t.writeRaw(queryVT52); err != nil {
		return false, err
	}
	got, err := p.collect(timeout, 16, vt52Pattern.Match)
	if err != nil {
		return false, err
	}
	is := vt52Pattern.Match(got)
	p.keep(got)
	if !is {
		return false, p.t.writeRaw("\r \r")
	}
	return true, nil
}

// identify: what an ANSI terminal says it is
func (p *prober) identify(timeout time.Duration) (vtInfo, error) {
	if err := p.t.writeRaw(queryVT); err != nil {
		return vtInfo{}, err
	}
	got, err := p.collect(timeout, 1024, cprPattern.Match)
	if err != nil {
		return vtInfo{}, err
	}
	in := parseVT(got)
	// the answerback is no key the caller typed
	if i := bytes.IndexByte(got, 0x1b); i > 0 && in.answerback != "" {
		got = got[i:]
	}
	p.keep(got)
	return in, nil
}

// detectTerminal: what the caller's terminal is, in sess (Terminal, Term,
// TermName, Rows, Cols), the line set for it
func detectTerminal(t *term, s *settings, sess *session) error {
	ts := s.terminal
	defer func() {
		if sess.Terminal == "petscii" {
			t.setPETSCII()
		}
		if sess.Rows != "" && sess.Cols != "" {
			rows, _ := strconv.Atoi(sess.Rows)
			cols, _ := strconv.Atoi(sess.Cols)
			// the line's size, for what a selection runs on it
			unix.IoctlSetWinsize(t.in, unix.TIOCSWINSZ, &unix.Winsize{Row: uint16(rows), Col: uint16(cols)})
		}
	}()
	switch {
	case sess.Terminal != "":
		// an upstream menu said
		return nil
	case ts.petscii == "yes":
		sess.Terminal, sess.Term, sess.Rows, sess.Cols = "petscii", "dumb", "25", "40"
		return nil
	}
	p := &prober{t: t}
	defer p.done()
	if !ts.detect || p.ppp {
		// not asked: perhaps a Commodore, whose keys will tell
		t.watchPETSCII = ts.petscii == "auto" && !p.ppp
		return nil
	}
	ansi, rows, cols, err := p.ansi(ts.timeout)
	if err != nil || p.ppp {
		return err
	}
	if !ansi {
		is52, err := p.vt52(ts.timeout / 2)
		if err != nil || p.ppp {
			return err
		}
		if is52 {
			sess.Terminal, sess.Term, sess.TermName, sess.Rows, sess.Cols = "vt52", "vt52", "VT52", "24", "80"
			return nil
		}
		sess.Terminal, sess.Term = "ascii", "dumb"
		// perhaps a Commodore: its keys will tell
		t.watchPETSCII = ts.petscii == "auto"
		return nil
	}
	sess.Terminal, sess.Term = "ansi", "ansi"
	if rows > 0 && cols > 0 {
		sess.Rows, sess.Cols = strconv.Itoa(rows), strconv.Itoa(cols)
	}
	in, err := p.identify(ts.timeout)
	if err != nil {
		return err
	}
	if n := in.name(); n != "" {
		sess.TermName = n
	}
	sess.Term = in.term()
	if in.level() > 0 || in.emulator() {
		sess.Terminal = "vt"
	}
	return nil
}

// becomePETSCII: the caller has a Commodore after all
func becomePETSCII(t *term, sess *session) {
	t.watchPETSCII = false
	sess.Terminal, sess.Term, sess.Rows, sess.Cols = "petscii", "dumb", "25", "40"
	unix.IoctlSetWinsize(t.in, unix.TIOCSWINSZ, &unix.Winsize{Row: 25, Col: 40})
	t.setPETSCII()
	logf("terminal: a Commodore (PETSCII) %v", sess)
}

// settlePETSCII: a name typed all in capitals, while it is not known
// whether the terminal is a Commodore: a Commodore's unshifted keys, or
// ASCII in capitals, and DEL tells which.  The name as the caller meant
// it.
func settlePETSCII(t *term, s *settings, sess *session, name string, deadline time.Time) (string, error) {
	t.watchPETSCII = false
	if strings.ToUpper(name) != name || strings.ToLower(name) == name {
		return name, nil // lower case, or no letters
	}
	if err := t.write("PRESS DEL (OR BACKSPACE): "); err != nil {
		return "", err
	}
	wait := min(30*time.Second, time.Until(deadline))
	c, err := t.readByte(wait)
	if err != nil && !errors.Is(err, errTimeout) {
		return "", err
	}
	t.write("\r\n")
	if c != 0x14 {
		return name, nil
	}
	becomePETSCII(t, sess)
	// its unshifted letters are lower case
	name = strings.ToLower(name)
	t.write(s.loginPrompt + name + "\r\n")
	return name, nil
}

// describeTerminal: for the log
func (s *session) describeTerminal() string {
	d := s.Terminal
	if s.TermName != "" {
		d += " (" + s.TermName + ")"
	}
	if s.Rows != "" {
		d += " " + s.Cols + "x" + s.Rows
	}
	return d
}

// petsciiOut: what the menu writes, for a Commodore in its lower/upper
// case set: the letters' cases swapped as that set has them, a line end
// as RETURN, backspace as cursor left, escape sequences and what it has
// not dropped
func petsciiOut(s string) []byte {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == 0x1b:
			// an escape sequence: CSI's parameters to its final byte, or
			// one character
			if i+1 < len(s) && s[i+1] == '[' {
				i += 2
				for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
					i++
				}
			} else {
				i++
			}
		case c == '\r':
			out = append(out, 0x0d)
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
		case c == '\n':
			out = append(out, 0x0d)
		case c == 0x08:
			out = append(out, 0x9d)
		case c >= 'a' && c <= 'z':
			out = append(out, c-'a'+0x41)
		case c >= 'A' && c <= 'Z':
			out = append(out, c-'A'+0xc1)
		case c == '{':
			out = append(out, '(')
		case c == '}':
			out = append(out, ')')
		case c == '|':
			out = append(out, '!')
		case c == '~':
			out = append(out, '-')
		case c == '`':
			out = append(out, '\'')
		case c == '\\':
			out = append(out, '/')
		case c == '_':
			out = append(out, '-')
		case c >= 0x20 && c < 0x7f:
			out = append(out, c)
		case c >= 0xc0:
			// a rune of UTF-8: one character the C64 has not
			out = append(out, '?')
		}
	}
	return out
}

// petsciiIn: a Commodore's key as ASCII: its unshifted letters lower
// case, its shifted ones upper case, DEL as DEL
func petsciiIn(c byte) byte {
	switch {
	case c >= 0x41 && c <= 0x5a:
		return c - 0x41 + 'a'
	case c >= 0xc1 && c <= 0xda:
		return c - 0xc1 + 'A'
	case c >= 0x61 && c <= 0x7a:
		return c - 0x61 + 'A'
	case c == 0x14:
		return 0x7f
	}
	return c
}
