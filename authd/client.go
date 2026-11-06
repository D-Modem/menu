// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package authd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

// Client asks the socket at Socket.
type Client struct {
	Socket  string
	Timeout time.Duration // for an answer (modemmenu waits fail_delay before a no); 0: 15s
}

// Check asks whether pw is user's password: true and the backend that
// said so, false if it is not; an error if modemmenu could not be asked
// or would not answer (refused: not a client it answers).
func (c *Client) Check(user, pw string) (bool, string, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	conn, err := net.DialTimeout("unix", c.Socket, timeout)
	if err != nil {
		return false, "", err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))
	b, _ := json.Marshal(Request{User: user, Password: pw})
	if _, err := conn.Write(append(b, '\n')); err != nil {
		return false, "", err
	}
	line, err := bufio.NewReader(conn).ReadSlice('\n')
	if err != nil {
		return false, "", fmt.Errorf("no answer: %w", err)
	}
	var r Reply
	if err := json.Unmarshal(line, &r); err != nil {
		return false, "", fmt.Errorf("an answer not understood: %q", line)
	}
	switch {
	case r.OK:
		return true, r.Auth, nil
	case r.Error == "denied":
		return false, "", nil
	case r.Error != "":
		return false, "", errors.New(r.Error)
	}
	return false, "", fmt.Errorf("an answer not understood: %q", line)
}
