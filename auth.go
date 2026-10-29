// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// errNoUser: the backend does not know the user (the next one is asked);
// errBadPassword: it knows the user, the password is wrong (no other
// backend is asked)
var (
	errNoUser      = errors.New("no such user")
	errBadPassword = errors.New("wrong password")
)

type authenticator interface {
	name() string
	authenticate(ctx context.Context, user, password string) error
}

// a hash compared against when the user is unknown, so that the answer
// takes as long either way
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy password"), bcrypt.DefaultCost)

// checkPassword against a stored password, in the scheme its prefix or
// scheme (auto: by prefix) names:
//
//	$2a$, $2b$, $2y$ ...    bcrypt
//	{SHA256}hex, {SHA512}hex (or base64), {SSHA256}/{SSHA512}base64(hash+salt)
//	{PLAIN}text, or text with scheme = plain
func checkPassword(stored, password, scheme string) (bool, error) {
	if scheme == "" || scheme == "auto" {
		scheme = "plain"
		switch {
		case strings.HasPrefix(stored, "$2a$"), strings.HasPrefix(stored, "$2b$"),
			strings.HasPrefix(stored, "$2y$"):
			scheme = "bcrypt"
		case strings.HasPrefix(stored, "{"):
			end := strings.IndexByte(stored, '}')
			if end < 0 {
				return false, fmt.Errorf("stored password: no } after the scheme")
			}
			scheme = strings.ToLower(stored[1:end])
			stored = stored[end+1:]
		default:
			// a bare value is plain text only when it is asked for
			return false, fmt.Errorf("stored password: unknown scheme (bcrypt, {SHA256}, {SHA512}, {SSHA256}, {SSHA512} or {PLAIN})")
		}
	}
	switch scheme {
	case "bcrypt":
		err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(password))
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return false, nil
		}
		return err == nil, err
	case "sha256":
		h := sha256.Sum256([]byte(password))
		return equalDigest(stored, h[:]), nil
	case "sha512":
		h := sha512.Sum512([]byte(password))
		return equalDigest(stored, h[:]), nil
	case "ssha256", "ssha512":
		raw, err := base64.StdEncoding.DecodeString(stored)
		size := sha256.Size
		if scheme == "ssha512" {
			size = sha512.Size
		}
		if err != nil || len(raw) <= size {
			return false, fmt.Errorf("stored password: bad {%s} value", strings.ToUpper(scheme))
		}
		salt := append([]byte(password), raw[size:]...)
		var h []byte
		if scheme == "ssha256" {
			s := sha256.Sum256(salt)
			h = s[:]
		} else {
			s := sha512.Sum512(salt)
			h = s[:]
		}
		return subtle.ConstantTimeCompare(h, raw[:size]) == 1, nil
	case "plain":
		return subtle.ConstantTimeCompare([]byte(stored), []byte(password)) == 1, nil
	}
	return false, fmt.Errorf("unknown password scheme %q", scheme)
}

// equalDigest: a stored digest in hex or base64 against the computed one
func equalDigest(stored string, sum []byte) bool {
	if b, err := hex.DecodeString(stored); err == nil && len(b) == len(sum) {
		return subtle.ConstantTimeCompare(b, sum) == 1
	}
	if b, err := base64.StdEncoding.DecodeString(stored); err == nil && len(b) == len(sum) {
		return subtle.ConstantTimeCompare(b, sum) == 1
	}
	return false
}

// burn: the time a password check takes, for an unknown user
func burn(password string) {
	bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}

// authenticate with each backend in turn: the name of the one that
// accepted the user, or why none did
func authenticate(ctx context.Context, backends []authenticator, user, password string, logf func(string, ...any)) (string, error) {
	for _, a := range backends {
		err := a.authenticate(ctx, user, password)
		switch {
		case err == nil:
			return a.name(), nil
		case errors.Is(err, errBadPassword):
			return "", err
		case errors.Is(err, errNoUser):
			if err != errNoUser {
				// known, but not one this backend lets in: why
				logf("auth %s: %v", a.name(), err)
			}
			continue
		default:
			// a backend that is down does not stop the others
			logf("auth %s: %v", a.name(), err)
		}
	}
	burn(password)
	return "", errNoUser
}
