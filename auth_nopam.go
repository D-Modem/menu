// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

//go:build !pam

package main

import "errors"

const havePAM = false

// pamAuthenticate: without PAM (built without the pam tag), no login
func pamAuthenticate(service, user, password string) error {
	return errors.New("built without PAM")
}

// pamChangePassword: without PAM, no password change
func pamChangePassword(service, user, old, new string, line *term) error {
	return errors.New("built without PAM")
}
