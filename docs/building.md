# Building


Go 1.22 or later:

```
make                    # or: go build ./cmd/modemmenu (-tags pam for PAM)
make PAM=0              # without PAM, when libpam's headers are there
make test
make install            # /usr/local/bin/modemmenu, and the samples into
                        # /etc/asterisk (and /etc/pam.d) if there are none
```

`make` builds with PAM when libpam's headers are installed
(`libpam0g-dev` on Debian and Ubuntu, `pam-devel` on Fedora).  Without
them modemmenu builds with the other backends, and `auth = system` is a
configuration error.

## The source

One Go module, one program:

| Where | What |
| --- | --- |
| `cmd/modemmenu/` | the program: the login, the menu, access rules and time limits, the selections, `-serve`, AutoPPP, drop files, the auth socket's server |
| `internal/config/` | modemmenu.conf's sections and keys, and the `{name}` templates |
| `internal/term/` | the caller's line: reads, writes and line editing, terminal detection, PETSCII |
| `internal/auth/` | the backends (users file, LDAP, SQL, system by PAM) and password changes |
| `internal/telnet/` | the telnet client of `mode = telnet` items |
| `authd/` | the auth socket's protocol and a client, for other programs (a BBS) to import |
| `pppd/` | pppd's `modemmenu.so` plugin, in C |
| `dialup/` | the PPP callers' network: VRF, firewall, DNS with overrides (`make install-dialup`) |
| `samples/` | modemmenu.conf, the users file, PAM and systemd samples |
| `freepbx/` | the FreePBX setup |
| `tools/confref/` | makes docs/reference.md from samples/modemmenu.conf.sample |

