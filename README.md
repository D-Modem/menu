# modemmenu: a login and menu for dial-up callers

`modemmenu` is the first thing a caller sees once the modems are
connected.  It asks for a user name and password, checks them against a
text file, LDAP or an SQL database (MySQL/MariaDB, PostgreSQL, SQLite),
and then offers a menu:

```
Welcome to D-Modem.

login: alice
Password: ******

Hello alice, connected at 33600 bit/s on 2026-10-10 21:05:09.

Main menu

  [B] Bulletin board
  [P] PPP (dial-up networking)
  [S] Shell
  [G] Goodbye (hang up)

Selection:
```

The selection starts with the caller's credentials and the details of
the call: user name, password, the backend that accepted them, the dialed
number, the caller ID, the connect rate and the date and time of the
connection.

## Running it from app_dmodem

`app_dmodem`, the Asterisk module of
[D-Modem-RE](https://github.com/Neo-Desktop/D-Modem-RE) (`asterisk/`), runs
it as a `mode = exec` program on the caller's line:

```
; dmodem.conf
[menu]
mode = exec
command = exec /usr/local/bin/modemmenu -config /etc/asterisk/modemmenu.conf

; extensions.conf
exten => 5551000,1,DModem(menu)
 same => n,Hangup()
```

It reads the call's details from app_dmodem's environment:
`DMODEM_RATE` (tx/rx bit/s), `DMODEM_CALLERID`, `DMODEM_DIALED`,
`DMODEM_CHANNEL` and `DMODEM_PROFILE` (`DMODEM_DIALED` needs an
app_dmodem that sets it; without it, or under another getty, pass
`-dialed`).  `-rate`, `-callerid` and
`-dialed` set them instead, e.g. under a getty on another modem.
The line is raw (app_dmodem's pty): modemmenu echoes and edits what the
caller types itself.  Its exit, like the selection's, hangs up the call.

## Building

Go 1.22 or later:

```
make                    # or: go build
make test
make install            # /usr/local/bin/modemmenu, and the samples into
                        # /etc/asterisk if there are none
```

## Configuration

[modemmenu.conf.sample](modemmenu.conf.sample) has every option.  In
short:

```
[general]
auth = file ldap sql    ; the backends, asked in this order

[file]
path = /etc/asterisk/modemmenu.users

[item bbs]
key = B
label = Bulletin board
mode = rlogin
host = 127.0.0.1
after = menu

[item ppp]
key = P
label = PPP
mode = exec
command = exec /usr/sbin/pppd nodetach noauth local lock passive 192.168.77.1:192.168.77.2 ipparam {user}

[item shell]
key = S
label = Shell
mode = exec
run_as = {user}
tty = sane
command = exec "$SHELL" -l
```

`modemmenu -check -config FILE` checks a configuration.

### Authentication

A backend that does not know the user hands over to the next; one that
knows the user and finds the password wrong ends the attempt there.  A
backend that is down (LDAP unreachable, SQL error) is logged and skipped.

- **file**: `user:password` lines.  The password is a bcrypt hash
  (`modemmenu -hash` prints one), `{SHA256}`/`{SHA512}` (hex or base64),
  `{SSHA256}`/`{SSHA512}` (salted, base64) or `{PLAIN}text`.  The file is
  read at each login.
- **ldap**: a bind as the user, either to `bind_dn` with `{user}` in it,
  or to the entry `search_filter` finds under `search_base`.  ldaps://,
  StartTLS and a private CA are supported; a group check goes in the
  filter (`memberOf=...`).  Values are escaped for DNs and filters.
- **sql**: `query` with the user (and optionally the password) as
  arguments returns the stored password, checked like the file's; or with
  `hash = none` any row means the login is right, for a database that
  checks passwords itself (`crypt()` in PostgreSQL).

User names must match `user_pattern` (letters, digits, `._@-` by default);
wrong logins wait `fail_delay` seconds, unknown users take as long as
known ones, and the call is hung up after `attempts` tries or
`login_timeout`.

### What the selection gets

| placeholder | environment | |
|---|---|---|
| `{user}` | `MODEM_USER` | the user name |
| `{password}` | `MODEM_PASSWORD` | the password (not with `pass_password = no`) |
| `{auth}` | `MODEM_AUTH` | `file`, `ldap` or `sql` |
| `{dialed}` | `MODEM_DIALED` | the number dialed |
| `{callerid}` | `MODEM_CALLERID` | the caller's number |
| `{baud}` | `MODEM_BAUD` | the connect rate towards the caller, bit/s |
| `{rate}`, `{tx_rate}`, `{rx_rate}` | `MODEM_RATE`, `MODEM_TX_RATE`, `MODEM_RX_RATE` | both rates |
| `{date}`, `{time}` | `MODEM_DATE`, `MODEM_TIME` | the connection's date and time |
| `{datetime}`, `{epoch}` | `MODEM_DATETIME`, `MODEM_EPOCH` | the same, RFC 3339 and Unix time |
| `{channel}`, `{profile}` | `MODEM_CHANNEL`, `MODEM_PROFILE` | app_dmodem's channel and profile |
| `{selection}` | `MODEM_SELECTION` | the item chosen |

How it reaches each kind of selection:

- **exec** (`command`, by `/bin/sh -c`): the environment, the
  placeholders in the command (each one a shell-quoted word already, so
  `{user}` and not `"{user}"`), and with `session_file = yes` all of it
  as JSON in a 0600 file named by `MODEM_SESSION_FILE`, removed when the
  program ends.  `run_as` runs it as a system user (modemmenu must be
  root), `tty = sane` gives it a cooked terminal.
- **rlogin**: the RFC 1282 handshake; by default the password as the
  client user, the user name as the server user and `ansi/{baud}` as the
  terminal, which is how Synchronet, Mystic and other BBSs log a caller
  in without asking again.
- **tcp** and **telnet**: `send`, e.g. `send = "{user}\r{password}\r"`,
  is sent once connected; telnet negotiates binary, echo and the
  terminal type.

`after = menu` returns to the menu when the selection ends, `after =
hangup` (the default) hangs up.

## Security notes

- Asterisk usually runs as its own user: the users file (and
  modemmenu.conf, which may hold database and LDAP passwords) must be
  readable by it, and by no one else.  `run_as` needs modemmenu to run as
  root, which app_dmodem only does when Asterisk does.

- The password goes to the selection unless `pass_password = no`.  In
  the environment it is readable by the same user and root
  (`/proc/PID/environ`); `session_file` keeps it out of the environment
  if the program reads the file instead.
- Never put `{password}` in a command line: other users see the
  arguments of every process.  Use `MODEM_PASSWORD` or the session file.
- The log (syslog's auth facility by default) gets logins, failures and
  selections, never passwords.  It is never the line: the caller would
  see it.

## Tests

`make test` checks the password schemes, the file and SQLite backends
and the telnet client, and runs modemmenu on a pty as app_dmodem does: a
wrong and a right login, an rlogin BBS and back to the menu, and an exec
selection's environment and session file.
`MODEMMENU_TEST_LDAP=ldap://host:port` also tests the LDAP backend
against a server with `uid=lduser,ou=people,dc=example,dc=com`
(password `ldpass`).
