# Security notes


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
- AutoPPP's PAP sends the password in clear on the line, as any PAP
  does: on a SIP trunk, give it SRTP or a network you trust.
- The log (syslog's auth facility by default) gets logins, failures and
  selections, never passwords.  It is never the line: the caller would
  see it.
