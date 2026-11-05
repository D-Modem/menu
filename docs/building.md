# Building


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
