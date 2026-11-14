# Running it from app_dmodem


`app_dmodem` is the Asterisk module of
[D-Modem](https://github.com/D-Modem/D-Modem) (`frontend/asterisk/`).
It gives the line of the caller to modemmenu in one of two ways:

- It starts modemmenu for each call (`mode = exec`).
- It connects each call to one `modemmenu -serve` on a unix socket
  (`mode = unix`, refer to the section below).

## A program for each call

app_dmodem runs modemmenu as a `mode = exec` program on the line of the
caller:

```
; dmodem.conf
[menu]
mode = exec
command = exec /usr/local/bin/modemmenu -config /etc/asterisk/modemmenu.conf

; extensions.conf
exten => 5551000,1,DModem(menu)
 same => n,Hangup()
```

modemmenu reads the details of the call from the environment of
app_dmodem. The details are `DMODEM_RATE` (tx/rx bit/s),
`DMODEM_CALLERID`, `DMODEM_DIALED`, `DMODEM_CHANNEL`, `DMODEM_PROFILE`
and `DMODEM_PEER` (the SIP address of the call). For `DMODEM_DIALED`,
the app_dmodem must be a version that sets it. The `-rate`, `-callerid`
and `-dialed` options can set these values in place of the environment.
For example, use them under a getty on a different modem.

The line is raw (the pty of app_dmodem). modemmenu itself echoes and
edits the text that the caller types. When modemmenu exits, the call
hangs up. The same occurs when the selection exits.

## One server on a unix socket

`modemmenu -serve` listens on `[server] listen` (a unix socket, or
`tcp:host:port`). app_dmodem connects each call to it:

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

A socket has no environment. Thus `header = yes` in app_dmodem sends
the details of the call first. The header has these items, in this
sequence:

1. `DMODEM_CHANNEL`, `DMODEM_PROFILE`, `DMODEM_RATE`,
   `DMODEM_CALLERID`, `DMODEM_DIALED`, `DMODEM_CONNECTED` (Unix time)
   and `DMODEM_PEER`, one `NAME=value` on each line.
2. An empty line.
3. The bytes from the caller.

`[server] header = yes` (the default) reads the header. With
`header = no`, the bytes of the caller come from the start, and the
details are unknown.

Each connection gets its own session. The session is a new pty with
modemmenu on it, the same as with `mode = exec`. Thus pppd, shells and
`^C` operate in the same way. When the caller hangs up, the pty hangs
up. When the session ends, it closes the connection, and the call hangs
up.

If many callers call at the same time, there are many connections.
app_dmodem connects each call separately. The server serves each
connection in its own session, to a maximum of `max_sessions`. Some
items send the caller to a server (`mode = unix`, `tcp`, `telnet`,
`rlogin`), for example, to a BBS. Such an item opens a new connection
for that caller from the session of the caller. Then it copies the bytes
between that connection and the line of the session.

### The handoff: the line itself, nothing copied

With `handoff = yes` in app_dmodem, the line of the modem is a pty.
The terminal of the pty comes with the header (`SCM_RIGHTS`). Then the
session runs on that terminal, as on a serial port. The menu, a shell
and pppd (which must have a tty) are on the line of the modem itself.
`-serve` copies nothing.

After the header, the connection carries no more data. It stays open
while the session continues. When the caller hangs up, the connection
closes and the session hangs up (`SIGHUP`). When the session ends, it
closes the connection. Then app_dmodem hangs up after the last bytes
go out.

An item with `handoff = yes` (`mode = unix`, `header = yes`) does the
same to its server:

1. The line of the session goes to the server with the header.
2. The menu waits.
3. When the server closes the connection, the line comes back to the
   menu. `after = menu` shows the menu again.

The `[listen]` of the BBS with `handoff = yes` accepts the line. Bytes
that the menu read but did not use, and the `send` of the item, come on
the connection immediately after the header. The time limit continues to
apply. At the deadline, the menu closes the connection and gets the line
back.

```
; dmodem.conf
[menu]
mode = unix
path = /run/modemmenu/modemmenu.sock
header = yes
handoff = yes

; modemmenu.conf
[item bbs]
key = B
mode = unix
path = /run/bbs/handoff.sock
header = yes
handoff = yes
after = menu
```

Without `handoff`, or on TCP, modemmenu copies the bytes, as described above.

| `[server]` option | default | |
|---|---|---|
| `listen` | `/run/modemmenu/modemmenu.sock` | the path of a unix socket, or `tcp:host:port` |
| `socket_mode` | `0660` | the permissions of the socket: the users that can connect |
| `socket_group` | | the group of the socket, for example, the group of Asterisk |
| `header` | `yes` | read the session header first |
| `header_timeout` | `10` | the maximum time in seconds until the header comes |
| `trust_user` | `no` | the `MODEM_USER` of a header is already logged in (refer to the section below) |
| `max_sessions` | `32` | the maximum number of sessions at the same time. More callers hear that the lines are busy |

`-listen PATH` overrides `listen`. `SIGTERM` hangs up all sessions,
removes the socket and stops the server. If a server stopped
unexpectedly and left an old socket, the new server replaces that socket
when it starts. It does not replace a socket that is in use.
[modemmenu.service.sample](../samples/modemmenu.service.sample) runs the
server under systemd. systemd also makes `/run/modemmenu`. modemmenu
records each connection in the log, with the pid and uid of its peer.

## On FreePBX

[freepbx/](../freepbx/README.md) is a configuration that is ready to use
for FreePBX 16 and 17 with the FreePBX module of D-Modem. It has these
parts:

- Logins from a users file.
- One `modemmenu -serve` under systemd.
- The D-Modem profile and the inbound route that send calls to the
  server.

`make install-freepbx` installs it.
