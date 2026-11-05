# Tests


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
`allow_users` and its settings.  AutoPPP is tested with the real
`modemmenu.so` loaded by a stand-in for pppd ([pppd/test/host.c](../pppd/test/host.c),
exporting what pppd exports to plugins; this needs a C compiler and
`ppp-dev`): PPP frames told from names, PAP logins right and wrong,
`maxconnect` from the time limit, the access item, `attempts`, one login
a call, and on a call a dialer's frame at the login prompt reaching
pppd's plugin through the call's socket.  A real pppd needs a kernel
with PPP (`ppp_generic`).  The time warnings are tested on a call
(two, the longer first, and none with `time_warning = no`), the last callers list
(kept to 200, the newest first, no caller ID) on two calls, and Active
Directory's password encoding.  The connect rate is tested as `{speed}` (the same both ways, apart, not
known) and on calls (after the banner, left out without a rate, its own
text, off).  The system backend's uid, root and
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
