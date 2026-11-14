# modemmenu documentation

- [Building](building.md): Go, PAM and `make install`.
- [Running it from app_dmodem](running.md): one program for each call, one server on a unix socket, and FreePBX.
- [Configuration](configuration.md): `modemmenu.conf`, the connect rate, terminals, the message of the day, the menu (full screen or lines) and its items, guests and sign-up, caller ID, time limits, drop files, last callers, and the transfer of callers to a different program.
- [Configuration reference](reference.md): all sections and keys, from the sample.
- [The sysop's setup](setup.md): `modemmenu -setup`, to change the configuration on a full screen, at the console or on the line.
- [Authentication](authentication.md): users file, LDAP, SQL, system accounts (PAM), password changes and the auth socket.
- [PPP dialers](ppp.md): AutoPPP and PAP, and the pppd plugin.
- [AOL callers](aol.md): the classic AOL client when it dials in, and the emulation of its network.
- [Security notes](security.md).
- [Tests](testing.md).

The samples: [modemmenu.conf.sample](../samples/modemmenu.conf.sample), [modemmenu.users.sample](../samples/modemmenu.users.sample), [modemmenu.pam.sample](../samples/modemmenu.pam.sample), [modemmenu.service.sample](../samples/modemmenu.service.sample), [modemmenu-authd.service.sample](../samples/modemmenu-authd.service.sample).
