# PPP dialers

## PPP dialers: AutoPPP and PAP


Dial-Up Networking of Windows and most PPP dialers never see a login
prompt. They start PPP immediately when the modems connect. Then they log
in with PAP, inside PPP. With `[autoppp] enabled = yes`, modemmenu looks
for a PPP frame at the login prompt, where it expects a name. If it gets
a PPP frame, it runs `[autoppp] command` (pppd) on the line:

```
[autoppp]
enabled = yes
command = exec /usr/sbin/pppd nodetach lock passive local {ppp_local}:{ppp_remote} ms-dns {ppp_dns} ms-dns {ppp_dns2} require-pap refuse-chap refuse-mschap refuse-mschap-v2 refuse-eap plugin modemmenu.so ipparam autoppp
item = ppp
```

The source of the `modemmenu.so` plugin for pppd is
[pppd/modemmenu.c](../pppd/modemmenu.c). If the headers of `ppp-dev`
are installed, `make` builds the plugin. `make install` puts it in the
plugin directory of pppd.

The plugin asks modemmenu for the PAP login on an auth socket. Each call
has its own auth socket. The environment of pppd gives the socket as
`MODEMMENU_AUTH_SOCKET`. You can also give it as `{auth_socket}` to the
`modemmenu-socket` option of the plugin.

modemmenu checks the login in the same way as at the prompt. It does
these checks in this sequence:

1. The backends, `user_pattern`, `attempts` and `fail_delay`.
2. The access rules of `[autoppp] item` (its `users`, `groups`,
   `min_baud` and `callerids`).
3. The time limits.

The time that remains becomes the `maxconnect` of pppd. If the link
continues after that time, modemmenu hangs up pppd a short time after.
modemmenu records the call in the log. It counts the call in the daily
use and in the last callers, the same as a login at the prompt. The
scripts of pppd get the name of the caller (`PEERNAME`). They also get
`MODEM_*` from modemmenu (the dialed number, the caller ID and the
rate).

- **PAP only.** For CHAP and MS-CHAPv2, the server must keep each
  password, or an equivalent, in a form that it can read back. The
  logins of modemmenu (bcrypt hashes, LDAP, PAM) do not keep passwords in
  this form. The pppd in the sample refuses CHAP and MS-CHAPv2.
- **The PAP password goes across the line as clear text.** On the
  telephone network, the privacy of the line protected it. On SIP, the
  password is in the RTP of the call. Give SRTP (and SIP over TLS) to
  the trunk, or keep the trunk on a network that you trust.
- **Fix the addresses** (`local:remote` on the command line of pppd,
  from `[ppp_pool]`). The plugin lets the logged-in peer use the address
  that pppd gives it.
- The plugin is built for the pppd that agrees with its headers
  (`pppd_version`). When you upgrade pppd, build the plugin again.

## Addresses, DNS and the callers' network

Each PPP caller gets these items:

- Its own address from `[ppp_pool]`.
- A DNS server with overrides for the services of this system.
- A network that is isolated from the network of the host.

Refer to [The dial-up network](dialup-network.md).
