# Authentication

How modemmenu checks a caller's name and password, and lets them change it.

## Authentication


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

## System accounts (PAM)


`auth = system` runs the auth and account stacks of the PAM service
`pam_service` (`modemmenu` by default), so local accounts and anything
PAM is set up for (sssd, Kerberos, OTP modules) work, with PAM's own
checks for expired and locked accounts.  It needs a build with PAM
(above).  To set it up:

1. Install the PAM service: `make install` puts
   [modemmenu.pam.sample](../modemmenu.pam.sample) in `/etc/pam.d/modemmenu`
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

## Changing passwords


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
[modemmenu.pam.sample](../modemmenu.pam.sample) shows.

## The same password elsewhere: the auth socket


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
