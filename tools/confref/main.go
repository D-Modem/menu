// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

// Command confref writes docs/reference.md, the configuration reference:
// every section and key of modemmenu.conf, with the help, the sample's
// value and the other values the sample shows, all read from the comments
// of samples/modemmenu.conf.sample.  From the repository's root:
//
//	go run ./tools/confref
//
// TestSampleHasEveryKey (cmd/modemmenu/reference_test.go) keeps the sample complete: a key
// the code reads that the sample doesn't show fails it.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/D-Modem/menu/internal/confdoc"
)

type kind = confdoc.Kind

func header(k *kind) string {
	if k.Named {
		return "[" + k.Name + " NAME]"
	}
	return "[" + k.Name + "]"
}

func anchor(k *kind) string {
	return strings.ToLower(strings.ReplaceAll(strings.Trim(header(k), "[]"), " ", "-"))
}

func cell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "<", "&lt;")
	return strings.Join(strings.Fields(s), " ")
}

func code(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	if strings.Contains(s, "`") {
		return "`` " + s + " ``"
	}
	return "`" + s + "`"
}

func prose(s string) string {
	s = strings.TrimSpace(s)
	if s != "" && !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, ")") && !strings.HasSuffix(s, ":") {
		s += "."
	}
	return s
}

func reference(kinds []*kind) string {
	var b strings.Builder
	b.WriteString(`<!-- Made by go run ./tools/confref from samples/modemmenu.conf.sample: edit the sample, not this page. -->

# Configuration reference

Every section and key of modemmenu.conf, from the comments in
[modemmenu.conf.sample](https://github.com/D-Modem/menu/blob/main/samples/modemmenu.conf.sample).
**Sample** is the value the sample sets (or shows, commented out);
[Configuration](configuration.md) explains the file and its sections in
full.

| section | what it is |
|---|---|
`)
	for _, k := range kinds {
		help := k.Help
		if i := strings.Index(help, ". "); i >= 0 {
			help = help[:i+1]
		}
		fmt.Fprintf(&b, "| [`%s`](#%s) | %s |\n", header(k), anchor(k), cell(prose(help)))
	}
	for _, k := range kinds {
		fmt.Fprintf(&b, "\n## `%s`\n\n", header(k))
		if k.Help != "" {
			b.WriteString(prose(k.Help) + "\n\n")
		}
		if k.Named {
			b.WriteString("There may be any number of these, each with its own NAME.\n\n")
		}
		if k.Free != "" {
			fmt.Fprintf(&b, "Each key is %s.\n", k.Free)
		}
		if len(k.Keys) == 0 {
			continue
		}
		b.WriteString("| key | sample | what it does |\n|---|---|---|\n")
		for _, x := range k.Keys {
			help := prose(x.Help)
			if help == "" {
				help = "See the section above."
			}
			if len(x.Examples) > 0 {
				var ex []string
				for _, e := range x.Examples {
					ex = append(ex, code(e))
				}
				help += " Also: " + strings.Join(ex, ", ") + "."
			}
			v := ""
			if x.Value != "" {
				v = code(x.Value)
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s |\n", x.Name, v, cell(help))
		}
	}
	b.WriteString("\n**See also:** [Configuration](configuration.md)\n")
	return b.String()
}

func main() {
	sample, err := os.ReadFile("samples/modemmenu.conf.sample")
	if err != nil {
		log.Fatal(err)
	}
	out := "docs/reference.md"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.WriteFile(out, []byte(reference(confdoc.Parse(string(sample)))), 0o644); err != nil {
		log.Fatal(err)
	}
}
