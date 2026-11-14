# Tests


`make test` does the checks that follow.

- It checks the password schemes, the file and SQLite backends, the
  menu choices and the telnet client.
- It runs modemmenu on a pty, as app_dmodem does. On the pty, it tests
  these items: a wrong login and a correct login, the message of the
  day, choices by key, by name and by alias, an unknown choice and `?`,
  an rlogin BBS and the return to the menu, a cooked shell, and the
  environment and session file of an exec selection.
- It runs `modemmenu -serve` on a unix socket. It tests these items: a
  session from an app_dmodem header (on a different pty), a caller who
  hangs up, a transfer of a logged-in caller from a different modemmenu
  through a `mode = unix` item, and `SIGTERM`.
- It tests drop files line by line in all three formats. The tests
  include the defaults of Synchronet, the other settings, the 31-line
  `DOOR.SYS`, and names that are split for `DORINFO1.DEF`. It also tests
  drop files on a call: the files, their names, the node, the
  environment and the removal of the files after the call.
- Calls test which items each caller can see (users, groups,
  `min_baud`, caller ID). Calls also test a guest, signups (logged in
  immediately, and pending), refused numbers and known numbers. They
  also test a selection that modemmenu hangs up at the daily limit, and
  the next call of that day, which modemmenu refuses.
- It tests password changes in the users file and in SQLite. In the
  users file, the other lines and notes must stay. On a call, it tests a
  wrong current password, the rules for the new password, the change,
  the next login with the new password, and a guest.
- It tests the auth socket: correct and wrong passwords, unknown,
  malformed and locked users, `allow_users` and its settings.
- It tests AutoPPP with the real `modemmenu.so`. A stand-in for pppd
  loads the plugin ([pppd/test/host.c](../pppd/test/host.c)). The
  stand-in exports the same items that pppd exports to plugins. This
  test must have a C compiler and `ppp-dev`. The test examines these
  items:
  - PPP frames, which modemmenu must tell apart from names.
  - Correct and wrong PAP logins.
  - `maxconnect` from the time limit.
  - The access item, `attempts`, and one login for each call.
  - On a call, a frame from a dialer at the login prompt. The frame must
    get to the pppd plugin through the socket of the call.

  A real pppd must have a kernel with PPP (`ppp_generic`).
- It tests the time warnings on a call. There must be two warnings, with
  the longer warning first. With `time_warning = no`, there must be no
  warning.
- It tests the list of the last callers on two calls. The list keeps a
  maximum of 200 callers, shows the newest caller first and shows no
  caller ID.
- It tests the password encoding of Active Directory.
- It tests the connect rate as `{speed}`: the same rate in the two
  directions, different rates, and an unknown rate. It also tests the
  connect rate on calls: after the banner, removed when there is no
  rate, its own text, and disabled.
- It tests the uid, root and group rules of the system backend, with a
  replacement for PAM.

Two more tests run if you give them their servers:

- `MODEMMENU_TEST_PAM=user:password make test`, as root with
  `/etc/pam.d/modemmenu`: this tests PAM against a real account. It
  changes the password of the account and then changes it back. The
  service must have a password stack.
- `MODEMMENU_TEST_LDAP=ldap://host:port make test`: this tests the LDAP
  backend against a server that has
  `uid=lduser,ou=people,dc=example,dc=com` (password `ldpass`). It
  changes that password and then changes it back. The server must let
  users write their own `userPassword`. If
  `ou=groups,dc=example,dc=com` contains the groups that follow, the
  test also examines LDAP groups:
  - `cn=sysops` (`groupOfNames`, `member` lduser)
  - `cn=dialup` (`posixGroup`, `memberUid` lduser)
  - `cn=other` (without lduser)
