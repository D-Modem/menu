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
[D-Modem-RE](https://github.com/Neo-Desktop/D-Modem-RE) (`asterisk/`),
hands it the caller's line in one of two ways: it starts modemmenu for
each call (`mode = exec`), or connects each call to one `modemmenu
-serve` on a unix socket (`mode = unix`, below).

### A program for each call

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

### One server on a unix socket

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
[modemmenu.service.sample](modemmenu.service.sample) runs it under
systemd, which also makes `/run/modemmenu`.  Each connection is logged
with its peer's pid and uid.

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
time_limit = 60m        ; a call
daily_limit = 2h        ; a user's calls in a day

[groups]
sysops = alice

[guest]
enabled = yes           ; "guest" at the login prompt, no password

[file]
path = /etc/asterisk/modemmenu.users

[system]
pam_service = modemmenu
groups = dialup

[menu]
hangup = G goodbye bye logoff
prompt = "\r\nSelection ({time_left} min left): "

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
min_baud = 9600
mode = exec
command = exec /usr/sbin/pppd nodetach noauth local lock passive 192.168.77.1:192.168.77.2 ipparam {user}

[item shell]
key = S
label = Shell
groups = sysops
mode = exec
run_as = {user}
tty = sane
pass_password = no
command = exec "$SHELL" -l

[item passwd]
key = PW
label = Change your password
mode = passwd
```

Comments start with `;` or `#`, on a line of their own or after a value
(following a space or tab, outside `"quotes"`); `\;` and `\#` are the
characters themselves.  `modemmenu -check -config FILE` checks a
configuration.

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
   (the distribution's `common-auth`, `common-account` and, for password
   changes, `common-password`).
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

### Changing passwords

A `mode = passwd` item lets callers change their own password; it needs
no command and returns to the menu (`after = menu`) by default.  The
caller gives their current password, then the new one twice (at least
the item's `min_password`, 8 characters, and not the current one; three
tries).  The current password is checked against the backends again,
which also finds the one that holds the account (a login by caller ID or
by an upstream menu does not say), and the password changes there:

| backend | how |
|---|---|
| file | the user's line rewritten in place under the file's lock (a note after the hash is kept); bcrypt, or text with `hash = plain` |
| sql | `[sql] update_query` with `update_args` (`user`, `password`, `old_password`, in the order of its placeholders); `update_hash = bcrypt` stores a bcrypt hash, `none` the password as typed, for a query that hashes it.  Without `update_query`, SQL passwords cannot be changed here |
| ldap | bound as the user, so the server's own password policy and hashing apply: the Password Modify extended operation (RFC 3062; OpenLDAP, 389 DS and most), or with `[ldap] password_change = ad` Active Directory's way, `unicodePwd`'s old value deleted and the new one added (AD takes it only over `ldaps://` or `starttls`) |
| system | PAM's password stack of `pam_service`; what it says (a quality module's reasons) goes to the caller |

A refused password (the server's or PAM's policy) leaves the old one;
the selections after a change get the new one in `MODEM_PASSWORD`.
Guests have no password to change.  The log says who changed their
password where, never the password.

Run as root, `pam_unix` applies no quality checks (its `minlen` and
the like are not for root): for a policy, put `pam_pwquality` before it
with `enforce_for_root`, as
[modemmenu.pam.sample](modemmenu.pam.sample) shows.

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

### Who sees what

An item's rules decide who sees it; a caller who may not use an item
neither sees it in the menu nor can choose it (it is an unknown
selection to them).  All the rules that are set must pass:

| item option | |
|---|---|
| `users = alice bob` | these users ... |
| `groups = sysops` | ... or the members of these groups: `[groups]` (`sysops = alice bob`), the system's groups for a login by `auth = system`, or the directory's for a login by LDAP (with `[ldap] group_base`, below) |
| `min_baud = 9600` | the connect rate towards the caller at least this; an unknown rate does not pass |
| `callerids = 555*` | the caller ID matching one of these patterns |
| `guest = yes` | guests see it too (they see nothing else) |

An item without `users` or `groups` is for every logged-in caller.

LDAP groups count once `[ldap] group_base` is set: a group is an entry
under it that `group_filter` finds.  The default filter,
`(&(cn={group})(|(member={dn})(uniqueMember={dn})(memberUid={user})))`,
takes `groupOfNames`, `groupOfUniqueNames` and `posixGroup` entries;
`{group}` is the item's group, `{dn}` the caller's entry, `{user}` their
name.  The search runs as `search_bind_dn` if there is one, else as the
caller.  Each group's answer is kept for the rest of the call.

### Guests and new accounts

With `[guest] enabled = yes`, its `name` (`guest`) at the login prompt
logs in without a password.  A guest sees only the items with `guest =
yes`, for `[guest] time_limit` (15 minutes) at most; `MODEM_AUTH` is
`guest`.  The guest name wins over an account of the same name.

With `[signup] enabled = yes`, its `name` (`new`) at the login prompt
starts a new account: a user name (one that `user_pattern` takes and no
users-file or system account has), and a password of `min_password`
characters, twice.  The account goes at the end of the users file
(`[signup] file`, the `[file]` backend's by default; `auth` must have
`file`) as `user:bcrypt:signup DATE CALLERID`.  With `pending = yes` (the
default) it is locked, `user:!bcrypt`, and the caller is told it waits
for the sysop, who removes the `!` to unlock it; with `pending = no` the
caller is logged in at once.

### Caller ID

`[callerid]` decides which calls the line takes, before the banner:
`allow` (only these numbers), `deny` (not these) and `anonymous = no`
(not calls without a caller ID).  Numbers are patterns, `*` and `?` as
in file names.  A refused caller hears `deny_message` and the call ends.

`[number PATTERN]` sections, the first that matches, treat one number
or a few specially:

```
[number 5551234]
user = alice    ; logged in as alice without a password
item = ppp      ; and PPP starts at once

[number 1900*]
deny = yes
```

`user` trusts the caller ID: only use it where the network vouches for
the number (your own trunk, not a SIP peer anyone can reach).

### Time limits

`[general] time_limit` limits each call, from the login; `daily_limit`
a user's calls in a day, kept in `state_dir/usage/USER` (made by the
systemd unit's `StateDirectory`).  Guests have `[guest] time_limit`
only.  A caller whose day is used up is told so after the login; the
menu ends when the time is up, and a running selection is hung up (a
program gets `SIGHUP`, then `SIGKILL` 10 seconds later; a server's
connection is closed).  `{time_left}` (whole minutes, empty without a
limit) shows what is left, e.g. in `[menu] prompt`, and the selection
gets it as `MODEM_TIME_LEFT`.

Before the limit, `time_warning_message` is written on the line in the
middle of what a running selection shows, so that a caller in a BBS or
a shell has time to finish: once for each of `time_warning`'s times
(`5m 1m`: at 5 minutes and at 1 minute left; one time, 2 minutes, by
default; `0` for none).  A selection started with less time left gets
only the warnings still ahead.  In the message, `{time_left}` is the
minutes left and `{seconds_left}` the seconds; `\a` beeps the caller's
terminal.  An item with
`time_warning = no` gets none: PPP should not (text between its frames),
and the sample's PPP item says so.

### Drop files for doors

BBS doors (games and utilities written for BBSs) learn who called, on
what line and for how long from a drop file.  An exec item with
`dropfile = door32.sys door.sys dorinfo1.def` writes those files before
its program starts and removes them after, laid out as Synchronet,
Mystic and MBSE write them:

| `dropfile =` | file | for | what |
|---|---|---|---|
| `door32.sys` | `DOOR32.SYS` | 32-bit doors on stdin/stdout or a socket (Mystic's Door32) | 11 lines: comm type and handle, rate, BBS name, user number, name, alias, security level, **minutes left**, emulation, node |
| `door.sys` | `DOOR.SYS` | DOS doors (GAP, Wildcat, PCBoard...) | 52 lines: port, rate, node, name, phone (the caller ID), security level, **seconds and minutes left**, graphics mode, user number, sysop... (`door_sys_lines = 31`: the old 31, the rest blank) |
| `dorinfo1.def` | `DORINFO1.DEF` | RBBS, QuickBBS, RemoteAccess doors | 13 lines, upper case: BBS, sysop, port, `38400 BAUD,N,8,1`, caller's first and last name, graphics, security level, **minutes left**, FOSSIL |
| `dorinfo#.def` | `DORINFO1.DEF` to `DORINFOF.DEF` | the same, a file per node | the node as one hex digit, so 15 nodes at most |

```
[dropfile]
bbs_name = My BBS
sysop = Alice
security = 10

[item lord]
key = L
label = Legend of the Red Dragon
dropfile = door.sys dorinfo1.def
dropfile_dir = /srv/doors/lord/node{node}
security = 20
tty = sane
command = cd /srv/doors/lord && exec dosemu -dumb -E "lord.bat $MODEM_NODE"
after = menu
```

(The DOSEMU command line is an example: it depends on your DOSEMU's
version and setup.)

- **Node:** each door run gets a node of its own, the first free one of
  `[dropfile] max_nodes`, held (a lock in `state_dir/nodes`) until the
  door ends, so that doors with shared data see callers on different
  nodes even under `modemmenu -serve`.
- **User number:** each user gets a number the first time
  (`state_dir/usernumbers`), the same one ever after, for doors that
  keep per-user records by it.
- **Directory:** `dropfile_dir` (with `{node}`, `{user}`...), or a new
  directory each time. Commands get it as `{dropfile_dir}` and
  `MODEM_DROPFILE_DIR`, and the node as `{node}` and `MODEM_NODE`.
- **Time left:** the session's at the start (a day, without a limit);
  the door's own clock counts down from there.
- **Password:** not in the files. Doors do not need it.
- **Path:** the first file's is `{dropfile}` and `MODEM_DROPFILE`, for
  doors that take it on their command line (Synchronet's `%f`).

The files have DOS line ends, as doors expect, and upper-case names
unless `dropfile_lowercase = yes` (doors on Linux often look for
`door32.sys`).  `[dropfile]` holds what they say of the BBS and the
line; its defaults are what Synchronet writes for a door on stdin and
stdout, which is how modemmenu runs doors:

| `[dropfile]` | default | why |
|---|---|---|
| `comm_type`, `comm_handle` | `0`, `-1` | `DOOR32.SYS`: local, no handle to inherit: the door talks on its stdin and stdout, the caller's line (Synchronet's choice for stdio doors; MBSE, on a modem's tty, writes `1` serial) |
| `rate` | `38400` | the DTE rate, as Synchronet writes: doors built on DMlib and others refuse a rate a FOSSIL does not have, such as a modem's 26400.  `connect` gives the modems' rate |
| `com_port`, `fossil` | `COM1:`, `yes` | a DOS door under DOSEMU reaches the caller through the port DOSEMU maps to the line, with a FOSSIL driver (`COM0:` is a local login) |
| `four_digit_years` | `no` | `DOOR.SYS`'s dates as MM/DD/YYYY, which some newer doors want (MBSE's option) |
| `bbs_name`, `sysop`, `security`, `emulation` | | what the BBS is called, its sysop, the callers' security level (an item's `security =` overrides it), `ansi` or `ascii` |

Synchronet's documents the formats in its wiki
([DOOR32.SYS](https://wiki.synchro.net/ref:door32.sys),
[DOOR.SYS](https://wiki.synchro.net/ref:door.sys),
[DORINFO1.DEF](https://wiki.synchro.net/ref:dorinfo1.def)), and how it
runs doors on Linux ([doors](https://wiki.synchro.net/howto:door:index),
[DOSEMU](https://wiki.synchro.net/howto:dosemu)).

### Last callers

A `mode = callers` item shows the last callers (`count`, 10): the time,
the user and the connect rate of each login, the newest first.  With
such an item, each login is recorded in `state_dir/callers` (the latest
200); the caller ID is not, since every caller sees the list.

### More items

The end of [modemmenu.conf.sample](modemmenu.conf.sample) has items to
copy, each needing its program:

| item | program |
|---|---|
| upload | `rz -b -R -E` (lrzsz): ZMODEM uploads into the caller's `~/uploads`, binary, restricted, renaming rather than overwriting |
| kermit | `kermit -Y -x` (C-Kermit): a Kermit server in the caller's files; their terminal's Kermit GETs and SENDs |
| web | `lynx -anonymous`: the web as text, without shell or saving |
| mail, news | `mutt`, `tin -r` |
| chat | `irssi` on your own IRC server |
| gopher | lynx on Gopherspace, quick even at 2400 bit/s |
| callers | the last callers (above) |

They run as the caller (`run_as = {user}`, so modemmenu as root) with a
cooked terminal (`tty = sane`), and return to the menu.

### What the selection gets

| placeholder | environment | |
|---|---|---|
| `{user}` | `MODEM_USER` | the user name |
| `{password}` | `MODEM_PASSWORD` | the password (not with `pass_password = no`) |
| `{auth}` | `MODEM_AUTH` | `file`, `ldap`, `sql`, `system`, `guest`, `callerid` (`[number]`'s `user`) or `upstream:...` |
| `{dialed}` | `MODEM_DIALED` | the number dialed |
| `{callerid}` | `MODEM_CALLERID` | the caller's number |
| `{baud}` | `MODEM_BAUD` | the connect rate towards the caller, bit/s |
| `{rate}`, `{tx_rate}`, `{rx_rate}` | `MODEM_RATE`, `MODEM_TX_RATE`, `MODEM_RX_RATE` | both rates |
| `{date}`, `{time}` | `MODEM_DATE`, `MODEM_TIME` | the connection's date and time |
| `{datetime}`, `{epoch}` | `MODEM_DATETIME`, `MODEM_EPOCH` | the same, RFC 3339 and Unix time |
| `{channel}`, `{profile}` | `MODEM_CHANNEL`, `MODEM_PROFILE` | app_dmodem's channel and profile |
| `{selection}` | `MODEM_SELECTION` | the item chosen |
| `{time_left}` | `MODEM_TIME_LEFT` | whole minutes left, empty without a limit |
| `{seconds_left}` | `MODEM_SECONDS_LEFT` | the same in seconds |
| `{node}`, `{dropfile_dir}` | `MODEM_NODE`, `MODEM_DROPFILE_DIR` | a door's node and its drop files' directory (`dropfile =` items) |

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
- **unix**: like **tcp**, raw, to a server on the unix socket `path`.
  `tcp`, `telnet` and `rlogin` items take a `path` too, for their
  protocol on a unix socket.
- **header** (`header = yes`, on any of the socket kinds): the session
  first, `MODEM_*` `NAME=value` lines and an empty line (the password
  only with `pass_password`), before `send` or the protocol.  It has
  the time left (`MODEM_TIME_LEFT`, `MODEM_SECONDS_LEFT`): a downstream
  `modemmenu -serve` keeps that as a limit of its own (trusted or not:
  it can only shorten the call), so that a caller does not get a new
  allowance by going from one menu to another.
- **drop files** (`dropfile = door32.sys door.sys`, exec items): what a
  BBS door reads of its caller, below.

`after = menu` returns to the menu when the selection ends, `after =
hangup` (the default) hangs up.

### Handing a caller to another menu

An item can hand the caller to another menu application on a unix
socket.  To another `modemmenu -serve`, with the session header and that
server's `trust_user = yes`, the caller stays logged in: it shows its own
banner, welcome and menu, with the user, the password (with
`pass_password`), `MODEM_AUTH` as `upstream:<backend>`, and the call's
rate, number and time from the header.

```
; the front menu
[item more]
key = M
label = More services
mode = unix
path = /run/modemmenu/more.sock
header = yes
after = menu

; the other menu's modemmenu.conf
[server]
listen = /run/modemmenu/more.sock
trust_user = yes
socket_mode = 0600
```

`trust_user` believes whoever can connect: keep that socket to the front
menu's user (`socket_mode`, `socket_group`).  Without it, the other menu
asks the caller to log in again.

### The same password elsewhere: the auth socket

Callers who log in here reach the BBS already logged in.  The same people
logging in to the BBS another way (telnet, SSH, its web forum) can use
the same password: the BBS asks modemmenu's backends through a unix
socket.

```
[authd]
listen = /run/modemmenu/auth.sock
socket_mode = 0660
socket_group = bbs
; and, closer: only these users' programs (by the peer's uid)
allow_users = bbs
```

`modemmenu -serve` answers it as well as its sessions; `modemmenu -authd`
answers it alone (modemmenu-authd.service.sample).  One request a
connection, a JSON line each way:

```
{"user": "alice", "password": "..."}
{"ok": true, "auth": "ldap"}          or          {"ok": false, "error": "denied"}
```

The backends are asked as at the login prompt, `user_pattern` kept; a
refusal comes after `fail_delay`.  Accounts locked in the users file (a
signup the sysop has not unlocked) are refused here too, and a password
changed from the menu (`mode = passwd`) is the one asked for at once.  The BBS's side: its `[modemmenu]
auth_socket` (docs/wiki/Modem-Callers.md there).

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
- Caller ID can be forged on many networks: `[number] user` logs in
  whoever presents the number.  Use it only for numbers your own network
  vouches for.
- Signup's `pending = yes` (the default) keeps new accounts locked until
  you unlock them; with `pending = no`, anyone who calls gets an account
  and every item without `users` or `groups`.  Give public items
  `guest = yes` and keep the rest to groups.
- A server's socket is only as safe as its permissions: anyone who can
  connect gets a login prompt, and with `trust_user` a login as whoever
  they name.  Keep `socket_mode` and `socket_group` to Asterisk or the
  front menu.
- The auth socket answers whether a password is right: whoever can
  connect can guess passwords with it, each wrong one `fail_delay` late.
  Keep it to the BBS's user (`socket_mode` 0660 or 0600, `socket_group`,
  `allow_users`); a mode open to others is refused, and so is TCP.
- The log (syslog's auth facility by default) gets logins, failures and
  selections, never passwords.  It is never the line: the caller would
  see it.

## Tests

`make test` checks the password schemes, the file and SQLite backends,
the menu's choices and the telnet client, and runs modemmenu on a pty as
app_dmodem does: a wrong and a right login, the message of the day,
choices by key, name and alias, an unknown choice and `?`, an rlogin BBS
and back to the menu, a cooked shell, and an exec selection's
environment and session file.  It also runs `modemmenu -serve` on a unix
socket: a session from an app_dmodem header (on a pty of its own), a
caller hanging up, another modemmenu handing a logged-in caller over by
a `mode = unix` item, and `SIGTERM`.  Drop files are tested line by line in all three formats
(Synchronet's defaults, the other settings, the 31-line `DOOR.SYS`,
names split for `DORINFO1.DEF`) and on a call (the files, their names,
the node, the environment, the clean-up).  Calls test who sees what (users,
groups, `min_baud`, caller ID), a guest, signups (logged in at once, and
pending), refused and known numbers, and a selection hung up at the
day's limit, then the day's next call refused.  Password changes are
tested in the users file (other lines and notes kept) and in SQLite, and
on a call: a wrong current password, the rules for the new one, the
change, the next login with it, and a guest.  The auth socket is
tested: right and wrong passwords, unknown, malformed and locked users,
`allow_users` and its settings.  The time warnings are tested on a call
(two, the longer first, and none with `time_warning = no`), the last callers list
(kept to 200, the newest first, no caller ID) on two calls, and Active
Directory's password encoding.  The system backend's uid, root and
group rules are tested with PAM replaced.

Two more tests run when their servers are given:

- `MODEMMENU_TEST_PAM=user:password make test`, as root with
  `/etc/pam.d/modemmenu`: PAM against a real account, and its password
  changed and back (the service needs a password stack).
- `MODEMMENU_TEST_LDAP=ldap://host:port make test`: the LDAP backend,
  against a server with `uid=lduser,ou=people,dc=example,dc=com`
  (password `ldpass`), and that password changed and back (the server
  must let users write their own `userPassword`); with
  `ou=groups,dc=example,dc=com` holding `cn=sysops` (`groupOfNames`,
  `member` lduser), `cn=dialup` (`posixGroup`, `memberUid` lduser) and
  `cn=other` (without), LDAP groups too.

## License

Copyright (C) 2026 Neo-Desktop.

modemmenu is free software under the GNU Affero General Public License,
version 3 only ([LICENSE](LICENSE), `SPDX-License-Identifier:
AGPL-3.0-only`): use it, study it, change it and share it, commercially
too, as long as what you distribute, and a modified version that people
use remotely (callers included), comes with its complete source under
the same license.

For uses the AGPL does not allow (a product whose source you keep, a
modified service without its source), Neo-Desktop offers commercial
licenses: see [COMMERCIAL-LICENSE.md](COMMERCIAL-LICENSE.md).

Contributions come with a contributor agreement that lets them be part
of both: see [CONTRIBUTING.md](CONTRIBUTING.md).

If you modify modemmenu and offer it to callers, the AGPL's section 13
has you offer them its source: a menu item, the banner or the message of
the day naming where it is will do.
