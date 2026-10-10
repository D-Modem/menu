# modemmenu: a login and menu for dial-up callers

`modemmenu` is the first thing a caller sees once the modems are
connected.  It asks for a user name and password, checks them against a
text file, LDAP, an SQL database (MySQL/MariaDB, PostgreSQL, SQLite) or
the system's own accounts through PAM, shows the message of the day and
offers a menu:

```
Welcome to D-Modem.

login: alice
Password: ******

Hello alice, connected at 33600 bit/s on 2026-10-10 21:05:09.

The line is down for maintenance on Sunday.

Main menu

  [B] Bulletin board
  [P] PPP (dial-up networking)
  [S] Shell
  [G] Goodbye (hang up)

Selection: ppp
```

The caller picks an item by its key, its name or an alias (`p`, `ppp`
or `dialup` here), and the item starts with the caller's credentials and
the details of the call: user name, password, the backend that accepted
them, the dialed number, the caller ID, the connect rate and the date
and time of the connection.

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

modemmenu reads the call's details from app_dmodem's environment:
`DMODEM_RATE` (tx/rx bit/s), `DMODEM_CALLERID`, `DMODEM_DIALED`,
`DMODEM_CHANNEL` and `DMODEM_PROFILE`.  `DMODEM_DIALED` needs an
app_dmodem that sets it.  The `-rate`, `-callerid` and `-dialed` options
set these instead, e.g. under a getty on another modem.

The line is raw (app_dmodem's pty): modemmenu echoes and edits what the
caller types itself.  Its exit, like the selection's, hangs up the call.

## Building

Go 1.22 or later:

```
make                    # or: go build (-tags pam for PAM)
make PAM=0              # without PAM, when libpam's headers are there
make test
make install            # /usr/local/bin/modemmenu, and the samples into
                        # /etc/asterisk (and /etc/pam.d) if there are none
```

`make` builds with PAM when libpam's headers are installed
(`libpam0g-dev` on Debian and Ubuntu, `pam-devel` on Fedora).  Without
them modemmenu builds with the other backends, and `auth = system` is a
configuration error.

## Configuration

[modemmenu.conf.sample](modemmenu.conf.sample) has every option.  In
short:

```
[general]
; the backends, asked in this order
auth = file system
; the message of the day, before the menu
motd_file = /etc/motd

[file]
path = /etc/asterisk/modemmenu.users

[system]
pam_service = modemmenu
groups = dialup

[menu]
hangup = G goodbye bye logoff

[item bbs]
key = B
label = Bulletin board
mode = rlogin
host = 127.0.0.1
after = menu

[item ppp]
; p, ppp (its name) and dialup select it
key = P
aliases = dialup
label = PPP (dial-up networking)
mode = exec
command = exec /usr/sbin/pppd nodetach noauth local lock passive 192.168.77.1:192.168.77.2 ipparam {user}

[item shell]
key = S
label = Shell
mode = exec
run_as = {user}
tty = sane
pass_password = no
command = exec "$SHELL" -l
```

Comments (`;` or `#`) go on lines of their own.  `modemmenu -check
-config FILE` checks a configuration.

### Authentication

`[general] auth` lists the backends, asked in that order.  A backend
that does not know the user hands over to the next; one that knows the
user and finds the password wrong ends the attempt there.  A backend
that is down (LDAP unreachable, SQL error) is logged and skipped.

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
- **system** (or **pam**): the machine's own accounts, through PAM.  See
  below.

User names must match `user_pattern` (letters, digits, `._@-` by default);
wrong logins wait `fail_delay` seconds, unknown users take as long as
known ones, and the call is hung up after `attempts` tries or
`login_timeout`.

### System accounts (PAM)

`auth = system` runs the auth and account stacks of the PAM service
`pam_service` (`modemmenu` by default), so local accounts and anything
PAM is set up for (sssd, Kerberos, OTP modules) work, with PAM's own
checks for expired and locked accounts.  It needs a build with PAM
(above).  To set it up:

1. Install the PAM service: `make install` puts
   [modemmenu.pam.sample](modemmenu.pam.sample) in `/etc/pam.d/modemmenu`
   (the distribution's `common-auth` and `common-account`).
2. Run Asterisk, and so modemmenu, as root for local accounts:
   `pam_unix` checks other users' passwords only for root.
3. Add `system` to `[general] auth`, and in `[system]`:

   | option | default | |
   |---|---|---|
   | `pam_service` | `modemmenu` | the file in `/etc/pam.d` |
   | `min_uid` | `1000` | accounts below it (system accounts) are not let in |
   | `allow_root` | `no` | `yes` lets root log in |
   | `groups` | | only members of these groups (membership or primary group) |

An account that these rules keep out is "unknown" to this backend: the
next one is asked, and the reason is logged.  With `run_as = {user}`, a
selection such as the shell then runs as the caller's own account.

### Message of the day

After the welcome and before the menu, modemmenu shows `[general] motd`
and then the file `motd_file` (e.g. `/etc/motd`), either or both.  The
file is read at each login, so edits show at once; `{placeholders}` work
in both, e.g. `motd = "Hello {user}, you called {dialed}.\r\n"`.

### The menu

The caller types a choice and presses Enter.  An item is selected by its
`key`, its name (`[item ppp]`) or one of its `aliases`, in any case; so
with `key = P` and `aliases = dialup`, `p`, `P`, `ppp` and `dialup` all
start PPP.  A key may be longer than one character (`key = SH`); the
menu shows the key, its labels lined up.

- `?` or `menu` shows the menu again; an unknown choice says so and asks
  again.
- `[menu] hangup` lists what hangs up, `G goodbye bye logoff` by
  default; the first one is shown in the menu.
- `hotkeys = yes` selects with one key, without Enter.  Every key (and
  the first hangup word) must then be one character.
- Two items, or an item and a hangup word, that answer to the same word
  are a configuration error.
- `[menu] title`, `prompt` and `items` (the order) set the rest; see
  the sample.

### What the selection gets

| placeholder | environment | |
|---|---|---|
| `{user}` | `MODEM_USER` | the user name |
| `{password}` | `MODEM_PASSWORD` | the password (not with `pass_password = no`) |
| `{auth}` | `MODEM_AUTH` | `file`, `ldap`, `sql` or `system` |
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
  readable by it, and by no one else.
- Running as root (for `auth = system` with local accounts, or for
  `run_as`) means app_dmodem's Asterisk runs as root.
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

`make test` checks the password schemes, the file and SQLite backends,
the menu's choices and the telnet client, and runs modemmenu on a pty as
app_dmodem does: a wrong and a right login, the message of the day,
choices by key, name and alias, an unknown choice and `?`, an rlogin BBS
and back to the menu, a cooked shell, and an exec selection's
environment and session file.  The system backend's uid, root and group
rules are tested with PAM replaced.

Two more tests run when their servers are given:

- `MODEMMENU_TEST_PAM=user:password make test`, as root with
  `/etc/pam.d/modemmenu`: PAM against a real account.
- `MODEMMENU_TEST_LDAP=ldap://host:port make test`: the LDAP backend,
  against a server with `uid=lduser,ou=people,dc=example,dc=com`
  (password `ldpass`).
