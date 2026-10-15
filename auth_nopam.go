//go:build !pam

package main

import "errors"

const havePAM = false

func pamAuthenticate(service, user, password string) error {
	return errors.New("built without PAM")
}

func pamChangePassword(service, user, old, new string, line *term) error {
	return errors.New("built without PAM")
}
