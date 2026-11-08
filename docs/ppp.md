# PPP dialers

## PPP dialers: AutoPPP and PAP


Windows' Dial-Up Networking and most PPP dialers never see a login
prompt: they start PPP as soon as the modems connect, and log in inside
it with PAP.  With `[autoppp] enabled = yes`, a PPP frame where the
login prompt wants a name makes modemmenu run `[autoppp] command` (pppd)
on the line instead:

```
[autoppp]
enabled = yes
command = exec /usr/sbin/pppd nodetach lock passive local {ppp_local}:{ppp_remote} ms-dns {ppp_dns} ms-dns {ppp_dns2} require-pap refuse-chap refuse-mschap refuse-mschap-v2 refuse-eap plugin modemmenu.so ipparam autoppp
item = ppp
```

pppd's `modemmenu.so` plugin ([pppd/modemmenu.c](../pppd/modemmenu.c);
`make` builds it when `ppp-dev`'s headers are there, `make install`
puts it in pppd's plugin directory) asks modemmenu for the PAP login on
an auth socket of the call's own, in pppd's environment as
`MODEMMENU_AUTH_SOCKET` (or `{auth_socket}` for its `modemmenu-socket`
option).  modemmenu checks the login as at the prompt: the backends,
`user_pattern`, `attempts`, `fail_delay`; then the access rules of
`[autoppp] item` (its `users`, `groups`, `min_baud`, `callerids`);
then the time limits.  The time left becomes pppd's `maxconnect` (and
modemmenu hangs pppd up soon after, should the link outlive it); the
call is logged, counted in the day's use and the last callers as a
login at the prompt is.  pppd's scripts get the caller's name
(`PEERNAME`) and, from modemmenu, `MODEM_*` (the dialed number, the
caller ID, the rate).

- **PAP only.** CHAP and MS-CHAPv2 need the server to keep each
  password where it can be read back, or an equivalent of it;
  modemmenu's logins (bcrypt hashes, LDAP, PAM) do not.  The sample's
  pppd refuses them.
- **PAP's password crosses the line in clear.** Over the telephone
  network that was the line's privacy; over SIP it is in the call's RTP:
  give the trunk SRTP (and SIP over TLS), or keep it on a network you
  trust.
- **Fix the addresses** (`local:remote` on pppd's command line, from
  `[ppp_pool]`): the plugin lets the logged-in peer have the address pppd
  gives it.
- The plugin is built for the pppd of the headers it is built against
  (`pppd_version`); rebuild it when pppd is upgraded.

## Addresses, DNS and the callers' network

Each PPP caller gets an address of its own from `[ppp_pool]`, a DNS
server with overrides for the services this system provides, and a
network kept apart from the host's: see
[The dial-up network](dialup-network.md).
