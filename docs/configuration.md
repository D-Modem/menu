# Configuration

[modemmenu.conf.sample](../modemmenu.conf.sample) has every option.  In
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

## The connect rate


Right after the banner, before the login prompt, modemmenu tells the
caller how fast the modems connected, as BBSs did:

```
Connected at 33600 bit/s.
Connected at 50666 bit/s down, 31200 bit/s up.
```

The second is a V.90 call, its two directions apart (down: to the
caller).  `[general] connect_message` (`"\r\nConnected at {speed}.\r\n"`)
is the text; `{speed}` is the rate put so, and works in the other texts
too (the welcome, the message of the day, the prompt).  The message is
left out when the rate is not known (no `DMODEM_RATE` and no `-rate`),
and `connect_message = ""` turns it off.  `{baud}`, `{tx_rate}` and
`{rx_rate}` are the bare numbers, e.g. `connect_message =
"CONNECT {baud}\r\n"` for the modem's own words.

## The caller's terminal


Before the banner, modemmenu asks the caller's terminal what it is, as
[bbs-sec](https://github.com/D-Modem/bbs) does, and in this
order:

1. **ANSI**: the cursor sent as far down and right as it goes, and its
   position asked for (`CSI 6n`).  An ANSI terminal answers, and the
   answer is its size.  Nothing shows on one that does; the question
   is wiped on one that does not.
2. **VT52**, if not ANSI: `ESC Z`, which a VT52 answers `ESC / K`.
3. **What an ANSI terminal is**: DEC's questions (the answerback, the
   primary and secondary device attributes), xterm's version
   (XTVERSION) and SyncTERM's CTerm attributes.  A DEC VT220 says it is
   one; xterm says `XTerm(390)`; SyncTERM its CTerm version.
4. **PETSCII**, if not ANSI: a Commodore 64 or 128 asks nothing and
   answers nothing, so the login prompt watches its keys.  DEL, the
   cursor keys, the function keys and the shifted letters are bytes only
   a Commodore sends: one of them, and the prompt starts again in
   PETSCII.  A name typed all in capitals (a Commodore's unshifted keys,
   or ASCII in capitals) gets `PRESS DEL (OR BACKSPACE):`, and the key
   that comes tells which.  From then on the menu's texts are PETSCII
   (the letters' cases swapped as the C64's lower/upper case set has
   them, line ends as RETURN, escape sequences left out) and its keys
   read as ASCII; what a selection sends and gets is its own.

What comes while the terminal is asked that is no answer (a caller who
types at once) is the login's; a PPP frame ends the questions and goes
to AutoPPP.  The result is in the log (`terminal vt (XTerm(390)) 132x50`)
and is the selection's:

| placeholder | environment | |
|---|---|---|
| `{terminal}` | `MODEM_TERMINAL` | `vt` (an ANSI terminal that said what it is), `ansi`, `vt52`, `petscii`, `ascii`; empty when not asked |
| `{term}` | `MODEM_TERM`, and `TERM` of exec items | a terminal type for it: `xterm`, `syncterm`, `vt100`, `vt220`... `vt520`, `ansi`, `vt52`, `dumb` |
| `{term_name}` | `MODEM_TERM_NAME` | what it said it is: `XTerm(390)`, `VT220`, `SyncTERM (CTerm 1.316)`, or its answerback |
| `{rows}`, `{cols}` | `MODEM_ROWS`, `MODEM_COLS` | its size (40 by 25 for a Commodore), also set on the line for exec items (`stty size`) |

E.g. `rlogin_terminal = {term}/{baud}` for a BBS that takes the type
from rlogin, or `[dropfile] emulation = auto` (the default), which
gives a door ASCII for a terminal without ANSI (VT52, PETSCII, a plain
one) and ANSI otherwise.  An item with `header = yes` passes it on: a
downstream `modemmenu -serve` does not ask again.

```ini
[terminal]
detect = yes        ; ask: ANSI, VT52, what an ANSI terminal is
petscii = auto      ; auto: watch the login for a Commodore; yes: every caller is one; no
timeout = 2s        ; for each question
```

The questions cost a moment on a terminal that answers (each ends with
its answer) and `timeout` for each one it does not (2 s for ANSI, 1 s
for VT52).

## Message of the day


After the welcome and before the menu, modemmenu shows `[general] motd`
and then the file `motd_file` (e.g. `/etc/motd`), either or both.  The
file is read at each login, so edits show at once; `{placeholders}` work
in both, e.g. `motd = "Hello {user}, you called {dialed}.\r\n"`.

## The menu


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

## Who sees what


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

## Guests and new accounts


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

## Caller ID


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

## Time limits


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

## Drop files for doors


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
| `bbs_name`, `sysop`, `security`, `emulation` | | what the BBS is called, its sysop, the callers' security level (an item's `security =` overrides it), `auto` (by the detected terminal; the default), `ansi` or `ascii` |

Synchronet's documents the formats in its wiki
([DOOR32.SYS](https://wiki.synchro.net/ref:door32.sys),
[DOOR.SYS](https://wiki.synchro.net/ref:door.sys),
[DORINFO1.DEF](https://wiki.synchro.net/ref:dorinfo1.def)), and how it
runs doors on Linux ([doors](https://wiki.synchro.net/howto:door:index),
[DOSEMU](https://wiki.synchro.net/howto:dosemu)).

## Last callers


A `mode = callers` item shows the last callers (`count`, 10): the time,
the user and the connect rate of each login, the newest first.  With
such an item, each login is recorded in `state_dir/callers` (the latest
200); the caller ID is not, since every caller sees the list.

## More items


The end of [modemmenu.conf.sample](../modemmenu.conf.sample) has items to
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

## What the selection gets


| placeholder | environment | |
|---|---|---|
| `{user}` | `MODEM_USER` | the user name |
| `{password}` | `MODEM_PASSWORD` | the password (not with `pass_password = no`) |
| `{auth}` | `MODEM_AUTH` | `file`, `ldap`, `sql`, `system`, `guest`, `callerid` (`[number]`'s `user`) or `upstream:...` |
| `{dialed}` | `MODEM_DIALED` | the number dialed |
| `{callerid}` | `MODEM_CALLERID` | the caller's number |
| `{baud}` | `MODEM_BAUD` | the connect rate towards the caller, bit/s |
| `{speed}` | `MODEM_SPEED` | the connect rate for reading: `33600 bit/s`, or `50666 bit/s down, 31200 bit/s up`; empty if not known |
| `{rate}`, `{tx_rate}`, `{rx_rate}` | `MODEM_RATE`, `MODEM_TX_RATE`, `MODEM_RX_RATE` | both rates |
| `{date}`, `{time}` | `MODEM_DATE`, `MODEM_TIME` | the connection's date and time |
| `{datetime}`, `{epoch}` | `MODEM_DATETIME`, `MODEM_EPOCH` | the same, RFC 3339 and Unix time |
| `{channel}`, `{profile}` | `MODEM_CHANNEL`, `MODEM_PROFILE` | app_dmodem's channel and profile |
| `{selection}` | `MODEM_SELECTION` | the item chosen |
| `{time_left}` | `MODEM_TIME_LEFT` | whole minutes left, empty without a limit |
| `{seconds_left}` | `MODEM_SECONDS_LEFT` | the same in seconds |
| `{terminal}`, `{term}`, `{term_name}` | `MODEM_TERMINAL`, `MODEM_TERM` (and `TERM`), `MODEM_TERM_NAME` | the caller's terminal, as detected (above) |
| `{rows}`, `{cols}` | `MODEM_ROWS`, `MODEM_COLS` | its size, if it said |
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

## Handing a caller to another menu


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
