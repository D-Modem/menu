<!-- Made by go run ./tools/confref from samples/modemmenu.conf.sample: edit the sample, not this page. -->

# Configuration reference

Every section and key of modemmenu.conf, from the comments in
[modemmenu.conf.sample](https://github.com/D-Modem/menu/blob/main/samples/modemmenu.conf.sample).
**Sample** is the value the sample sets (or shows, commented out);
[Configuration](configuration.md) explains the file and its sections in
full.

| section | what it is |
|---|---|
| [`[general]`](#general) | The login: the log, the backends, what the caller sees, the time limits. |
| [`[terminal]`](#terminal) | the caller's terminal, asked before the banner: ANSI (and its size), VT52, what an ANSI terminal is (xterm, a VT220, SyncTERM...), and a Commodore by its keys at the login. |
| [`[groups]`](#groups) | groups of users for the items' groups = (members of system groups count too, for logins by auth = system) |
| [`[guest]`](#guest) | a guest account: its name at the login prompt, no password; it sees only the items with guest = yes, for time_limit at most. |
| [`[signup]`](#signup) | new accounts: the name at the login prompt starts a signup, which adds user:bcrypt to the users file (auth must have file). |
| [`[callerid]`](#callerid) | caller ID: numbers or patterns (* and ? as in file names; anonymous: no caller ID). |
| [`[number NAME]`](#number-name) | a number of its own: deny = yes refuses it; user = logs it in as that user without a password (caller ID can be forged: only where the network vouches for it); item = starts that item at once after the login. |
| [`[file]`](#file) | users in a text file: user:hash, # comments. |
| [`[ldap]`](#ldap) | LDAP: a bind as the user. |
| [`[system]`](#system) | system: the accounts of this machine, by PAM: the auth and account stacks of pam_service (modemmenu.pam.sample as /etc/pam.d/modemmenu), so local accounts, sssd, Kerberos, whatever PAM is set up for. |
| [`[sql]`](#sql) | SQL: query gets args (user, password, in that order of placeholders) and returns one column: the stored password, checked as hash says (auto: by its prefix as in [file]; or bcrypt, sha256, sha512, plain). |
| [`[server]`](#server) | modemmenu -serve: one modemmenu for all calls, on a unix socket (or tcp:host:port). |
| [`[authd]`](#authd) | The auth socket: the BBS's other logins (telnet, SSH, the web) ask the backends above for a user's password through it, so that it is the same one. |
| [`[ppp_pool]`](#ppp_pool) | The PPP callers' addresses: a command that names {ppp_remote} (or {ppp_local}, {ppp_dns}, {ppp_dns2}) takes the first free address of range for the call, so that several lines run PPP at once, and gives it back when pppd ends. |
| [`[autoppp]`](#autoppp) | AutoPPP: a dialer that starts PPP at once (Windows' Dial-Up Networking, most others) sends a PPP frame where the login prompt wants a name; then command runs on the line instead (pppd), and its modemmenu.so plugin (make builds it with ppp-dev's headers; make install puts it in pppd's plugin directory) asks this call's auth socket for the PAP login: the backends check it as at the prompt, then item's access rules, then the time limits, whose time left becomes pppd's maxconnect. |
| [`[aol]`](#aol) | AOL: the classic AOL client (2.5 to 4.0) dials a network and its CCL script answers the network's prompts before it speaks P3. |
| [`[dropfile]`](#dropfile) | drop files for doors (an exec item's dropfile =): what they say of the BBS, and how the door reaches the caller. |
| [`[menu]`](#menu) | The menu: its title, prompt, items and the words that hang up. |
| [`[item NAME]`](#item-name) | A menu item: a program on the line, a server to connect the caller to, or a built-in (see the item keys below). |

## `[general]`

The login: the log, the backends, what the caller sees, the time limits.

| key | sample | what it does |
|---|---|---|
| `log` | `syslog` | syslog (auth facility), a file, or none. Never the line: stderr is the caller's screen. |
| `auth` | `file` | the backends, asked in this order: file, ldap, sql, system (PAM). A backend that does not know the user passes to the next; a wrong password stops there. |
| `banner` | `\r\nWelcome to D-Modem.\r\n` | the backends, asked in this order: file, ldap, sql, system (PAM). A backend that does not know the user passes to the next; a wrong password stops there. |
| `banner_file` | `/etc/asterisk/modemmenu.banner` | the backends, asked in this order: file, ldap, sql, system (PAM). A backend that does not know the user passes to the next; a wrong password stops there. |
| `login_prompt` | `\r\nlogin: ` | the backends, asked in this order: file, ldap, sql, system (PAM). A backend that does not know the user passes to the next; a wrong password stops there. |
| `password_prompt` | `Password: ` | the backends, asked in this order: file, ldap, sql, system (PAM). A backend that does not know the user passes to the next; a wrong password stops there. |
| `connect_message` | `\r\nConnected at {speed}.\r\n` | after the banner, before the login prompt: the connect rate ({speed}: "33600 bit/s", or "50666 bit/s down, 31200 bit/s up"); left out when it is not known; "" for none. |
| `welcome` | `\r\nHello {user}, on {date} at {time}.\r\n` | after the banner, before the login prompt: the connect rate ({speed}: "33600 bit/s", or "50666 bit/s down, 31200 bit/s up"); left out when it is not known; "" for none. |
| `motd` | `\r\nThe line is down for maintenance on Sunday.\r\n` | the message of the day, after the welcome and before the menu: motd, then the file (read at each login), {placeholders} in both. |
| `motd_file` | `/etc/motd` | the message of the day, after the welcome and before the menu: motd, then the file (read at each login), {placeholders} in both. |
| `goodbye` | `\r\nGoodbye.\r\n` | the message of the day, after the welcome and before the menu: motd, then the file (read at each login), {placeholders} in both. |
| `password_echo` | `*` | the echo of each password character, or none. |
| `attempts` | `3` | logins before the call is hung up, seconds after a wrong one. |
| `fail_delay` | `3` | logins before the call is hung up, seconds after a wrong one. |
| `login_timeout` | `90` | seconds to log in, to choose in the menu. |
| `idle_timeout` | `300` | seconds to log in, to choose in the menu. |
| `user_pattern` | `^[A-Za-z0-9][A-Za-z0-9._@-]{0,63}$` | user names taken (others are refused without asking a backend) |
| `max_input` | `64` | the longest line the caller may type at a prompt: a user name or a menu choice (a password twice as long) |
| `time_limit` | `0` | time limits (0: none), from the login: a call, and a user's calls in a day (kept in state_dir/usage). A selection is hung up at the limit. {time_left} (whole minutes) shows what is left, e.g. in [menu] prompt. |
| `daily_limit` | `0` | time limits (0: none), from the login: a call, and a user's calls in a day (kept in state_dir/usage). A selection is hung up at the limit. {time_left} (whole minutes) shows what is left, e.g. in [menu] prompt. |
| `state_dir` | `/var/lib/modemmenu` | time limits (0: none), from the login: a call, and a user's calls in a day (kept in state_dir/usage). A selection is hung up at the limit. {time_left} (whole minutes) shows what is left, e.g. in [menu] prompt. |
| `time_warning` | `5m 1m` | these long before the limit, the message is written on the line in the middle of a running selection (not one with time_warning = no), once for each: 5m 1m warns at 5 minutes and at 1 minute left; 0: none. {time_left} is in minutes, {seconds_left} in seconds; \a beeps. |
| `time_warning_message` | `\r\n\r\n*** {time_left} minute(s) left in this call ***\r\n` | these long before the limit, the message is written on the line in the middle of a running selection (not one with time_warning = no), once for each: 5m 1m warns at 5 minutes and at 1 minute left; 0: none. {time_left} is in minutes, {seconds_left} in seconds; \a beeps. |

## `[terminal]`

the caller's terminal, asked before the banner: ANSI (and its size), VT52, what an ANSI terminal is (xterm, a VT220, SyncTERM...), and a Commodore by its keys at the login.  {terminal}, {term}, {term_name}, {rows} and {cols} in the selections (TERM for exec items)

| key | sample | what it does |
|---|---|---|
| `detect` | `yes` | See the section above. |
| `petscii` | `auto` | auto: watch the login for a Commodore's keys; yes: every caller is one. |
| `timeout` | `2s` | for each question a terminal may not answer. |

## `[groups]`

groups of users for the items' groups = (members of system groups count too, for logins by auth = system)

Each key is a group: its name, then its members (`sysops = alice bob`).

## `[guest]`

a guest account: its name at the login prompt, no password; it sees only the items with guest = yes, for time_limit at most.

| key | sample | what it does |
|---|---|---|
| `enabled` | `no` | See the section above. |
| `name` | `guest` | See the section above. |
| `time_limit` | `15m` | See the section above. |

## `[signup]`

new accounts: the name at the login prompt starts a signup, which adds user:bcrypt to the users file (auth must have file).  pending = yes locks the account (user:!hash) until the sysop removes the !.

| key | sample | what it does |
|---|---|---|
| `enabled` | `no` | See the section above. |
| `name` | `new` | See the section above. |
| `pending` | `yes` | See the section above. |
| `min_password` | `8` | See the section above. |
| `file` | `/etc/asterisk/modemmenu.users` | See the section above. |
| `message` | `\r\nNew account.  Choose a user name and a password.\r\n` | See the section above. |

## `[callerid]`

caller ID: numbers or patterns (* and ? as in file names; anonymous: no caller ID).  With allow, only those numbers; deny refuses some.

| key | sample | what it does |
|---|---|---|
| `allow` | `555* 1800*` | See the section above. |
| `deny` | `1900* 5550000` | See the section above. |
| `anonymous` | `yes` | See the section above. |
| `deny_message` | `\r\nThis line does not take your call.\r\n` | See the section above. |

## `[number NAME]`

a number of its own: deny = yes refuses it; user = logs it in as that user without a password (caller ID can be forged: only where the network vouches for it); item = starts that item at once after the login.

There may be any number of these, each with its own NAME.

| key | sample | what it does |
|---|---|---|
| `user` | `alice` | See the section above. |
| `item` | `ppp` | See the section above. |

## `[file]`

users in a text file: user:hash, # comments.  hash: bcrypt ($2b$..., modemmenu -hash makes one), {SHA256}hex, {SHA512}hex, {SSHA256}base64, {SSHA512}base64 or {PLAIN}text (hash = plain: the rest of the line, as is)

| key | sample | what it does |
|---|---|---|
| `path` | `/etc/asterisk/modemmenu.users` | See the section above. |
| `hash` | `auto` | See the section above. |

## `[ldap]`

LDAP: a bind as the user.  Either bind_dn with {user} in it, or a search for the user's entry (search_base, search_filter) as search_bind_dn, then a bind as the entry found.  Restrict who may log in in the filter, e.g. (&(uid={user})(memberOf=cn=dialup,ou=groups,dc=example,dc=com))

| key | sample | what it does |
|---|---|---|
| `url` | `ldaps://ldap.example.com` | See the section above. Also: `ldap://ldap.example.com:389`. |
| `starttls` | `yes` | See the section above. |
| `ca_file` | `/etc/ssl/certs/example-ca.pem` | See the section above. |
| `insecure_skip_verify` | `no` | See the section above. |
| `timeout` | `10` | See the section above. |
| `bind_dn` | `uid={user},ou=people,dc=example,dc=com` | See the section above. |
| `search_base` | `ou=people,dc=example,dc=com` | See the section above. |
| `search_filter` | `(&(objectClass=posixAccount)(uid={user}))` | See the section above. |
| `search_bind_dn` | `cn=modem,ou=services,dc=example,dc=com` | See the section above. |
| `search_bind_password` | `secret` | See the section above. |
| `group_base` | `ou=groups,dc=example,dc=com` | groups for the items' groups = (no group_base: LDAP has none): a group is an entry under group_base that group_filter finds, {group} the item's group, {dn} the caller's entry, {user} their name; searched as search_bind_dn, else as the caller. |
| `group_filter` | `(&(cn={group})(\|(member={dn})(uniqueMember={dn})(memberUid={user})))` | groups for the items' groups = (no group_base: LDAP has none): a group is an entry under group_base that group_filter finds, {group} the item's group, {dn} the caller's entry, {user} their name; searched as search_bind_dn, else as the caller. |
| `password_change` | `exop` | how mode = passwd changes a password: exop (Password Modify, RFC 3062; OpenLDAP, 389 DS and most), or ad (Active Directory's unicodePwd, over ldaps:// or starttls only) |

## `[system]`

system: the accounts of this machine, by PAM: the auth and account stacks of pam_service (modemmenu.pam.sample as /etc/pam.d/modemmenu), so local accounts, sssd, Kerberos, whatever PAM is set up for.  Needs a build with PAM (make finds libpam's headers), and root for pam_unix.  Accounts with a uid below min_uid are not let in, nor root unless allow_root; with groups, only their members.

| key | sample | what it does |
|---|---|---|
| `pam_service` | `modemmenu` | See the section above. |
| `min_uid` | `1000` | See the section above. |
| `allow_root` | `no` | See the section above. |
| `groups` | `dialup` | See the section above. |

## `[sql]`

SQL: query gets args (user, password, in that order of placeholders) and returns one column: the stored password, checked as hash says (auto: by its prefix as in [file]; or bcrypt, sha256, sha512, plain).  hash = none: any row means the login is right (the query checks the password itself)

| key | sample | what it does |
|---|---|---|
| `driver` | `mysql` | mysql (or mariadb), postgres, sqlite. Also: `postgres`, `sqlite`. |
| `dsn` | `modem:secret@tcp(127.0.0.1:3306)/bbs` | mysql (or mariadb), postgres, sqlite. Also: `postgres://modem:secret@127.0.0.1/bbs?sslmode=disable`, `/var/lib/asterisk/users.db`. |
| `query` | `SELECT password FROM users WHERE username = ? AND enabled = 1` | mysql (or mariadb), postgres, sqlite. Also: `SELECT 1 FROM users WHERE username = $1 AND password = crypt($2, password)`, `SELECT password FROM users WHERE username = ?`. |
| `args` | `user` | mysql (or mariadb), postgres, sqlite. Also: `user password`. |
| `hash` | `auto` | mysql (or mariadb), postgres, sqlite. Also: `none`. |
| `update_query` | `UPDATE users SET password = ? WHERE username = ?` | mode = passwd items: update_query sets the password, with update_args (user, password, old_password, in the order of its placeholders); update_hash = bcrypt stores a bcrypt hash, none the password as typed (for a query that hashes it). Without update_query, SQL passwords cannot be changed from the menu. Also: `UPDATE users SET password = crypt($1, gen_salt('bf')) WHERE username = $2`. |
| `update_args` | `password user` | mode = passwd items: update_query sets the password, with update_args (user, password, old_password, in the order of its placeholders); update_hash = bcrypt stores a bcrypt hash, none the password as typed (for a query that hashes it). Without update_query, SQL passwords cannot be changed from the menu. |
| `update_hash` | `bcrypt` | mode = passwd items: update_query sets the password, with update_args (user, password, old_password, in the order of its placeholders); update_hash = bcrypt stores a bcrypt hash, none the password as typed (for a query that hashes it). Without update_query, SQL passwords cannot be changed from the menu. Also: `none`. |

## `[server]`

modemmenu -serve: one modemmenu for all calls, on a unix socket (or tcp:host:port).  Each connection gets a session of its own on a new pty, as app_dmodem's mode = exec would give it.

| key | sample | what it does |
|---|---|---|
| `listen` | `/run/modemmenu/modemmenu.sock` | See the section above. |
| `socket_mode` | `0660` | the socket's permissions: who may connect (app_dmodem: Asterisk's group) |
| `socket_group` | `asterisk` | the socket's permissions: who may connect (app_dmodem: Asterisk's group) |
| `header` | `yes` | the call's details come first on each connection, NAME=value lines and an empty line: app_dmodem's header = yes (DMODEM_*), or a menu item's header = yes (MODEM_*). no: the caller's bytes from the start. A header from app_dmodem's handoff = yes brings the modem's line with it (SCM_RIGHTS): the session runs on that terminal itself, nothing copied. |
| `header_timeout` | `10` | the call's details come first on each connection, NAME=value lines and an empty line: app_dmodem's header = yes (DMODEM_*), or a menu item's header = yes (MODEM_*). no: the caller's bytes from the start. A header from app_dmodem's handoff = yes brings the modem's line with it (SCM_RIGHTS): the session runs on that terminal itself, nothing copied. |
| `trust_user` | `no` | yes: a header with MODEM_USER (another modemmenu's item, header = yes) logs the caller in without asking again. Only for a socket that nothing but trusted menus can connect to. |
| `max_sessions` | `32` | connections at once; more are told the lines are busy. |

## `[authd]`

The auth socket: the BBS's other logins (telnet, SSH, the web) ask the backends above for a user's password through it, so that it is the same one.  Answered by -serve, or by -authd alone.  Who may ask: socket_mode and socket_group, and allow_users (the peer's user)

| key | sample | what it does |
|---|---|---|
| `listen` | `/run/modemmenu/auth.sock` | See the section above. |
| `socket_mode` | `0660` | See the section above. |
| `socket_group` | `bbs` | See the section above. |
| `allow_users` | `bbs` | See the section above. |
| `max_clients` | `16` | requests at once. |

## `[ppp_pool]`

The PPP callers' addresses: a command that names {ppp_remote} (or {ppp_local}, {ppp_dns}, {ppp_dns2}) takes the first free address of range for the call, so that several lines run PPP at once, and gives it back when pppd ends.  They are lock files in state_dir/ppp-pool (each with who has it), shared by every modemmenu on the host.  local is the server's end of every link; dns, the DNS servers offered (ms-dns), local by default (the resolver of dialup/: docs/dialup-network.md).  Pick a subnet none of the host's networks use.  No range: no pool.  The scripts see MODEM_PPP_LOCAL, MODEM_PPP_REMOTE, MODEM_PPP_DNS and MODEM_PPP_DNS2.

| key | sample | what it does |
|---|---|---|
| `local` | `100.120.224.1` | See the section above. |
| `range` | `100.120.224.10-100.120.255.254` | See the section above. |
| `dns` | `100.120.224.1` | See the section above. |

## `[autoppp]`

AutoPPP: a dialer that starts PPP at once (Windows' Dial-Up Networking, most others) sends a PPP frame where the login prompt wants a name; then command runs on the line instead (pppd), and its modemmenu.so plugin (make builds it with ppp-dev's headers; make install puts it in pppd's plugin directory) asks this call's auth socket for the PAP login: the backends check it as at the prompt, then item's access rules, then the time limits, whose time left becomes pppd's maxconnect.  PAP only: CHAP needs the passwords kept where they can be read back.  Fix the addresses (local:remote, from [ppp_pool]): the plugin lets the peer have the one pppd gives it.

| key | sample | what it does |
|---|---|---|
| `enabled` | `no` | See the section above. |
| `command` | `exec /usr/sbin/pppd nodetach lock passive local {ppp_local}:{ppp_remote} ms-dns {ppp_dns} ms-dns {ppp_dns2} require-pap refuse-chap refuse-mschap refuse-mschap-v2 refuse-eap plugin modemmenu.so ipparam autoppp` | See the section above. |
| `item` | `ppp` | the access rules (groups, min_baud, callerids...) a PPP login must pass. |

## `[aol]`

AOL: the classic AOL client (2.5 to 4.0) dials a network and its CCL script answers the network's prompts before it speaks P3.  modemmenu plays the network (AOLnet, SprintNet, Tymnet, as AOL 3.0's scripts wait for them), then joins the line to host: the BBS's AOL front end, where the caller signs on with their BBS account.  AOLnet needs its greeting (aolnet_prompt) said before the first login prompt, which every caller sees; and the name "aol" (AOLnet's answer) and "@D" (SprintNet's) are then the client's, not a user's.

| key | sample | what it does |
|---|---|---|
| `enabled` | `no` | See the section above. |
| `host` | `localhost:5190` | See the section above. |
| `networks` | `aolnet sprintnet tymnet p3` | which to play: aolnet sprintnet tymnet, and p3 (a CCL of one's own that starts P3 at once) |
| `aolnet_prompt` | `UQKT1` | AOLnet's greeting, which its script waits for. |
| `sprintnet_speed` | `11:14` | the answer to SprintNet's "par? 11", the line's speed: 11:14 is 9600 bps and up (the client then uses its high-speed settings) |

## `[dropfile]`

drop files for doors (an exec item's dropfile =): what they say of the BBS, and how the door reaches the caller.  The defaults are what Synchronet writes for a door on stdin and stdout.

| key | sample | what it does |
|---|---|---|
| `bbs_name` | `modemmenu` | See the section above. |
| `sysop` | `Sysop` | See the section above. |
| `security` | `10` | the callers' security level (an item's security = overrides it) |
| `emulation` | `auto` | auto (ASCII for a terminal detected without ANSI: VT52, PETSCII, a plain one; ANSI otherwise), ansi or ascii. |
| `comm_type` | `0` | DOOR32.SYS's comm type: 0 local (stdin and stdout: a door on the line), 1 serial, 2 telnet; its handle (-1 for 0, else 0 by default) |
| `comm_handle` | `-1` | DOOR32.SYS's comm type: 0 local (stdin and stdout: a door on the line), 1 serial, 2 telnet; its handle (-1 for 0, else 0 by default) |
| `com_port` | `COM1:` | DOOR.SYS's and DORINFO's port: COM1: (COM0: is local); a DOS door under DOSEMU wants the port DOSEMU gives it, with a FOSSIL driver (DORINFO's FOSSIL flag) |
| `fossil` | `yes` | DOOR.SYS's and DORINFO's port: COM1: (COM0: is local); a DOS door under DOSEMU wants the port DOSEMU gives it, with a FOSSIL driver (DORINFO's FOSSIL flag) |
| `rate` | `38400` | the rate the files give: a FOSSIL's rate (doors built on DMlib refuse others, such as a modem's 26400), or connect for the modems'. |
| `four_digit_years` | `no` | MM/DD/YYYY instead of MM/DD/YY in DOOR.SYS (some newer doors) |
| `max_nodes` | `64` | doors running at once, each on a node of its own (state_dir/nodes); 15 at most with dorinfo#.def. |

## `[menu]`

The menu: its title, prompt, items and the words that hang up.

| key | sample | what it does |
|---|---|---|
| `title` | `\r\nMain menu\r\n` | See the section above. |
| `prompt` | `\r\nSelection: ` | See the section above. |
| `items` | `bbs ppp shell passwd` | the items in order (default: every [item ...] in file order) |
| `hangup` | `G goodbye bye logoff` | the caller types a choice and Enter: an item's key, its name or one of its aliases, in any case (P, p, ppp). ? shows the menu again. What hangs up (the first one is shown in the menu): |
| `hotkeys` | `no` | yes: one key selects at once, without Enter (one-character keys only) |

## `[item NAME]`

A menu item: a program on the line, a server to connect the caller to, or a built-in (see the item keys below).

There may be any number of these, each with its own NAME.

| key | sample | what it does |
|---|---|---|
| `key` | `B` | its key in the menu, one or more characters (default: its name's first letter) Also: `P`, `PW`, `S`, `M`, `L`, `LC`, `U`, `K`, `W`, `N`, `C`, `GO`. |
| `aliases` | `...` | more words that select it (its name always does) Also: `dialup net`. |
| `label` | `...` | its line in the menu. Also: `Bulletin board`, `PPP (dial-up networking)`, `Change your password`, `Shell`, `More services`, `The BBS`, `Legend of the Red Dragon`, `Last callers`, `Upload a file (ZMODEM)`, `Kermit file server`, `The web (lynx)`, `Mail`, `Usenet news`, `Chat (IRC)`, `Gopherspace`. |
| `mode` | `exec` | a program on the line: command (by /bin/sh -c); `passwd`: built in: the caller changes their password (min_password = 8 characters); `callers`: built in: the last callers (count = 10), the time, user and rate of each login (state_dir/callers, the latest 200; kept only with such an item); `tcp`: a TCP server, raw: host, port; send is sent first; `unix`: a server on a unix socket, raw: path; send too (tcp, telnet and rlogin take a path as well, for their protocol on a unix socket); `telnet`: a telnet server: host, port (23), terminal (ANSI); `rlogin`: an rlogin server: host, port (513), and rlogin_client_user ({password}), rlogin_server_user ({user}), rlogin_terminal (ansi/{baud}) Also: `rlogin`, `tcp`, `passwd`, `unix`, `callers`. |
| `run_as` | `{user}` | exec as this system user (modemmenu must be root) |
| `tty` | `raw` | the line as app_dmodem has it; sane: cooked, echo, CR to LF, for programs that want a terminal. Also: `sane`. |
| `pass_password` | `yes` | the password in {password} and MODEM_PASSWORD. Also: `no`. |
| `session_file` | `no` | yes: the session as JSON in a 0600 file, its path in MODEM_SESSION_FILE (removed after) |
| `after` | `hangup` | when the item ends: hang up, or back to the menu. Also: `menu`. |
| `connect_timeout` | `10` | seconds to reach a server. |
| `terminal` | `ANSI` | telnet: the terminal type it tells the server ({term} and the other placeholders work) |
| `header` | `no` | tcp, unix, telnet, rlogin: yes sends the session first, MODEM_* NAME=value lines and an empty line (the password only with pass_password), for modemmenu -serve or any program that reads it. Also: `yes`. |
| `handoff` | `no` | unix with header = yes: yes hands the server the caller's line itself (SCM_RIGHTS, with the header) instead of copying bytes to it; the server gives it back by closing the connection (the bbs's listen with handoff = yes takes it) Also: `yes`. |
| `users` | `alice` | bob these users ... |
| `groups` | `sysops` | ... or members of these groups ([groups], or the system's for auth = system) Also: `dialup`. |
| `min_baud` | `9600` | the connect rate at least this (unknown: no) |
| `callerids` | `555*` | the caller ID matching one of these. |
| `guest` | `no` | yes: guests see it too. |
| `dropfile` | `door32.sys` | door.sys dorinfo1.def dorinfo#.def written before the program starts, removed after, with the user, the rate and the time left; the node, the directory and the first file are {node}, {dropfile_dir} and {dropfile} in command, MODEM_NODE, MODEM_DROPFILE_DIR and MODEM_DROPFILE in the environment. dorinfo#.def: DORINFO1.DEF to DORINFOF.DEF by the node. Also: `door.sys dorinfo1.def`. |
| `dropfile_lowercase` | `no` | yes: door32.sys..., for doors on Linux. |
| `door_sys_lines` | `52` | 31: the old DOOR.SYS (the rest blank), for doors that do not take the longer one. |
| `security` | `0` | the drop files' security level ([dropfile]'s if 0) |
| `host` | `127.0.0.1` | See the section above. |
| `port` | `513` | or a BBS that reads its login from the line: Also: `2323`. |
| `rlogin_client_user` | `{password}` | See the section above. |
| `rlogin_server_user` | `{user}` | See the section above. |
| `rlogin_terminal` | `ansi/{baud}` | See the section above. |
| `send` | `{user}\r{password}\r` | or a BBS that reads its login from the line: |
| `time_warning` | `no` | no time warning in the middle of PPP's frames. |
| `command` | `exec /usr/sbin/pppd nodetach noauth local lock passive {ppp_local}:{ppp_remote} ms-dns {ppp_dns} ms-dns {ppp_dns2} ipparam {user}` | no time warning in the middle of PPP's frames. Also: `exec "$SHELL" -l`, `cd /srv/doors/lord && exec dosemu -dumb -E "lord.bat $MODEM_NODE"`, `cd "$HOME/uploads" && exec rz -b -R -E`, `cd "$HOME" && exec kermit -Y -x`, `exec lynx -anonymous https://example.org/`, `exec mutt`, `exec tin -r`, `exec irssi -c 127.0.0.1 -n {user}`, `exec lynx -anonymous gopher://gopher.floodgap.com/`. |
| `min_password` | `8` | See the section above. |
| `path` | `/run/modemmenu/more.sock` | See the section above. Also: `/run/bbs/handoff.sock`. |
| `dropfile_dir` | `/srv/doors/lord/node{node}` | See the section above. |
| `count` | `10` | See the section above. |

**See also:** [Configuration](configuration.md)
