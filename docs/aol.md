# AOL callers

## The classic AOL client, dialing in

AOL's own client (2.5 to 4.0) never types at a login prompt. It dials a
network, AOLnet, SprintNet or Tymnet, and its CCL script answers that
network's prompts. Then it speaks P3, AOL's own protocol, to the AOL host.
With `[aol] enabled = yes`, modemmenu plays the network and then joins the
line to `host`. That is [D-Modem/bbs](https://github.com/D-Modem/bbs)'s
AOL front end (`[aol]` there), where the caller signs on with their BBS
name and password.

```
[aol]
enabled = yes
host = localhost:5190
```

modemmenu tells which network the client expects from what its script
does at the login prompt. The strings are the ones AOL 3.0's
`AOLNET.CCL`, `SPRINT.CCL` and `TYMNET.CCL` wait for.

| Network | The script | modemmenu |
|---|---|---|
| AOLnet | waits for `UQKT1`, answers `aol` | says `UQKT1` before the first login prompt, then `CONNECTED` |
| SprintNet | sends `@D` | asks `TERMINAL=`, gives the `@` prompt, answers `par? 11` with the line's speed, and `C <address>` with `<address> CONNECTED` |
| Tymnet | sends `A`, with no line end | asks `PLEASE LOG IN:`, answers the login with `+` and the host's name with `CALL CONNECTED` |
| P3 | a script of one's own that starts P3 at once | joins the line to the host at the first packet |

In the client's Setup, add the D-Modem line's phone number as an access
number of the network to play, and pick it.

`networks` limits which of these modemmenu plays. Leave out `aolnet` to
keep its `UQKT1` line off every caller's screen. With AOLnet or SprintNet
played, the names `aol` and `@D` at the login prompt are the client's,
not a user's. A caller's name beginning with `A` is still a name, because
a person types on while the Tymnet script waits seconds for an answer.

The call is logged (`AOL client (sprintnet) at the login prompt`, then
`aol: to localhost:5190`, then how long it lasted). modemmenu's own
logins, time limits and menus don't apply to it: the BBS signs the caller
on and keeps its own limits.
