# modemmenu on FreePBX

This is a configuration for [FreePBX](https://www.freepbx.org/) 16 and
17 that is ready to use. Callers dial in, and their modem connects to
[D-Modem](https://github.com/D-Modem/D-Modem). Then modemmenu asks for a
user name and a password and checks them against a text file. Then it
shows its menu.

| file | goes to | |
|---|---|---|
| [modemmenu.conf](modemmenu.conf) | `/etc/modemmenu/modemmenu.conf` | the login and the menu |
| [modemmenu.users](modemmenu.users) | `/etc/modemmenu/modemmenu.users` | the names and passwords of the callers |
| [modemmenu.service](modemmenu.service) | `/etc/systemd/system/modemmenu.service` | runs modemmenu for all calls |
| [extensions_custom.conf](extensions_custom.conf) | add to `/etc/asterisk/extensions_custom.conf` | an optional test extension |

The files are in `/etc/modemmenu`, not in `/etc/asterisk`. Thus the
`fwconsole chown` of FreePBX never gives the asterisk user access to the
users file.

## How a call goes

```
caller's modem ──SIP──▶ FreePBX inbound route ──▶ D-Modem: menu
                                                   (dmodem-ari or app_dmodem)
                                                         │ unix socket, call details first
                                                         ▼
                                    modemmenu -serve ──▶ login from modemmenu.users
                                                    ──▶ menu: PPP, last callers, password
```

One `modemmenu -serve` answers all calls on
`/run/modemmenu/modemmenu.sock`. The socket belongs to the asterisk
group. Thus Asterisk and dmodem-ari can connect, and other users cannot.
Each call gets its own session.

## Setting it up

Before you start, install the FreePBX module of D-Modem, with
dmodem-ari or app_dmodem. Refer to
[docs/freepbx.md](https://github.com/D-Modem/D-Modem/blob/main/docs/freepbx.md)
of D-Modem.

1. **Build modemmenu.** It must have Go 1.22 or later. Debian 12
   (FreePBX 17) has Go 1.19. Thus, build modemmenu on a different
   machine, or use bookworm-backports. This configuration does not use
   PAM. Thus you can copy a static build to any x86-64 Linux:

       CGO_ENABLED=0 make PAM=0
       scp modemmenu root@freepbx:/usr/local/bin/

   Alternatively, on the FreePBX machine, run
   `make PAM=0 install-freepbx`. This command also does steps 2 and 3.
   It does not overwrite files that are already there.

2. **Install the files** (as root):

       install -d -m 755 /etc/modemmenu
       install -m 644 freepbx/modemmenu.conf    /etc/modemmenu/
       install -m 600 freepbx/modemmenu.users   /etc/modemmenu/
       install -m 644 freepbx/modemmenu.service /etc/systemd/system/

3. **Add the callers.** `modemmenu -hash` asks for a password and
   prints its bcrypt hash. Each caller is a `name:hash` line in
   `/etc/modemmenu/modemmenu.users`:

       echo "alice:$(modemmenu -hash)" >> /etc/modemmenu/modemmenu.users

   For a first test, you can use `test:{PLAIN}changeme`. Remove it
   after the test. modemmenu reads the file at each login. Thus you do
   not have to restart after a change.

4. **Check and start modemmenu:**

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

   Keep the default values for the other fields. This step writes a
   `[menu]` profile with `mode = unix` and `header = yes` into
   `/etc/asterisk/dmodem.conf`. It also adds the destination
   `app-dmodem,menu,1`.

6. **Send calls to the profile.** In **Connectivity > Inbound Routes**,
   select **D-Modem: menu** as the destination of the DID of the modem
   line. Click **Apply Config**. If you use dmodem-ari, restart it. Then
   it reads the new profile.

7. **Test the configuration** (optional). Add the lines of
   [extensions_custom.conf](extensions_custom.conf) to
   `/etc/asterisk/extensions_custom.conf`. Then run `fwconsole reload`.
   A modem on one of your extensions (an ATA) can then dial `6368`. The
   call gets to the menu and does not go through the trunk.

Limit the trunk and the extension to one codec (ulaw or alaw), and
disable fax detection. The line check page of D-Modem lists the
remaining problems.

## The menu

- **PPP**: dial-up networking. First, run `apt install ppp`. Each
  caller gets an address from `[ppp_pool]` (100.120.224.10 to
  100.120.255.254). The server is 100.120.224.1. For DNS, for the
  internet and to keep callers away from your LAN, run
  `make install-dialup` ([the dial-up network](../docs/dialup-network.md)).
  Dial-Up Networking of Windows logs in with PAP, not at the prompt. For
  Windows, enable `[autoppp]` and install the `modemmenu.so` plugin of
  pppd. If `ppp-dev` is installed, `make` builds the plugin.
- **Last callers**, and **Change your password**. **Change your
  password** writes the line of the caller in the users file again.
- **Bulletin board** (disabled until you add `bbs` to `[menu] items`):
  [D-Modem's BBS](https://github.com/D-Modem/bbs) on its unix socket.
  The BBS accepts the caller as logged in.

Each call can continue for a maximum of one hour, and each user for a
maximum of two hours each day (`time_limit`, `daily_limit`). The menu
prompt shows the minutes that remain. To let callers make their own
accounts, set `[signup] enabled = yes`. Then "new" at the login prompt
adds a locked line to the users file. To unlock the line, remove its
`!`.

## When it does not work

- `journalctl -t modemmenu` shows each call, login and selection.
- If you see "The lines are busy", or if there is no menu: make sure
  that `modemmenu.service` runs, and that the group of the socket is
  `asterisk`. Asterisk must be in that group (on FreePBX, it is).
- If the screen shows no rate or caller ID: enable **Call details
  first** in the profile.
- With dmodem-ari, the dialed number that modemmenu sees is the name of
  the profile (`menu`), not the DID. The cause is that FreePBX gets to
  it through the `app-dmodem` context.
