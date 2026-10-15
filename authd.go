package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// The auth socket: modemmenu's backends (file, ldap, sql, system) asked by
// a program on this machine for a user's password, so that the BBS's
// other logins (telnet, SSH, the web) take the same one.  One request a
// connection, a JSON line each way:
//
//	{"user": "alice", "password": "..."}
//	{"ok": true, "auth": "ldap"}      or      {"ok": false, "error": "denied"}
//
// The socket's mode and group say who may ask; allow_users narrows it to
// those users (by the peer's uid).  A refusal comes after fail_delay, as at
// the login prompt; user_pattern is kept.

// authdSettings: [authd]
type authdSettings struct {
	listen      string
	socketMode  os.FileMode
	socketGroup string
	allowUIDs   map[uint32]bool // nil: whoever can connect
	maxClients  int
}

// authRequest and authReply are the socket's JSON.
type authRequest struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

type authReply struct {
	OK    bool   `json:"ok"`
	Auth  string `json:"auth,omitempty"`  // the backend that took it
	Error string `json:"error,omitempty"` // denied, refused, bad request
}

const maxAuthRequest = 4096

func loadAuthd(sec *section) (*authdSettings, error) {
	if sec.get("listen", "") == "" {
		return nil, nil
	}
	a := &authdSettings{listen: sec.get("listen", ""), socketGroup: sec.get("socket_group", "")}
	if !strings.HasPrefix(a.listen, "/") {
		return nil, fmt.Errorf("[authd] listen: %q (an absolute path: a unix socket only, passwords cross it)", a.listen)
	}
	mode, err := strconv.ParseUint(sec.get("socket_mode", "0660"), 8, 32)
	if err != nil || mode > 0o777 {
		return nil, fmt.Errorf("[authd] socket_mode: %q (octal, e.g. 0660)", sec.get("socket_mode", ""))
	}
	if mode&0o007 != 0 {
		return nil, fmt.Errorf("[authd] socket_mode: %04o lets anyone ask for passwords", mode)
	}
	a.socketMode = os.FileMode(mode)
	if names := fields(sec.get("allow_users", "")); len(names) > 0 {
		a.allowUIDs = map[uint32]bool{}
		for _, n := range names {
			u, err := user.Lookup(n)
			if err != nil {
				return nil, fmt.Errorf("[authd] allow_users: %s: %v", n, err)
			}
			uid, _ := strconv.ParseUint(u.Uid, 10, 32)
			a.allowUIDs[uint32(uid)] = true
		}
	}
	if a.maxClients, err = sec.int("max_clients", 16); err != nil {
		return nil, err
	}
	if a.maxClients < 1 {
		return nil, fmt.Errorf("[authd] max_clients: %d", a.maxClients)
	}
	return a, nil
}

// authd: modemmenu -authd, the auth socket alone, until SIGTERM.
func authd(s *settings) int {
	if s.authd == nil {
		fmt.Fprintln(os.Stderr, "modemmenu: -authd: no [authd] listen")
		return 2
	}
	ln, err := listenAuth(s.authd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "modemmenu:", err)
		return 1
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		sig := <-ch
		logf("%v: stopping", sig)
		ln.Close()
	}()
	serveAuth(s, ln)
	return 0
}

func listenAuth(a *authdSettings) (net.Listener, error) {
	ln, err := listen(&serverSettings{listen: a.listen, socketMode: a.socketMode, socketGroup: a.socketGroup})
	if err != nil {
		return nil, fmt.Errorf("[authd] %v", err)
	}
	logf("auth socket on %s", a.listen)
	return ln, nil
}

// serveAuth answers on ln until it is closed.
func serveAuth(s *settings, ln net.Listener) {
	slots := make(chan struct{}, s.authd.maxClients)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			logf("auth accept: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		select {
		case slots <- struct{}{}:
		default:
			logf("auth: %d requests at once already, refused", s.authd.maxClients)
			conn.Close()
			continue
		}
		go func() {
			defer func() { <-slots }()
			answerAuth(s, conn)
		}()
	}
}

// peerUID is the uid of a unix socket's peer.
func peerUID(conn net.Conn) (uint32, bool) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, false
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, false
	}
	var cred *unix.Ucred
	raw.Control(func(fd uintptr) {
		cred, _ = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if cred == nil {
		return 0, false
	}
	return cred.Uid, true
}

func answerAuth(s *settings, conn net.Conn) {
	defer conn.Close()
	who := peer(conn)
	reply := func(r authReply) {
		b, _ := json.Marshal(r)
		conn.Write(append(b, '\n'))
	}
	if s.authd.allowUIDs != nil {
		if uid, ok := peerUID(conn); !ok || !s.authd.allowUIDs[uid] {
			logf("auth request from %s: not an allowed user", who)
			reply(authReply{Error: "refused"})
			return
		}
	}
	// one line, soon: a client that says less is not waited for
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	line, err := bufio.NewReaderSize(io.LimitReader(conn, maxAuthRequest), maxAuthRequest).ReadSlice('\n')
	conn.SetDeadline(time.Now().Add(time.Minute))
	var req authRequest
	if err != nil || json.Unmarshal(line, &req) != nil || req.User == "" {
		logf("auth request from %s: bad request", who)
		reply(authReply{Error: "bad request"})
		return
	}
	var auth string
	if s.userPattern.MatchString(req.User) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		auth, err = authenticate(ctx, s.backends, req.User, req.Password, logf)
		cancel()
	} else {
		// not a name we take: the same answer, after the same time
		burn(req.Password)
		err = errNoUser
	}
	if err != nil {
		logf("auth request from %s: user=%q denied (%v)", who, req.User, err)
		time.Sleep(s.failDelay)
		reply(authReply{Error: "denied"})
		return
	}
	logf("auth request from %s: user=%s auth=%s", who, req.User, auth)
	reply(authReply{OK: true, Auth: auth})
}
