# Running it from app_dmodem


`app_dmodem`, the Asterisk module of
[D-Modem](https://github.com/D-Modem/D-Modem) (`frontend/asterisk/`),
hands it the caller's line in one of two ways: it starts modemmenu for
each call (`mode = exec`), or connects each call to one `modemmenu
-serve` on a unix socket (`mode = unix`, below).

## A program for each call

app_dmodem runs modemmenu as a `mode = exec` program on the caller's
line:

```
; dmodem.conf
[menu]
mode = exec
command = exec /usr/local/bin/modemmenu -config /etc/asterisk/modemmenu.conf

; extensions.conf
exten => 5551000,1,DModem(menu)
 same => n,Hangup()
```

modemmenu reads the call's details from app_dmodem's environment:
`DMODEM_RATE` (tx/rx bit/s), `DMODEM_CALLERID`, `DMODEM_DIALED`,
`DMODEM_CHANNEL` and `DMODEM_PROFILE`.  `DMODEM_DIALED` needs an
app_dmodem that sets it.  The `-rate`, `-callerid` and `-dialed` options
set these instead, e.g. under a getty on another modem.

The line is raw (app_dmodem's pty): modemmenu echoes and edits what the
caller types itself.  Its exit, like the selection's, hangs up the call.

## One server on a unix socket

`modemmenu -serve` listens on `[server] listen` (a unix socket, or
`tcp:host:port`) and app_dmodem connects each call to it:

```
; dmodem.conf
[menu]
mode = unix
path = /run/modemmenu/modemmenu.sock
header = yes

; modemmenu.conf
[server]
listen = /run/modemmenu/modemmenu.sock
socket_mode = 0660
socket_group = asterisk
```

A socket has no environment, so app_dmodem's `header = yes` sends the
call's details first: `DMODEM_CHANNEL`, `DMODEM_PROFILE`, `DMODEM_RATE`,
`DMODEM_CALLERID`, `DMODEM_DIALED` and `DMODEM_CONNECTED` (Unix time),
one `NAME=value` per line, then an empty line, then the caller's bytes.
`[server] header = yes` (the default) reads it; with `header = no` the
caller's bytes come from the start and the details are unknown.

Each connection gets a session of its own: a new pty with modemmenu on
it, as `mode = exec` would give it, so pppd, shells and `^C` work the
same.  The caller hanging up hangs up the pty; the session ending closes
the connection, which hangs up the call.

| `[server]` option | default | |
|---|---|---|
| `listen` | `/run/modemmenu/modemmenu.sock` | a unix socket's path, or `tcp:host:port` |
| `socket_mode` | `0660` | the socket's permissions: who may connect |
| `socket_group` | | the socket's group, e.g. Asterisk's |
| `header` | `yes` | read the session header first |
| `header_timeout` | `10` | seconds for the header to come |
| `trust_user` | `no` | a header's `MODEM_USER` is logged in already (below) |
| `max_sessions` | `32` | sessions at once; more callers hear the lines are busy |

`-listen PATH` overrides `listen`.  `SIGTERM` hangs up every session,
removes the socket and ends the server; a stale socket left by a server
that died is replaced at start, a live one is not.
[modemmenu.service.sample](../samples/modemmenu.service.sample) runs it under
systemd, which also makes `/run/modemmenu`.  Each connection is logged
with its peer's pid and uid.

## On FreePBX

[freepbx/](../freepbx/README.md) is a ready-made setup for FreePBX 16 and
17 with D-Modem's FreePBX module: logins from a users file, one
`modemmenu -serve` under systemd, and the D-Modem profile and inbound
route that send calls to it.  `make install-freepbx` installs it.
