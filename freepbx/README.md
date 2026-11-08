# modemmenu on FreePBX

A ready-made setup for [FreePBX](https://www.freepbx.org/) 16 and 17:
callers dial in, their modem connects to
[D-Modem](https://github.com/D-Modem/D-Modem), and modemmenu asks for a
user name and password from a text file before showing its menu.

| file | goes to | |
|---|---|---|
| [modemmenu.conf](modemmenu.conf) | `/etc/modemmenu/modemmenu.conf` | the login and the menu |
| [modemmenu.users](modemmenu.users) | `/etc/modemmenu/modemmenu.users` | the callers' names and passwords |
| [modemmenu.service](modemmenu.service) | `/etc/systemd/system/modemmenu.service` | runs modemmenu for every call |
| [extensions_custom.conf](extensions_custom.conf) | add to `/etc/asterisk/extensions_custom.conf` | an optional test extension |

The files live in `/etc/modemmenu`, not `/etc/asterisk`, so that FreePBX's
`fwconsole chown` never opens the users file to the asterisk user.

## How a call goes

```
caller's modem ──SIP──▶ FreePBX inbound route ──▶ D-Modem: menu
                                                   (dmodem-ari or app_dmodem)
                                                         │ unix socket, call details first
                                                         ▼
                                    modemmenu -serve ──▶ login from modemmenu.users
                                                    ──▶ menu: PPP, last callers, password
```

One `modemmenu -serve` answers every call on
`/run/modemmenu/modemmenu.sock`.  The socket belongs to the asterisk
group, so Asterisk and dmodem-ari can connect and other users cannot.
Each call gets a session of its own.

## Setting it up

This assumes D-Modem's FreePBX module is installed, with dmodem-ari or
app_dmodem, as in D-Modem's
[docs/freepbx.md](https://github.com/D-Modem/D-Modem/blob/main/docs/freepbx.md).

1. **Build modemmenu.**  It needs Go 1.22 or later; Debian 12 (FreePBX
   17) ships Go 1.19, so build it on another machine or use
   bookworm-backports.  This setup does not use PAM, so a static build
   copies to any x86-64 Linux:

       CGO_ENABLED=0 make PAM=0
       scp modemmenu root@freepbx:/usr/local/bin/

   Or on the FreePBX machine itself: `make PAM=0 install-freepbx`,
   which also does steps 2 and 3 without overwriting files that are
   already there.

2. **Install the files** (as root):

       install -d -m 755 /etc/modemmenu
       install -m 644 freepbx/modemmenu.conf    /etc/modemmenu/
       install -m 600 freepbx/modemmenu.users   /etc/modemmenu/
       install -m 644 freepbx/modemmenu.service /etc/systemd/system/

3. **Add the callers.**  `modemmenu -hash` asks for a password and prints
   its bcrypt hash; each caller is a `name:hash` line in
   `/etc/modemmenu/modemmenu.users`:

       echo "alice:$(modemmenu -hash)" >> /etc/modemmenu/modemmenu.users

   For a first test, `test:{PLAIN}changeme` will do; take it out after.
   The file is read at each login, so changes need no restart.

4. **Check and start it:**

       modemmenu -check -config /etc/modemmenu/modemmenu.conf
       systemctl daemon-reload
       systemctl enable --now modemmenu
       ls -l /run/modemmenu/modemmenu.sock    # srw-rw---- root asterisk

5. **Add the D-Modem profile** in **Applications > D-Modem > Profiles**:

   | field | value |
   |---|---|
   | Name | `menu` |
   | Data goes to | A server on a unix socket |
   | Socket | `/run/modemmenu/modemmenu.sock` |
   | Call details first | on |

   Leave the rest at their defaults.  This writes a `[menu]` profile with
   `mode = unix` and `header = yes` into `/etc/asterisk/dmodem.conf`, and
   the destination `app-dmodem,menu,1`.

6. **Send calls to it.**  In **Connectivity > Inbound Routes**, pick
   **D-Modem: menu** as the destination of the modem line's DID.  Click
   **Apply Config**.  With dmodem-ari, restart it so that it reads the new
   profile.

7. **Test it** (optional).  Add [extensions_custom.conf](extensions_custom.conf)'s
   lines to `/etc/asterisk/extensions_custom.conf` and run
   `fwconsole reload`: a modem on one of your extensions (an ATA) dialling
   `6368` reaches the menu without going through the trunk.

Keep the trunk and the extension to one codec (ulaw or alaw), with fax
detection off; D-Modem's line check page lists what is still in the way.

## The menu

- **PPP**: dial-up networking.  `apt install ppp` first.  Each caller
  gets an address of `[ppp_pool]` (100.120.224.10 to 100.120.255.254, the server is
  100.120.224.1).  For DNS, the internet and keeping callers off your LAN,
  `make install-dialup` ([the dial-up network](../docs/dialup-network.md)).
  Windows' Dial-Up Networking logs in
  with PAP instead of at the prompt: turn on `[autoppp]` and install
  pppd's `modemmenu.so` plugin (`make` builds it when `ppp-dev` is
  installed).
- **Last callers**, and **Change your password**, which rewrites the
  caller's line in the users file.
- **Bulletin board** (off until you add `bbs` to `[menu] items`):
  [D-Modem's BBS](https://github.com/D-Modem/bbs) on its unix socket,
  taking the caller as logged in.

Each call may last an hour and each user two hours a day
(`time_limit`, `daily_limit`); the menu prompt shows the minutes left.
To let callers make their own accounts, set `[signup] enabled = yes`:
"new" at the login prompt adds a locked line to the users file, which
you unlock by removing its `!`.

## When it does not work

- `journalctl -t modemmenu` shows each call, login and selection.
- "The lines are busy" or no menu: is `modemmenu.service` running, and is
  the socket's group `asterisk`?  Asterisk must be in that group (it is
  on FreePBX).
- No rate or caller ID on the screen: turn on **Call details first** in
  the profile.
- With dmodem-ari, the dialed number modemmenu sees is the profile's
  name (`menu`), not the DID, because FreePBX reaches it through the
  `app-dmodem` context.
