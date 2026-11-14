# AOL callers

## The classic AOL client, dialing in

The AOL client (versions 2.5 to 4.0) never types at a login prompt. It
dials a network (AOLnet, SprintNet or Tymnet), and its CCL script answers
the prompts of that network. Then it uses P3, the protocol of AOL, to
communicate with the AOL host. With `[aol] enabled = yes`, modemmenu
emulates the network. Then it connects the line to `host`. That host is
the AOL front end of [D-Modem/bbs](https://github.com/D-Modem/bbs)
(`[aol]` in its configuration). There, the caller signs on with the BBS
name and password.

```
[aol]
enabled = yes
host = localhost:5190
```

modemmenu finds the network that the client expects from the actions
of its script at the login prompt. The strings are the strings that
`AOLNET.CCL`, `SPRINT.CCL` and `TYMNET.CCL` of AOL 3.0 wait for.

| Network | The script | modemmenu |
|---|---|---|
| AOLnet | waits for `UQKT1`, answers `aol` | sends `UQKT1` before the first login prompt, then `CONNECTED` |
| SprintNet | sends `@D` | asks `TERMINAL=`, gives the `@` prompt, answers `par? 11` with the speed of the line, and answers `C <address>` with `<address> CONNECTED` |
| Tymnet | sends `A`, with no line end | asks `PLEASE LOG IN:`, answers the login with `+`, and answers the name of the host with `CALL CONNECTED` |
| P3 | a custom script that starts P3 immediately | connects the line to the host at the first packet |

In the Setup of the client, do these steps:

1. Add the phone number of the D-Modem line as an access number of the
   network that modemmenu must emulate.
2. Select that access number.

`networks` limits the networks that modemmenu emulates. If you remove
`aolnet`, the `UQKT1` line does not show on the screen of each caller. If
modemmenu emulates AOLnet or SprintNet, the names `aol` and `@D` at the
login prompt come from the client, not from a user. A name that starts
with `A` is still a user name. The Tymnet script waits some seconds for
an answer, but a person continues to type.

modemmenu records the call in the log. It records
`AOL client (sprintnet) at the login prompt`, then
`aol: to localhost:5190`, then the duration of the call. The logins, time
limits and menus of modemmenu do not apply to this call. The BBS signs
the caller on and applies its own limits.
