// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

// Package samples has the sample configuration in the program, for the
// help of modemmenu -setup's screens.
package samples

import _ "embed"

// Conf is modemmenu.conf.sample.
//
//go:embed modemmenu.conf.sample
var Conf string
