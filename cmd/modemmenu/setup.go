// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/D-Modem/menu/internal/confdoc"
	"github.com/D-Modem/menu/internal/confedit"
	"github.com/D-Modem/menu/internal/term"
	"github.com/D-Modem/menu/internal/tui"
	"github.com/D-Modem/menu/samples"
	"golang.org/x/sys/unix"
)

// The sysop's setup: modemmenu.conf on a full screen, as the BBS setup
// programs of the 1990s had theirs.  Each section is a window of its
// keys, with the sample's help for the key under the bar; values are typed
// in a box or picked from a list.  Saving checks the file as -check does
// first, and keeps its comments and order: only the lines of the keys
// changed are.  modemmenu -setup runs it on the console (or over SSH);
// an [item] with mode = setup on the line of a sysop who calls in.

// kindGroups: the windows of the first one, under their headings (and
// the menus of the bar); a section the sample has that is in none goes
// under Other
var kindGroups = []struct {
	label string
	kinds []string
}{
	{"Login", []string{"general", "terminal", "callerid", "number", "guest", "signup"}},
	{"Users", []string{"file", "ldap", "sql", "system", "groups"}},
	{"Menu", []string{"menu", "item", "dropfile"}},
	{"Dial-up", []string{"ppp_pool", "autoppp", "aol"}},
	{"Server", []string{"server", "authd"}},
}

// kindTitles: a section's window's title
var kindTitles = map[string]string{
	"general": "Login and messages", "terminal": "Terminal detection", "callerid": "Caller ID",
	"number": "Numbers by caller ID", "guest": "Guest account", "signup": "New accounts (sign-up)",
	"file": "Users file", "ldap": "LDAP", "sql": "SQL database", "system": "System accounts (PAM)",
	"groups": "Groups", "menu": "The menu", "item": "Menu items", "dropfile": "Door drop files",
	"ppp_pool": "PPP addresses and DNS", "autoppp": "AutoPPP (dialers)", "aol": "AOL networks",
	"server": "Server (-serve)", "authd": "Auth socket (authd)",
}

// setupUI is a setup session: the file, what it was, the windows up.
type setupUI struct {
	d      *tui.Desk
	path   string
	doc    *confedit.Doc
	orig   *confedit.Doc // as it was read or last saved
	isNew  bool          // the file is not there yet: the sample to begin with
	kinds  []*confdoc.Kind
	byKind map[string]*confdoc.Kind
	stack  []*page
	flash  string // a word for the help box, once
	warn   bool
	size   func() (rows, cols int) // the terminal's, if it can change (nil: no)
	who    string                  // for the log
}

// page is a window: the first one, a section's, or a list of the named
// sections of a kind ([item NAME]).
type page struct {
	what string // "main", "section", "named"
	sec  string // the section ("item bbs"), or the kind of the named
	kind *confdoc.Kind
	l    *tui.List
}

// newSetup: the setup of the file at path (the sample when there is none)
func newSetup(d *tui.Desk, path string) (*setupUI, error) {
	u := &setupUI{d: d, path: path, byKind: map[string]*confdoc.Kind{}}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		b, u.isNew = []byte(samples.Conf), true
		u.flash = path + " is not there yet: this is the sample, saved there with F10."
	case err != nil:
		return nil, err
	}
	u.doc, u.orig = confedit.Parse(string(b)), confedit.Parse(string(b))
	u.kinds = confdoc.Parse(samples.Conf)
	for _, k := range u.kinds {
		u.byKind[k.Name] = k
	}
	d.Name = "modemmenu Setup"
	d.Menus = u.menus()
	u.stack = []*page{{what: "main", l: &tui.List{Title: "modemmenu Setup", MinW: 50}}}
	return u, nil
}

// groups: kindGroups, and Other for the sample's sections in none
func (u *setupUI) groups() []struct {
	label string
	kinds []string
} {
	out := append(kindGroups[:0:0], kindGroups...)
	in := map[string]bool{}
	for _, g := range kindGroups {
		for _, k := range g.kinds {
			in[k] = true
		}
	}
	var other []string
	for _, k := range u.kinds {
		if !in[k.Name] {
			other = append(other, k.Name)
		}
	}
	if len(other) > 0 {
		out = append(out, struct {
			label string
			kinds []string
		}{"Other", other})
	}
	return out
}

// menus: the bar's pull-downs
func (u *setupUI) menus() []tui.Menu {
	ms := []tui.Menu{{Label: "File", Items: []tui.MenuItem{
		{Label: "Save            F10", ID: "save"}, {Label: "Check", ID: "check"},
		{Label: "Undo all changes", ID: "revert"}, {Heading: true}, {Label: "Quit             F9", ID: "quit"}}}}
	for _, g := range u.groups() {
		m := tui.Menu{Label: g.label}
		for _, k := range g.kinds {
			if u.byKind[k] != nil {
				m.Items = append(m.Items, tui.MenuItem{Label: u.title(k), ID: "kind:" + k})
			}
		}
		ms = append(ms, m)
	}
	return append(ms, tui.Menu{Label: "Help", Items: []tui.MenuItem{{Label: "Keys", ID: "keys"}, {Label: "About", ID: "about"}}})
}

func (u *setupUI) title(kind string) string {
	if t, ok := kindTitles[kind]; ok {
		return t
	}
	return "[" + kind + "]"
}

// named: the sections of a named kind in the file ("item bbs"...)
func (u *setupUI) named(kind string) []string {
	var out []string
	for _, s := range u.doc.Sections() {
		if strings.HasPrefix(s, kind+" ") {
			out = append(out, s)
		}
	}
	return out
}

// fill: a page's entries from the file as it is now
func (u *setupUI) fill(p *page) {
	var items []tui.Entry
	switch p.what {
	case "main":
		for _, g := range u.groups() {
			items = append(items, tui.Entry{Heading: true, Label: g.label})
			for _, k := range g.kinds {
				kd := u.byKind[k]
				if kd == nil {
					continue
				}
				e := tui.Entry{ID: "kind:" + k, Label: u.title(k), Help: kd.Help, Mark: "►"}
				switch {
				case kd.Named:
					n := len(u.named(k))
					e.Value = fmt.Sprintf("%d set", n)
				case !u.doc.Has(k):
					e.Value, e.Unset = "not set", true
				case u.doc.Has(k) && hasKey(kd, "enabled"):
					v, ok := u.doc.Get(k, "enabled")
					if !ok {
						v = kd.ByKey["enabled"].Value
					}
					e.Value = map[bool]string{true: "on", false: "off"}[yes(v)]
				}
				e.Changed = u.sectionChanged(k, kd.Named)
				items = append(items, e)
			}
		}
	case "named":
		for _, s := range u.named(p.kind.Name) {
			name := strings.TrimPrefix(s, p.kind.Name+" ")
			v, _ := u.doc.Get(s, "label")
			if p.kind.Name == "number" {
				v, _ = u.doc.Get(s, "user")
			}
			items = append(items, tui.Entry{ID: "sec:" + s, Label: name, Value: v, Mark: "►",
				Changed: u.changed(s, ""), Help: "Enter opens it; Del deletes it."})
		}
		items = append(items, tui.Entry{Heading: true}, tui.Entry{ID: "add", Label: "Add a new one…",
			Help: "A new [" + p.kind.Name + " NAME]: its name first."})
	case "section":
		kd := p.kind
		if kd.Free != "" {
			for _, k := range u.doc.Keys(p.sec) {
				v, _ := u.doc.Get(p.sec, k)
				items = append(items, tui.Entry{ID: "key:" + k, Label: k, Value: v, Changed: u.changed(p.sec, k),
					Help: "Each key is " + kd.Free + ".  Del takes it out."})
			}
			items = append(items, tui.Entry{Heading: true}, tui.Entry{ID: "addkey", Label: "Add a new one…", Help: "Each key is " + kd.Free + "."})
			break
		}
		known := map[string]bool{}
		for _, k := range kd.Keys {
			known[k.Name] = true
			e := u.keyEntry(p.sec, k)
			if strings.TrimSpace(k.Help) == "" && kd.Help != "" {
				// the section's comment is the key's
				e.Help = capFirst(strings.TrimSpace(kd.Help))
			}
			items = append(items, e)
		}
		for _, k := range u.doc.Keys(p.sec) {
			if !known[k] {
				v, _ := u.doc.Get(p.sec, k)
				items = append(items, tui.Entry{ID: "key:" + k, Label: k, Value: v, Changed: u.changed(p.sec, k),
					Help: "Not a key of [" + kd.Name + "] that modemmenu knows: a mistake, or for a newer version.  Del takes it out."})
			}
		}
		if kd.Named {
			items = append(items, tui.Entry{Heading: true},
				tui.Entry{ID: "rename", Label: "Rename…", Help: "A new name for this [" + p.sec + "]."},
				tui.Entry{ID: "delete", Label: "Delete…", Help: "Take [" + p.sec + "] out of the file."})
		}
	}
	p.l.Items = items
	p.l.Fix()
}

// keyEntry: a key's line in its section's window
func (u *setupUI) keyEntry(sec string, k *confdoc.Key) tui.Entry {
	e := tui.Entry{ID: "key:" + k.Name, Label: k.Name, Changed: u.changed(sec, k.Name)}
	if v, ok := u.doc.Get(sec, k.Name); ok {
		e.Value = v
		if v == "" {
			e.Value = `""`
		}
	} else {
		e.Value, e.Unset = "(not set)", true
		if k.Value != "" {
			e.Note = "sample: " + k.Value
		}
	}
	e.Help = keyHelp(k)
	if len(choices(k)) > 1 {
		e.Mark = "↕"
	}
	return e
}

// keyHelp: a key's help as the sample has it
func keyHelp(k *confdoc.Key) string {
	h := strings.TrimSpace(k.Help)
	if h == "" {
		h = "See the comments in the sample."
	}
	h = strings.ToUpper(h[:1]) + h[1:]
	if !strings.HasSuffix(h, ".") && !strings.HasSuffix(h, ")") {
		h += "."
	}
	if k.Value != "" {
		h += "  Sample: " + k.Value
	}
	return h
}

// choiceRE: a value the help names (confdoc writes them "; `name`: what")
var choiceRE = regexp.MustCompile("`([a-z0-9_]+)`:")

// choices: the values a key takes, when the sample says (yes and no for
// a yes-or-no; the names its help gives); nil when it is anything typed
func choices(k *confdoc.Key) []string {
	if isBool(k.Value) {
		return []string{"yes", "no"}
	}
	var out []string
	seen := map[string]bool{}
	add := func(v string) {
		if v != "" && !seen[v] && !strings.ContainsAny(v, " {}/\\\"") {
			seen[v] = true
			out = append(out, v)
		}
	}
	for _, m := range choiceRE.FindAllStringSubmatch(k.Help, -1) {
		add(m[1])
	}
	if len(out) == 0 {
		return nil
	}
	add(k.Value)
	for _, e := range k.Examples {
		add(e)
	}
	return out
}

func isBool(v string) bool {
	switch strings.ToLower(v) {
	case "yes", "no", "true", "false", "on", "off":
		return true
	}
	return false
}

func yes(v string) bool {
	switch strings.ToLower(v) {
	case "yes", "true", "on", "1":
		return true
	}
	return false
}

func hasKey(k *confdoc.Kind, name string) bool { return k.ByKey[name] != nil }

// changed: whether a key (or, key "", any of a section) is not as it was
func (u *setupUI) changed(sec, key string) bool {
	if key == "" {
		if u.doc.Has(sec) != u.orig.Has(sec) {
			return true
		}
		keys := append(u.doc.Keys(sec), u.orig.Keys(sec)...)
		for _, k := range keys {
			if u.changed(sec, k) {
				return true
			}
		}
		return false
	}
	a, aok := u.doc.Get(sec, key)
	b, bok := u.orig.Get(sec, key)
	return a != b || aok != bok
}

// sectionChanged: a section's, or with named any of the kind's
func (u *setupUI) sectionChanged(kind string, named bool) bool {
	if !named {
		return u.changed(kind, "")
	}
	all := map[string]bool{}
	for _, s := range append(u.doc.Sections(), u.orig.Sections()...) {
		if strings.HasPrefix(s, kind+" ") {
			all[s] = true
		}
	}
	for s := range all {
		if u.changed(s, "") {
			return true
		}
	}
	return false
}

// changes: how many keys are not as they were
func (u *setupUI) changes() int {
	n := 0
	secs := map[string]bool{}
	for _, s := range append(u.doc.Sections(), u.orig.Sections()...) {
		secs[s] = true
	}
	for s := range secs {
		keys := map[string]bool{}
		for _, k := range append(u.doc.Keys(s), u.orig.Keys(s)...) {
			keys[k] = true
		}
		for k := range keys {
			if u.changed(s, k) {
				n++
			}
		}
		if len(keys) == 0 && u.doc.Has(s) != u.orig.Has(s) {
			n++
		}
	}
	return n
}

// frame: the desk with the windows up (open: a bar menu pulled down)
func (u *setupUI) frame(open int) *tui.Screen {
	d := u.d
	scr := d.Blank(open)
	for i, p := range u.stack {
		p.l.Depth = i
		p.l.Draw(d, scr, i == len(u.stack)-1)
	}
	top := u.stack[len(u.stack)-1]
	switch e := top.l.Selected(); {
	case u.flash != "":
		d.HelpBox(scr, "Note", u.flash, u.warn)
	case e != nil:
		d.HelpBox(scr, e.Label, e.Help, false)
	}
	status := ""
	if n := u.changes(); n > 0 || u.isNew {
		status = fmt.Sprintf("%d unsaved", n)
		if u.isNew {
			status = "not saved yet"
		}
	}
	keys := tui.Hints("F1", "Help", "Enter", "Change", "Esc", "Back", "/", "Menu")
	if top.what == "section" {
		keys = append(keys, tui.Hint{Key: "Del", Label: "Default"})
	}
	keys = append(keys, tui.Hints("F9", "Quit", "F10", "Save")...)
	d.KeyBar(scr, keys, status)
	return scr
}

// run: the setup until the sysop quits (or hangs up: the error)
func (u *setupUI) run() error {
	d := u.d
	defer d.Close()
	for {
		if u.size != nil {
			if r, c := u.size(); (r != d.Rows || c != d.Cols) && tui.OK(r, c) {
				d.Rows, d.Cols = r, c
				d.Redraw()
			}
		}
		top := u.stack[len(u.stack)-1]
		u.fill(top)
		scr := u.frame(-1)
		u.flash, u.warn = "", false
		if err := d.Show(scr); err != nil {
			return err
		}
		k, alt, err := d.Key(-1)
		if errors.Is(err, term.ErrTimeout) && u.size != nil {
			continue // at the console: no one to hang up on
		}
		if err != nil {
			return err
		}
		if alt || k == '/' || k == tui.F(12) {
			i := 0
			if alt {
				if i = d.MenuKey(k); i < 0 {
					continue
				}
			}
			id, err := d.PullDown(u.frame, i)
			if err != nil {
				return err
			}
			done, err := u.command(id)
			if done || err != nil {
				return err
			}
			continue
		}
		switch k {
		case tui.F(10):
			if _, err := u.save(); err != nil {
				return err
			}
		case tui.F(9):
			if done, err := u.quit(); done || err != nil {
				return err
			}
		case tui.KeyEsc, tui.KeyLeft:
			if len(u.stack) > 1 {
				u.stack = u.stack[:len(u.stack)-1]
				continue
			}
			if done, err := u.quit(); done || err != nil {
				return err
			}
		case tui.F(1), '?':
			if err := u.help(top); err != nil {
				return err
			}
		case tui.KeyDel, tui.KeyBackspace:
			if err := u.remove(top); err != nil {
				return err
			}
		case tui.KeyEnter, tui.KeyRight, ' ':
			if err := u.pick(top, k == ' '); err != nil {
				return err
			}
		default:
			top.l.Move(d, k)
		}
	}
}

// command: a pull-down's item; done: the setup is over
func (u *setupUI) command(id string) (bool, error) {
	switch {
	case id == "":
	case id == "save":
		_, err := u.save()
		return false, err
	case id == "check":
		err := checkConf(u.path, u.doc.String())
		if err != nil {
			_, err = u.d.Message(u.frame(-1), tui.Alert, "Check", err.Error())
			return false, err
		}
		u.flash = "The configuration checks: " + u.summary()
	case id == "revert":
		if u.changes() == 0 {
			return false, nil
		}
		ok, err := u.d.Confirm(u.frame(-1), tui.Alert, "Undo", "Undo every change since the file was read or saved?")
		if ok {
			u.doc = confedit.Parse(u.orig.String())
			u.stack = u.stack[:1]
		}
		return false, err
	case id == "quit":
		return u.quit()
	case id == "keys":
		_, err := u.d.Message(u.frame(-1), tui.Info, "Keys", setupKeys)
		return false, err
	case id == "about":
		_, err := u.d.Message(u.frame(-1), tui.Info, "About", "modemmenu Setup\n\n"+u.path+
			"\n\nThe help of each key is the sample configuration's.  Saving keeps the file's comments and order.")
		return false, err
	case strings.HasPrefix(id, "kind:"):
		u.stack = u.stack[:1]
		u.open(strings.TrimPrefix(id, "kind:"))
	}
	return false, nil
}

// setupKeys: Help > Keys
const setupKeys = "↑ ↓ PgUp PgDn Home End, or a letter: move the bar.\n" +
	"Enter: open a window, change a value (typed, or picked from a list).\n" +
	"Space: the next of a value's choices.   Del: back to the default.\n" +
	"Esc: back a window.   / or F12 or Alt and a letter: the menus.\n" +
	"F1: the help of the line.   F10: save.   F9: quit.   ^L: draw again."

// open: the window of a kind (its section's, or its named ones')
func (u *setupUI) open(kind string) {
	kd := u.byKind[kind]
	if kd == nil {
		return
	}
	if kd.Named {
		u.stack = append(u.stack, &page{what: "named", sec: kind, kind: kd,
			l: &tui.List{Title: u.title(kind), MinW: 44}})
		return
	}
	u.stack = append(u.stack, &page{what: "section", sec: kind, kind: kd,
		l: &tui.List{Title: "[" + kind + "]  " + u.title(kind), MinW: 56}})
}

// pick: Enter (or Space: the next choice) on the bar's entry
func (u *setupUI) pick(p *page, space bool) error {
	e := p.l.Selected()
	if e == nil {
		return nil
	}
	d := u.d
	switch {
	case strings.HasPrefix(e.ID, "kind:"):
		if !space {
			u.open(strings.TrimPrefix(e.ID, "kind:"))
		}
	case strings.HasPrefix(e.ID, "sec:"):
		if !space {
			s := strings.TrimPrefix(e.ID, "sec:")
			u.stack = append(u.stack, &page{what: "section", sec: s, kind: p.kind,
				l: &tui.List{Title: "[" + s + "]", MinW: 56}})
		}
	case e.ID == "add":
		name, ok, err := d.Input(u.frame(-1), "New ["+p.kind.Name+" NAME]", "Its name (one word: letters, digits, - and _"+
			map[bool]string{true: "; for a number, the caller's number", false: ""}[p.kind.Name == "number"]+"):", "", 32, false)
		if err != nil || !ok || name == "" {
			return err
		}
		if !nameRE.MatchString(name) || u.doc.Has(p.kind.Name+" "+name) {
			u.flash, u.warn = fmt.Sprintf("%q is not a name that can be had: one word, not one there already.", name), true
			return nil
		}
		sec := p.kind.Name + " " + name
		u.doc.AddSection(sec)
		if p.kind.Name == "item" {
			u.doc.Set(sec, "label", name)
			if v, ok := u.doc.Get(menuSection, "items"); ok {
				u.doc.Set(menuSection, "items", strings.TrimSpace(v+" "+name))
			}
		}
		u.stack = append(u.stack, &page{what: "section", sec: sec, kind: p.kind, l: &tui.List{Title: "[" + sec + "]", MinW: 56}})
	case e.ID == "addkey":
		name, ok, err := d.Input(u.frame(-1), "New key", "Its name (one word):", "", 32, false)
		if err != nil || !ok || name == "" {
			return err
		}
		name = strings.ToLower(name)
		if !nameRE.MatchString(name) {
			u.flash, u.warn = fmt.Sprintf("%q is not one word.", name), true
			return nil
		}
		v, ok, err := d.Input(u.frame(-1), name, p.kind.Free, "", 255, false)
		if err != nil || !ok {
			return err
		}
		u.doc.Set(p.sec, name, v)
	case e.ID == "rename":
		kind := p.kind.Name
		old := strings.TrimPrefix(p.sec, kind+" ")
		name, ok, err := d.Input(u.frame(-1), "Rename", "A new name for ["+p.sec+"]:", old, 32, false)
		if err != nil || !ok || name == "" || name == old {
			return err
		}
		if !nameRE.MatchString(name) || u.doc.Has(kind+" "+name) {
			u.flash, u.warn = fmt.Sprintf("%q is not a name that can be had: one word, not one there already.", name), true
			return nil
		}
		u.doc.RenameSection(p.sec, kind+" "+name)
		if kind == "item" {
			u.renameInItems(old, name)
		}
		p.sec = kind + " " + name
		p.l.Title = "[" + p.sec + "]"
	case e.ID == "delete":
		ok, err := d.Confirm(u.frame(-1), tui.Alert, "Delete", "Take ["+p.sec+"] out of the file?")
		if err != nil || !ok {
			return err
		}
		u.doc.RemoveSection(p.sec)
		if p.kind.Name == "item" {
			u.renameInItems(strings.TrimPrefix(p.sec, "item "), "")
		}
		u.stack = u.stack[:len(u.stack)-1]
	case strings.HasPrefix(e.ID, "key:"):
		return u.edit(p, strings.TrimPrefix(e.ID, "key:"), space)
	}
	return nil
}

// nameRE: a section's NAME, a key the sysop names
var nameRE = regexp.MustCompile(`^[A-Za-z0-9_+*#.-]{1,32}$`)

// renameInItems: an item's name in [menu] items changed (to "": out)
func (u *setupUI) renameInItems(old, name string) {
	v, ok := u.doc.Get(menuSection, "items")
	if !ok {
		return
	}
	var out []string
	for _, f := range strings.Fields(strings.ReplaceAll(v, ",", " ")) {
		if f == old {
			if name == "" {
				continue
			}
			f = name
		}
		out = append(out, f)
	}
	u.doc.Set(menuSection, "items", strings.Join(out, " "))
}

// edit: a key's value: picked from its choices, or typed
func (u *setupUI) edit(p *page, key string, space bool) error {
	d := u.d
	var k *confdoc.Key
	if p.kind != nil {
		k = p.kind.ByKey[key]
	}
	if k == nil {
		k = &confdoc.Key{Name: key}
	}
	cur, set := u.doc.Get(p.sec, key)
	cs := choices(k)
	if len(cs) > 1 {
		at := -1
		for i, c := range cs {
			if strings.EqualFold(c, cur) {
				at = i
			}
		}
		if space {
			u.doc.Set(p.sec, key, cs[(at+1)%len(cs)])
			return nil
		}
		def := "(not set: the default)"
		list := append(append([]string{}, cs...), def, "Type a value…")
		start := at
		if !set {
			start = len(cs)
		}
		i, ok, err := d.Choose(u.frame(-1), key, list, nil, max(start, 0))
		if err != nil || !ok {
			return err
		}
		switch {
		case i < len(cs):
			u.doc.Set(p.sec, key, cs[i])
			return nil
		case i == len(cs):
			u.doc.Unset(p.sec, key)
			return nil
		}
	} else if space {
		return nil
	}
	prompt := strings.TrimSpace(k.Help)
	if prompt != "" {
		prompt = strings.ToUpper(prompt[:1]) + prompt[1:]
	}
	if len(prompt) > 230 {
		prompt = prompt[:229] + "…"
	}
	if !strings.HasSuffix(prompt, ".") && prompt != "" {
		prompt += "."
	}
	if !set && k.Value != "" {
		prompt += "  Sample: " + k.Value
	}
	v, ok, err := d.Input(u.frame(-1), "["+p.sec+"] "+key, prompt+"  (Empty: not set, the default; \"\" for nothing.)", cur, 1024, false)
	if err != nil || !ok {
		return err
	}
	switch v {
	case "":
		u.doc.Unset(p.sec, key)
	case `""`:
		u.doc.Set(p.sec, key, "")
	default:
		u.doc.Set(p.sec, key, v)
	}
	return nil
}

// remove: Del on a key (its default again) or a named section (out)
func (u *setupUI) remove(p *page) error {
	e := p.l.Selected()
	if e == nil {
		return nil
	}
	switch {
	case strings.HasPrefix(e.ID, "key:"):
		u.doc.Unset(p.sec, strings.TrimPrefix(e.ID, "key:"))
	case strings.HasPrefix(e.ID, "sec:"):
		s := strings.TrimPrefix(e.ID, "sec:")
		ok, err := u.d.Confirm(u.frame(-1), tui.Alert, "Delete", "Take ["+s+"] out of the file?")
		if err != nil || !ok {
			return err
		}
		u.doc.RemoveSection(s)
		if p.kind.Name == "item" {
			u.renameInItems(strings.TrimPrefix(s, "item "), "")
		}
	}
	return nil
}

// help: F1, the whole help of the bar's line
func (u *setupUI) help(p *page) error {
	e := p.l.Selected()
	if e == nil {
		return nil
	}
	text := e.Help
	if strings.HasPrefix(e.ID, "key:") && p.kind != nil {
		if k := p.kind.ByKey[strings.TrimPrefix(e.ID, "key:")]; k != nil {
			text = keyHelp(k)
			if len(k.Examples) > 0 {
				text += "\n\nAlso: " + strings.Join(k.Examples, ", ")
			}
		}
	}
	_, err := u.d.Message(u.frame(-1), tui.Info, e.Label, text)
	return err
}

// summary: what the file sets up, for a check that passed
func (u *setupUI) summary() string {
	items := len(u.named("item"))
	return fmt.Sprintf("%d menu items.", items)
}

// save: the file checked and written; whether it was
func (u *setupUI) save() (bool, error) {
	text := u.doc.String()
	if err := checkConf(u.path, text); err != nil {
		i, err := u.d.Message(u.frame(-1), tui.Alert, "Check", "The configuration does not check:\n\n"+err.Error()+
			"\n\nmodemmenu would not start with it.", "Back", "Save anyway")
		if err != nil || i != 1 {
			return false, err
		}
	}
	if err := writeConf(u.path, text); err != nil {
		_, err = u.d.Message(u.frame(-1), tui.Alert, "Save", "Not saved: "+err.Error())
		return false, err
	}
	logf("setup: %s saved (%d changes) %s", u.path, u.changes(), u.who)
	serverChanged := u.changed("server", "")
	u.orig, u.isNew = confedit.Parse(text), false
	u.flash = "Saved to " + u.path + ".  The next call reads it."
	if serverChanged {
		u.flash += "  [server] is read when modemmenu -serve starts: restart it."
	}
	return true, nil
}

// quit: whether the setup ends (after asking to save what changed)
func (u *setupUI) quit() (bool, error) {
	if u.changes() == 0 && !u.isNew {
		return true, nil
	}
	i, err := u.d.Message(u.frame(-1), tui.Info, "Quit", "Save the changes first?", "Save", "Don't save", "Cancel")
	if err != nil {
		return true, err
	}
	switch i {
	case 0:
		saved, err := u.save()
		return saved, err
	case 1:
		return true, nil
	}
	return false, nil
}

// checkConf: whether text loads as a configuration at path would (in a
// file beside it, for its relative paths)
func checkConf(path, text string) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".modemmenu-check-*.conf")
	if err != nil {
		f, err = os.CreateTemp("", "modemmenu-check-*.conf")
		if err != nil {
			return err
		}
	}
	defer os.Remove(f.Name())
	_, err = f.WriteString(text)
	f.Close()
	if err != nil {
		return err
	}
	_, err = loadSettings(f.Name())
	if err != nil {
		return errors.New(strings.ReplaceAll(err.Error(), f.Name(), path))
	}
	return nil
}

// writeConf: text as the file at path: a new file renamed over it (its
// mode and owner kept), or written in place where its directory takes no
// new file
func writeConf(path, text string) error {
	mode := os.FileMode(0o640)
	uid, gid := -1, -1
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			uid, gid = int(sys.Uid), int(sys.Gid)
		}
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".modemmenu-*.conf")
	if err != nil {
		return os.WriteFile(path, []byte(text), mode)
	}
	tmp := f.Name()
	_, err = f.WriteString(text)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, mode)
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if uid >= 0 {
		os.Chown(tmp, uid, gid) // as it was, where we may
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// runSetupLocal: modemmenu -setup on the terminal it was started on
func runSetupLocal(path string) int {
	t := term.New(os.Stdin, os.Stdout)
	defer t.Close()
	size := func() (int, int) {
		ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
		if err != nil || ws.Row == 0 || ws.Col == 0 {
			return 24, 80
		}
		return int(ws.Row), int(ws.Col)
	}
	rows, cols := size()
	if !tui.OK(rows, cols) {
		fmt.Fprintf(os.Stderr, "modemmenu: the setup needs 40x14 at least (this is %dx%d)\n", cols, rows)
		return 1
	}
	d := &tui.Desk{Line: t, Rows: rows, Cols: cols, Charset: localCharset(path), Mono: localMono(), Shade: true}
	u, err := newSetup(d, path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "modemmenu:", err)
		return 1
	}
	u.size = size
	d.Idle = time.Minute // the size looked at again
	u.who = "on the console"
	if err := u.run(); err != nil {
		fmt.Fprintln(os.Stderr, "modemmenu:", err)
		return 1
	}
	return 0
}

// localCharset: [terminal] charset if it says, else UTF-8 where the
// locale is, a VT's lines on a VT, ASCII otherwise
func localCharset(path string) tui.Charset {
	if b, err := os.ReadFile(path); err == nil {
		if v, ok := confedit.Parse(string(b)).Get("terminal", "charset"); ok {
			if cs, ok := tui.ParseCharset(v); ok {
				return cs
			}
		}
	}
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(k); v != "" {
			v = strings.ToLower(strings.ReplaceAll(v, "-", ""))
			if strings.Contains(v, "utf8") {
				return tui.UTF8
			}
			break
		}
	}
	switch t := os.Getenv("TERM"); {
	case strings.HasPrefix(t, "vt"), strings.HasPrefix(t, "xterm"), strings.HasPrefix(t, "screen"), strings.HasPrefix(t, "tmux"):
		return tui.DEC
	case t == "linux", t == "ansi", t == "pcansi", strings.HasPrefix(t, "syncterm"):
		return tui.CP437
	}
	return tui.ASCII
}

// localMono: a terminal without colors (a VT's TERM)
func localMono() bool {
	t := os.Getenv("TERM")
	return strings.HasPrefix(t, "vt1") || strings.HasPrefix(t, "vt2") || strings.HasPrefix(t, "vt3") || t == "dumb"
}

// setupItem: an [item] of mode = setup: the setup on the caller's line
func setupItem(t *term.Term, cfg *settings, it *menuItem, s *session, deadline time.Time) error {
	d := deskForSetup(t, cfg, s)
	if d == nil {
		return t.Write("\r\nThe setup needs an ANSI terminal of 80x24 (40x14 at least).\r\n")
	}
	u, err := newSetup(d, cfg.path)
	if err != nil {
		return t.Write("\r\nThe setup cannot read " + cfg.path + ": " + err.Error() + "\r\n")
	}
	u.who = "user=" + s.User
	logf("setup: %s by user=%s", cfg.path, s.User)
	if !deadline.IsZero() && time.Until(deadline) < d.Idle {
		d.Idle = max(time.Until(deadline), time.Second)
	}
	err = u.run()
	if errors.Is(err, term.ErrTimeout) {
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return errTimeLimit
		}
		return t.Write("\r\nIdle too long: the setup ended (what was not saved is not).\r\n")
	}
	return err
}

// deskForSetup: the full screen for the setup, whether or not the menu
// is full screen
func deskForSetup(t *term.Term, cfg *settings, s *session) *tui.Desk {
	c := *cfg
	c.fullscreen = true
	return deskFor(t, &c, s)
}

// capFirst: s with its first letter a capital
func capFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// menuSection: [menu], whose items = the setup keeps up with the items'
const menuSection = "menu"
