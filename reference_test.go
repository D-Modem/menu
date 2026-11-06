// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestSampleHasEveryKey: each key the code reads is in
// modemmenu.conf.sample, so it has help there and a line in the
// configuration reference (go run ./tools/confref makes docs/reference.md
// from the sample).
func TestSampleHasEveryKey(t *testing.T) {
	sample, err := os.ReadFile("modemmenu.conf.sample")
	if err != nil {
		t.Fatal(err)
	}
	inSample := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^;?\s*([a-z][a-z0-9_]*)\s*=`).FindAllSubmatch(sample, -1) {
		inSample[string(m[1])] = true
	}
	re := regexp.MustCompile(`\.(?:get|has|bool|int|duration|durations)\("([a-z0-9_]+)"`)
	files, _ := filepath.Glob("*.go")
	var missing []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			if !inSample[m[1]] {
				missing = append(missing, f+": "+m[1])
				inSample[m[1]] = true
			}
		}
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("not in modemmenu.conf.sample: %s", m)
	}
}
