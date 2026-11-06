# modemmenu documentation

- [Building](building.md): Go, PAM, `make install`
- [Running it from app_dmodem](running.md): a program per call, one server on a unix socket, FreePBX
- [Configuration](configuration.md): `modemmenu.conf`, the connect rate, terminals, the message of the day, the menu and its items, guests and sign-up, caller ID, time limits, drop files, last callers, handing callers on
- [Configuration reference](reference.md): every section and key, from the sample
- [Authentication](authentication.md): users file, LDAP, SQL, system accounts (PAM), changing passwords, the auth socket
- [PPP dialers](ppp.md): AutoPPP and PAP, the pppd plugin
- [Security notes](security.md)
- [Tests](testing.md)

The samples: [modemmenu.conf.sample](../samples/modemmenu.conf.sample), [modemmenu.users.sample](../samples/modemmenu.users.sample), [modemmenu.pam.sample](../samples/modemmenu.pam.sample), [modemmenu.service.sample](../samples/modemmenu.service.sample), [modemmenu-authd.service.sample](../samples/modemmenu-authd.service.sample).
