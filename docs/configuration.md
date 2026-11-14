# Configuration

[modemmenu.conf.sample](../samples/modemmenu.conf.sample) has all the options. The
[configuration reference](reference.md) lists all of them, section by section. This is a
short example:

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

Comments start with `;` or `#`. A comment can be on its own line, or
after a value. After a value, a space or tab must come before the
comment, and the comment must be outside `"quotes"`. `\;` and `\#` are
the characters themselves. `modemmenu -check -config FILE` checks a
configuration. `modemmenu -setup -config FILE` changes it on a full
screen ([the sysop's setup](setup.md)).

## The connect rate


Immediately after the banner, before the login prompt, modemmenu tells
the caller the connect rate of the modems, as BBSs did:

```
Connected at 33600 bit/s.
Connected at 50666 bit/s down, 31200 bit/s up.
```

The second line is for a V.90 call, with a different rate in each
direction ("down" is to the caller). `[general] connect_message`
(`"\r\nConnected at {speed}.\r\n"`) is the text. `{speed}` is the rate
in this format. It also operates in the other texts (the welcome, the
message of the day, the prompt). If the rate is not known (no
`DMODEM_RATE` and no `-rate`), modemmenu does not show the message.
`connect_message = ""` disables the message. `{baud}`, `{tx_rate}` and
`{rx_rate}` are only the numbers. For example, use `connect_message =
"CONNECT {baud}\r\n"` to show the same words as a modem.

## The caller's terminal


Before the banner, modemmenu asks the terminal of the caller what type
of terminal it is, as [bbs-sec](https://github.com/D-Modem/bbs) does. It
asks in this sequence:

1. **ANSI**: modemmenu moves the cursor to the bottom right corner and
   asks for its position (`CSI 6n`). An ANSI terminal answers, and the
   answer is its size. On an ANSI terminal, nothing shows. On other
   terminals, modemmenu erases the question.
2. **VT52**, if the terminal is not ANSI: `ESC Z`. A VT52 answers
   `ESC / K`.
3. **The type of an ANSI terminal**: the questions of DEC (the
   answerback, the primary and secondary device attributes), the version
   of xterm (XTVERSION) and the CTerm attributes of SyncTERM. A DEC VT220
   identifies itself as a VT220. xterm identifies itself as
   `XTerm(390)`. SyncTERM gives its CTerm version.
4. **PETSCII**, if the terminal is not ANSI: a Commodore 64 or 128 asks
   nothing and answers nothing. Thus the login prompt monitors its keys.
   DEL, the cursor keys, the function keys and the shifted letters are
   bytes that only a Commodore sends. If one of them comes, the prompt
   starts again in PETSCII. If the caller types a name all in capitals
   (the unshifted keys of a Commodore, or ASCII in capitals), modemmenu
   shows `PRESS DEL (OR BACKSPACE):`. The key that comes tells which
   terminal it is. After that, the texts of the menu are in PETSCII. The
   cases of the letters are swapped, as in the lower/upper case set of
   the C64. Line ends are RETURN, and modemmenu removes escape sequences.
   modemmenu reads the keys of the caller as ASCII. A selection sends and
   gets its own data without changes.

Some input can come during the questions that is not an answer (for
example, a caller who types immediately). That input goes to the login.
A PPP frame stops the questions and goes to AutoPPP. modemmenu records
the result in the log (`terminal vt (XTerm(390)) 132x50`) and gives it
to the selection:

| placeholder | environment | |
|---|---|---|
| `{terminal}` | `MODEM_TERMINAL` | `vt` (an ANSI terminal that gave its type), `ansi`, `vt52`, `petscii`, `ascii`. Empty if modemmenu did not ask |
| `{term}` | `MODEM_TERM`, and `TERM` of exec items | a terminal type for the terminal: `xterm`, `syncterm`, `vt100`, `vt220`... `vt520`, `ansi`, `vt52`, `dumb` |
| `{term_name}` | `MODEM_TERM_NAME` | the type that the terminal gave: `XTerm(390)`, `VT220`, `SyncTERM (CTerm 1.316)`, or its answerback |
| `{rows}`, `{cols}` | `MODEM_ROWS`, `MODEM_COLS` | the size of the terminal (40 by 25 for a Commodore). modemmenu also sets the size on the line for exec items (`stty size`) |

For example, use `rlogin_terminal = {term}/{baud}` for a BBS that gets
the type from rlogin. Another example is `[dropfile] emulation = auto`
(the default). It gives ASCII to a door for a terminal without ANSI
(VT52, PETSCII, a plain terminal), and ANSI for other terminals. An item
with `header = yes` sends the result to the next program. Thus a
downstream `modemmenu -serve` does not ask again.

```ini
[terminal]
detect = yes        ; ask: ANSI, VT52, what an ANSI terminal is
petscii = auto      ; auto: watch the login for a Commodore; yes: every caller is one; no
timeout = 2s        ; for each question
```

On a terminal that answers, the questions take a short time, because
each question ends with its answer. For each question that the terminal
does not answer, they take `timeout` (2 s for ANSI, 1 s for VT52).

## Message of the day


After the welcome and before the menu, modemmenu shows
`[general] motd`. Then it shows the file `motd_file` (for example,
`/etc/motd`). You can use one of them or the two. modemmenu reads the
file at each login, thus changes show immediately. `{placeholders}`
operate in the two. For example:
`motd = "Hello {user}, you called {dialed}.\r\n"`.

## The menu


The caller types a choice and pushes Enter. The caller can select an
item by its `key`, by its name (`[item ppp]`) or by one of its
`aliases`, in uppercase or lowercase. Thus, with `key = P` and
`aliases = dialup`, `p`, `P`, `ppp` and `dialup` all start PPP. A key
can have more than one character (`key = SH`). The menu shows the keys,
and aligns their labels.

- `?` or `menu` shows the menu again. If the choice is unknown,
  modemmenu tells the caller and asks again.
- `[menu] hangup` lists the words that hang up. By default, these are
  `G goodbye bye logoff`. The menu shows the first word.
- `hotkeys = yes` selects with one key, without Enter. Then each key
  (and the first hangup word) must be one character.
- If two items, or an item and a hangup word, answer to the same word,
  this is a configuration error.
- `[menu] title`, `prompt` and `items` (the sequence) set the other
  parts. Refer to the sample.

## The full-screen menu

If the terminal of the caller is applicable, the caller gets the
full-screen menu. The terminal must be ANSI, or a VT that gave its type,
with a minimum size of 40x14. The full-screen menu has these parts:

- The items, in a window on a blue desktop, with the welcome at the top.
- A bar. The caller moves the bar with the cursor keys (or the first
  letter of an item) and pushes Enter. Alternatively, the key of an item
  in `[ ]` starts the item immediately.
- The motd, in a box over the menu. If the motd has ANSI escapes,
  modemmenu first shows it without changes, then "Press any key".
- The `help` of an item, under the menu.
- A bar at the top with the user, the rate and the time. A bar at the
  bottom with the minutes that remain.

Esc asks for confirmation before it hangs up. F1 shows the keys. Some
programs (`mode = exec`, `passwd`, `callers`) come back to the menu.
After such a program, "Press any key for the menu" keeps the output of
the program on the screen until the caller reads it.

The login stays in lines, as before. PPP dialers (AutoPPP) and AOL
clients read and answer the login prompt. Also, the answers of the
terminal are not known until then. Other terminals (VT52, a Commodore,
a terminal that answers nothing) get the menu in lines.

```ini
[menu]
fullscreen = auto   ; no: lines for every caller

[terminal]
charset = auto      ; utf8, cp437, dec or ascii

[item bbs]
help = Messages, files and doors on the board
```

For the lines of the screen, modemmenu asks an ANSI terminal if it
shows UTF-8. To do this, it writes an `é` and asks for the position of
the cursor. SyncTERM uses code page 437. Without UTF-8, a VT or xterm
gets its line-drawing set, and other terminals get code page 437.
modemmenu sends only the cells that change. Below 9600 bit/s, the
desktop is plain blue, not shaded, because modemmenu can clear it in a
few bytes. At 2400 bit/s, the first screen is about 1 KB, and a movement
of the bar is a few hundred bytes.

## Who sees what


The rules of an item tell which callers see it. If a caller cannot use
an item, the caller does not see it in the menu and cannot select it.
For that caller, it is an unknown selection. All the rules that are set
must pass:

| item option | |
|---|---|
| `users = alice bob` | these users ... |
| `groups = sysops` | ... or the members of these groups: `[groups]` (`sysops = alice bob`), the groups of the system for a login by `auth = system`, or the groups of the directory for a login by LDAP (with `[ldap] group_base`, refer to the text below) |
| `min_baud = 9600` | the connect rate to the caller is a minimum of this value. An unknown rate does not pass |
| `callerids = 555*` | the caller ID agrees with one of these patterns |
| `guest = yes` | guests also see the item (they see no other items) |

An item without `users` or `groups` is for all logged-in callers.

modemmenu uses LDAP groups when `[ldap] group_base` is set. A group is
an entry under `group_base` that `group_filter` finds. The default filter
is `(&(cn={group})(|(member={dn})(uniqueMember={dn})(memberUid={user})))`.
It accepts `groupOfNames`, `groupOfUniqueNames` and `posixGroup` entries.
`{group}` is the group of the item, `{dn}` is the entry of the caller,
and `{user}` is the name of the caller. The search runs as
`search_bind_dn` if it is set. If not, it runs as the caller. modemmenu
keeps the answer for each group until the call ends.

## Guests and new accounts


With `[guest] enabled = yes`, the `name` of the guest (`guest`) at the
login prompt logs in without a password. A guest sees only the items
with `guest = yes`, for a maximum of `[guest] time_limit` (15 minutes).
`MODEM_AUTH` is `guest`. If an account has the same name as the guest,
the guest name has priority.

With `[signup] enabled = yes`, the `name` of the signup (`new`) at the
login prompt starts a new account. The caller gives these items:

- A user name. `user_pattern` must accept it, and no account in the
  users file or the system can have it.
- A password of `min_password` characters, two times.

modemmenu adds the account at the end of the users file as
`user:bcrypt:signup DATE CALLERID`. The users file is `[signup] file`
(by default, the file of the `[file]` backend). `auth` must have `file`.
With `pending = yes` (the default), the account is locked
(`user:!bcrypt`). modemmenu tells the caller that the account waits for
the sysop. To unlock the account, the sysop removes the `!`. With
`pending = no`, the caller is logged in immediately.

## Caller ID


Before the banner, `[callerid]` tells which calls the line accepts:

- `allow`: only these numbers.
- `deny`: not these numbers.
- `anonymous = no`: no calls without a caller ID.

Numbers are patterns, with `*` and `?` as in file names. A refused caller
hears `deny_message`, and the call ends.

`[number PATTERN]` sections apply special rules to one number or to a
small number of numbers. modemmenu uses the first section that agrees
with the number:

```
[number 5551234]
user = alice    ; logged in as alice without a password
item = ppp      ; and PPP starts at once

[number 1900*]
deny = yes
```

`user` trusts the caller ID. Use it only if the network can confirm the
number (your own trunk, not a SIP peer that all persons can get to).

## Time limits


`[general] time_limit` limits each call, from the login.
`daily_limit` limits all the calls of a user in one day. modemmenu keeps
the daily use in `state_dir/usage/USER`. The `StateDirectory` of the
systemd unit makes this directory. Guests have only `[guest] time_limit`.

If a caller has no more time for the day, modemmenu tells the caller
after the login. When the time ends, the menu ends, and modemmenu hangs
up a selection that runs. A program gets `SIGHUP`, then `SIGKILL` 10
seconds later. modemmenu closes the connection to a server.
`{time_left}` shows the time that remains, in whole minutes (empty
without a limit). For example, you can use it in `[menu] prompt`. The
selection gets it as `MODEM_TIME_LEFT`.

Before the limit, modemmenu writes `time_warning_message` on the line,
in the output of the selection that runs. Thus a caller in a BBS or a
shell has time to complete the work. modemmenu writes the message one
time for each of the times in `time_warning`. For example, `5m 1m` gives
a warning when 5 minutes remain and when 1 minute remains. The default
is one warning at 2 minutes. `0` gives no warning. If a selection starts
with less time, it gets only the warnings that remain. In the message,
`{time_left}` is the minutes that remain, and `{seconds_left}` is the
seconds. `\a` makes a beep on the terminal of the caller. An item with
`time_warning = no` gets no warning. PPP must not get warnings (the text
comes between its frames), and the PPP item in the sample has this
setting.

## Drop files for doors


BBS doors are games and utilities for BBSs. A door gets data from a
drop file: the caller, the line and the time that remains. An exec item
with `dropfile = door32.sys door.sys dorinfo1.def` writes those files
before its program starts, and removes them after the program ends. The
layout of the files is the same as Synchronet, Mystic and MBSE write
them:

| `dropfile =` | file | for | what |
|---|---|---|---|
| `door32.sys` | `DOOR32.SYS` | 32-bit doors on stdin/stdout or a socket (Door32 of Mystic) | 11 lines: comm type and handle, rate, BBS name, user number, name, alias, security level, **minutes left**, emulation, node |
| `door.sys` | `DOOR.SYS` | DOS doors (GAP, Wildcat, PCBoard...) | 52 lines: port, rate, node, name, phone (the caller ID), security level, **seconds and minutes left**, graphics mode, user number, sysop... (`door_sys_lines = 31`: the old 31 lines, and the other lines blank) |
| `dorinfo1.def` | `DORINFO1.DEF` | RBBS, QuickBBS, RemoteAccess doors | 13 lines, upper case: BBS, sysop, port, `38400 BAUD,N,8,1`, first and last name of the caller, graphics, security level, **minutes left**, FOSSIL |
| `dorinfo#.def` | `DORINFO1.DEF` to `DORINFOF.DEF` | the same, one file for each node | the node as one hex digit, thus a maximum of 15 nodes |

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

(The DOSEMU command line is an example. It changes with the version
and configuration of your DOSEMU.)

- **Node:** each run of a door gets its own node. This is the first free
  node of `[dropfile] max_nodes`. modemmenu holds the node (a lock in
  `state_dir/nodes`) until the door ends. Thus doors with shared data see
  callers on different nodes, also under `modemmenu -serve`.
- **User number:** each user gets a number the first time
  (`state_dir/usernumbers`). The user keeps the same number after that.
  This is for doors that keep records for each user by this number.
- **Directory:** `dropfile_dir` (with `{node}`, `{user}`...), or a new
  directory each time. Commands get it as `{dropfile_dir}` and
  `MODEM_DROPFILE_DIR`, and the node as `{node}` and `MODEM_NODE`.
- **Time left:** the time that remains in the session at the start (one
  day, without a limit). Then the clock of the door counts down.
- **Password:** not in the files. Doors do not use it.
- **Path:** the path of the first file is `{dropfile}` and
  `MODEM_DROPFILE`. This is for doors that get it on their command line
  (`%f` of Synchronet).

The files have DOS line ends, as doors expect. They have upper-case
names, unless `dropfile_lowercase = yes` (doors on Linux frequently look
for `door32.sys`). `[dropfile]` holds the data about the BBS and the
line that the files contain. Its defaults are the values that Synchronet
writes for a door on stdin and stdout. modemmenu runs doors in this
way:

| `[dropfile]` | default | why |
|---|---|---|
| `comm_type`, `comm_handle` | `0`, `-1` | `DOOR32.SYS`: local, with no handle to inherit. The door communicates on its stdin and stdout, which is the line of the caller. Synchronet uses this value for stdio doors. MBSE, on the tty of a modem, writes `1` (serial) |
| `rate` | `38400` | the DTE rate, as Synchronet writes it. Doors built on DMlib, and other doors, refuse a rate that a FOSSIL does not have, for example the 26400 of a modem. `connect` gives the rate of the modems |
| `com_port`, `fossil` | `COM1:`, `yes` | a DOS door under DOSEMU gets to the caller through the port that DOSEMU maps to the line, with a FOSSIL driver (`COM0:` is a local login) |
| `four_digit_years` | `no` | the dates in `DOOR.SYS` as MM/DD/YYYY, which some newer doors must have (an option of MBSE) |
| `bbs_name`, `sysop`, `security`, `emulation` | | the name of the BBS, its sysop, the security level of the callers (the `security =` of an item overrides it), and `auto` (from the detected terminal, the default), `ansi` or `ascii` |

The wiki of Synchronet gives the formats
([DOOR32.SYS](https://wiki.synchro.net/ref:door32.sys),
[DOOR.SYS](https://wiki.synchro.net/ref:door.sys),
[DORINFO1.DEF](https://wiki.synchro.net/ref:dorinfo1.def)). It also tells
how Synchronet runs doors on Linux
([doors](https://wiki.synchro.net/howto:door:index),
[DOSEMU](https://wiki.synchro.net/howto:dosemu)).

## Last callers


A `mode = callers` item shows the last callers (`count`, 10). It shows
the time, the user and the connect rate of each login, with the newest
first. If there is such an item, modemmenu records each login in
`state_dir/callers` (the latest 200). It does not record the caller ID,
because all callers see the list.

## More items


The end of [modemmenu.conf.sample](../samples/modemmenu.conf.sample) has items that
you can copy. Each item must have its program:

| item | program |
|---|---|
| upload | `rz -b -R -E` (lrzsz): ZMODEM uploads into `~/uploads` of the caller, binary and restricted. It renames files and does not overwrite them |
| kermit | `kermit -Y -x` (C-Kermit): a Kermit server in the files of the caller. The Kermit of their terminal does GETs and SENDs |
| web | `lynx -anonymous`: the web as text, without a shell and without save operations |
| mail, news | `mutt`, `tin -r` |
| chat | `irssi` on your own IRC server |
| gopher | lynx on Gopherspace, fast also at 2400 bit/s |
| callers | the last callers (refer to the section above) |

They run as the caller (`run_as = {user}`, thus modemmenu runs as root)
with a cooked terminal (`tty = sane`). After they end, the caller goes
back to the menu.

## What the selection gets


| placeholder | environment | |
|---|---|---|
| `{user}` | `MODEM_USER` | the user name |
| `{password}` | `MODEM_PASSWORD` | the password (not with `pass_password = no`) |
| `{auth}` | `MODEM_AUTH` | `file`, `ldap`, `sql`, `system`, `guest`, `callerid` (the `user` of `[number]`) or `upstream:...` |
| `{dialed}` | `MODEM_DIALED` | the number dialed |
| `{callerid}` | `MODEM_CALLERID` | the number of the caller |
| `{baud}` | `MODEM_BAUD` | the connect rate to the caller, bit/s |
| `{speed}` | `MODEM_SPEED` | the connect rate in a format for persons to read: `33600 bit/s`, or `50666 bit/s down, 31200 bit/s up`. Empty if not known |
| `{rate}`, `{tx_rate}`, `{rx_rate}` | `MODEM_RATE`, `MODEM_TX_RATE`, `MODEM_RX_RATE` | both rates |
| `{date}`, `{time}` | `MODEM_DATE`, `MODEM_TIME` | the date and time of the connection |
| `{datetime}`, `{epoch}` | `MODEM_DATETIME`, `MODEM_EPOCH` | the same, RFC 3339 and Unix time |
| `{channel}`, `{profile}` | `MODEM_CHANNEL`, `MODEM_PROFILE` | the channel and profile of app_dmodem |
| `{peer}` | `MODEM_PEER` | the SIP address of the call (`DMODEM_PEER` of app_dmodem). Empty if not known. The BBS uses it to find the location of a caller without a caller ID |
| `{selection}` | `MODEM_SELECTION` | the selected item |
| `{time_left}` | `MODEM_TIME_LEFT` | the whole minutes that remain. Empty without a limit |
| `{seconds_left}` | `MODEM_SECONDS_LEFT` | the same in seconds |
| `{terminal}`, `{term}`, `{term_name}` | `MODEM_TERMINAL`, `MODEM_TERM` (and `TERM`), `MODEM_TERM_NAME` | the terminal of the caller, as detected (refer to the section above) |
| `{rows}`, `{cols}` | `MODEM_ROWS`, `MODEM_COLS` | the size of the terminal, if the terminal gave it |
| `{node}`, `{dropfile_dir}` | `MODEM_NODE`, `MODEM_DROPFILE_DIR` | the node of a door and the directory of its drop files (`dropfile =` items) |

Each type of selection gets these values in a different way:

- **exec** (`command`, through `/bin/sh -c`): the environment and the
  placeholders in the command. Each placeholder is already a
  shell-quoted word, thus write `{user}` and not `"{user}"`. With
  `session_file = yes`, the program also gets all the values as JSON in
  a 0600 file. `MODEM_SESSION_FILE` gives the name of the file.
  modemmenu removes the file when the program ends. `run_as` runs the
  program as a system user (modemmenu must be root). `tty = sane` gives
  it a cooked terminal.
- **rlogin**: the RFC 1282 handshake. By default, it sends the password
  as the client user, the user name as the server user and `ansi/{baud}`
  as the terminal. Synchronet, Mystic and other BBSs use this to log in a
  caller without a second login prompt.
- **tcp** and **telnet**: modemmenu sends `send` (for example,
  `send = "{user}\r{password}\r"`) when the connection is made. telnet
  negotiates binary, echo and the terminal type.
- **unix**: the same as **tcp**, raw, to a server on the unix socket
  `path`. `tcp`, `telnet` and `rlogin` items also accept a `path`, for
  their protocol on a unix socket.
- **header** (`header = yes`, on all types of socket): first the
  session, as `MODEM_*` `NAME=value` lines and an empty line. The header
  contains the password only with `pass_password`. The header comes
  before `send` or the protocol. It contains the time that remains
  (`MODEM_TIME_LEFT`, `MODEM_SECONDS_LEFT`). A downstream
  `modemmenu -serve` keeps that time as its own limit, also if it does
  not trust the header, because the limit can only make the call
  shorter. Thus a caller cannot get more time when the caller goes from
  one menu to a different menu.
- **drop files** (`dropfile = door32.sys door.sys`, exec items): the data
  about the caller that a BBS door reads. Refer to
  [Drop files for doors](#drop-files-for-doors).

`after = menu` goes back to the menu when the selection ends.
`after = hangup` (the default) hangs up.

## Handing a caller to another menu


An item can transfer the caller to a different menu application on a
unix socket. The caller can stay logged in if these conditions are
true:

- The other application is a `modemmenu -serve`.
- The item sends the session header.
- That server has `trust_user = yes`.

Then the other server shows its own banner, welcome and menu. It uses
the user, the password (with `pass_password`), `MODEM_AUTH` as
`upstream:<backend>`, and the rate, number and time of the call from the
header.

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

`trust_user` trusts each user that can connect. Thus, limit that socket
to the user of the front menu (`socket_mode`, `socket_group`). Without
`trust_user`, the other menu asks the caller to log in again.
