# Authentication

This page tells how modemmenu checks the name and password of a caller, and how a caller can change the password.

## Authentication


`[general] auth` lists the backends. modemmenu asks them in that
sequence. If a backend does not know the user, modemmenu asks the next
backend. If a backend knows the user and the password is wrong, the
attempt ends there. If a backend does not operate (LDAP is not
available, or there is an SQL error), modemmenu records this in the log
and skips the backend.

- **file**: `user:password` lines. The password is one of these:
  a bcrypt hash (`modemmenu -hash` prints one), `{SHA256}`/`{SHA512}`
  (hex or base64), `{SSHA256}`/`{SSHA512}` (salted, base64) or
  `{PLAIN}text`. modemmenu reads the file at each login.
- **ldap**: a bind as the user. The bind goes to `bind_dn` with `{user}`
  in it, or to the entry that `search_filter` finds under `search_base`.
  modemmenu supports ldaps://, StartTLS and a private CA. Put a group
  check in the filter (`memberOf=...`). modemmenu escapes the values for
  DNs and filters.
- **sql**: `query` gets the user (and, as an option, the password) as
  arguments. It returns the stored password, and modemmenu checks it in
  the same way as for the file. Alternatively, with `hash = none`, a row
  in the result means that the login is correct. Use this for a database
  that checks passwords itself (`crypt()` in PostgreSQL).
- **system** (or **pam**): the accounts of the machine, through PAM.
  Refer to the section below.

User names must agree with `user_pattern` (by default, letters, digits
and `._@-`). After a wrong login, modemmenu waits `fail_delay` seconds.
A login with an unknown user takes the same time as a login with a known
user. modemmenu hangs up the call after `attempts` tries or after
`login_timeout`.

## System accounts (PAM)


`auth = system` runs the auth and account stacks of the PAM service
`pam_service` (by default, `modemmenu`). Thus local accounts operate, and
all other items that PAM has a configuration for (sssd, Kerberos, OTP
modules). PAM does its own checks for expired and locked accounts. This
backend must have a build with PAM (refer to
[Building](building.md)). To configure it, do these steps:

1. Install the PAM service. `make install` puts
   [modemmenu.pam.sample](../samples/modemmenu.pam.sample) in `/etc/pam.d/modemmenu`.
   The sample uses `common-auth`, `common-account` and, for password
   changes, `common-password` of the distribution.
2. For local accounts, run Asterisk (and thus modemmenu) as root.
   `pam_unix` checks the passwords of other users only for root.
3. Add `system` to `[general] auth`. Then set these options in
   `[system]`:

   | option | default | |
   |---|---|---|
   | `pam_service` | `modemmenu` | the file in `/etc/pam.d` |
   | `min_uid` | `1000` | accounts below this value (system accounts) cannot log in |
   | `allow_root` | `no` | `yes` lets root log in |
   | `groups` | | only members of these groups can log in (membership or primary group) |

If these rules refuse an account, the account is "unknown" to this
backend. modemmenu asks the next backend and records the reason in the
log. With `run_as = {user}`, a selection (for example, the shell) runs
as the account of the caller.

## Changing passwords


A `mode = passwd` item lets callers change their own password. The
item does not have a command. By default, it goes back to the menu
(`after = menu`). The caller does these steps:

1. Type the current password.
2. Type the new password two times. The new password must have a minimum
   of `min_password` characters of the item (8 by default). It must be
   different from the current password. The caller has three tries.

modemmenu checks the current password against the backends again. This
check also finds the backend that holds the account. A login by caller
ID or by an upstream menu does not give this information. The password
changes in that backend:

| backend | how |
|---|---|
| file | modemmenu writes the line of the user again in the same location, under the lock of the file. It keeps a note after the hash. The password is bcrypt, or text with `hash = plain` |
| sql | `[sql] update_query` with `update_args` (`user`, `password`, `old_password`, in the sequence of its placeholders). `update_hash = bcrypt` stores a bcrypt hash. `none` stores the password as the caller typed it, for a query that makes the hash itself. Without `update_query`, you cannot change SQL passwords here |
| ldap | modemmenu binds as the user, thus the password policy and hash of the server apply. It uses the Password Modify extended operation (RFC 3062, for OpenLDAP, 389 DS and most other servers). With `[ldap] password_change = ad`, it uses the method of Active Directory: it deletes the old value of `unicodePwd` and adds the new value. AD accepts this only on `ldaps://` or `starttls` |
| system | the password stack of `pam_service` in PAM. The caller sees the messages of PAM (for example, the reasons from a quality module) |

If the policy of the server or of PAM refuses the new password, the
old password stays. After a change, the selections get the new password
in `MODEM_PASSWORD`. Guests have no password to change. The log records
which user changed the password and in which backend. It never records
the password.

When it runs as root, `pam_unix` does no quality checks. Its `minlen`
and similar options do not apply to root. For a policy, put
`pam_pwquality` before `pam_unix` with `enforce_for_root`, as
[modemmenu.pam.sample](../samples/modemmenu.pam.sample) shows.

## The same password elsewhere: the auth socket


Callers who log in here are already logged in when they get to the
BBS. The same persons can also log in to the BBS in a different way
(telnet, SSH, the web forum of the BBS). They can use the same password,
because the BBS asks the backends of modemmenu through a unix socket.

```
[authd]
listen = /run/modemmenu/auth.sock
socket_mode = 0660
socket_group = bbs
; and, closer: only these users' programs (by the peer's uid)
allow_users = bbs
```

`modemmenu -serve` answers on this socket, and also serves its sessions.
`modemmenu -authd` answers on this socket only
([modemmenu-authd.service.sample](../samples/modemmenu-authd.service.sample)).
Each connection has one request, with one JSON line in each direction:

```
{"user": "alice", "password": "..."}
{"ok": true, "auth": "ldap"}          or          {"ok": false, "error": "denied"}
```

modemmenu asks the backends in the same way as at the login prompt, and
applies `user_pattern`. A refusal comes after `fail_delay`. modemmenu
also refuses accounts that are locked in the users file (a signup that
the sysop did not unlock). If a caller changes a password from the menu
(`mode = passwd`), the socket immediately uses the new password. For the
configuration on the BBS side, refer to its `[modemmenu] auth_socket`
(docs/wiki/Modem-Callers.md in the BBS repository).
