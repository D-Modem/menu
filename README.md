<img src=".github/banner.png" alt="D-Modem/menu: User login and management for dial-up callers" width="100%">

# modemmenu

modemmenu is a login and a menu for dial-up callers. When the modems
connect, the caller sees modemmenu first.

modemmenu asks for a user name and a password. Then it shows the message
of the day and a menu of the tasks that the caller can do. These tasks
include a bulletin board, PPP dial-up networking, a shell or any other
program. modemmenu runs behind `app_dmodem`, the Asterisk module of
[D-Modem](https://github.com/D-Modem/D-Modem). It can run as one process
for each call or as one server for all calls.

```
Welcome to D-Modem.

Connected at 33600 bit/s.

login: alice
Password: ******

Hello alice, on 2026-10-10 at 21:05:09.

The line is down for maintenance on Sunday.

Main menu

  [B] Bulletin board
  [P] PPP (dial-up networking)
  [S] Shell
  [G] Goodbye (hang up)

Selection: ppp
```

- Logins from a users file, LDAP, SQL (MySQL/MariaDB, PostgreSQL, SQLite) or system accounts through PAM.
- Detection of PPP dialers (AutoPPP and PAP), with a pppd plugin.
- Support for the classic AOL client when it dials in. modemmenu emulates AOLnet, SprintNet and Tymnet. Then it connects the line to the AOL host of a BBS.
- Menus with access by user and by group, guest access and self sign-up.
- A full-screen menu on ANSI and VT terminals, and a line menu on other terminals. The full-screen menu looks like a blue desktop from the 1990s. The cursor keys move the bar. For slow lines, modemmenu sends only the changes to the screen.
- `modemmenu -setup`: full screens with pull-down menus and the help text from the sample, to change the configuration. The sysop can use them at the console or on a call.
- Each item starts with the credentials of the caller and the details of the call: dialed number, caller ID, connect rate and time.
- Drop files for doors, a list of the last callers, and the transfer of a caller to a different menu.
- A FreePBX configuration that is ready to use ([freepbx/](freepbx/README.md)).

## Quick start

You must have Go 1.22 or later. Do these steps:

```
make                                                    # with PAM when libpam's headers are there
make install                                            # /usr/local/bin/modemmenu and the samples
modemmenu -config /etc/asterisk/modemmenu.conf -check   # check the configuration
modemmenu -config /etc/asterisk/modemmenu.conf -setup   # or change it on a full screen
```

Then configure an `app_dmodem` profile to use modemmenu ([docs/running.md](docs/running.md)).

## Documentation

The documentation is also a website, [d-modem.github.io/menu](https://d-modem.github.io/menu/). Some reference pages on the website come from the code.

[docs/](docs/README.md) has information about these topics: building,
running, configuration, authentication, PPP, security and the tests.
[samples/modemmenu.conf.sample](samples/modemmenu.conf.sample) has all the options.

## The D-Modem suite

| Repository | What it is |
| --- | --- |
| [D-Modem](https://github.com/D-Modem/D-Modem) | The open V.92 softmodem: dmodemd, libdmodem and the Asterisk, FreeSWITCH and ARI modules |
| [app](https://github.com/D-Modem/app) | The D-Modem app for Android and iOS: a dial-up modem on a SIP call |
| [hardware](https://github.com/D-Modem/hardware) | pico-serial: RP2040 serial adapters and phone-line USB sticks |
| [menu](https://github.com/D-Modem/menu) | modemmenu: the login and menu dial-up callers reach |
| [bbs](https://github.com/D-Modem/bbs) | A bulletin board for telnet, SSH, TN3270 and modem callers |
| [go3270](https://github.com/D-Modem/go3270) | A Go library for 3270 terminal servers, used by bbs |
| [brand](https://github.com/D-Modem/brand) | Logos, colours, type and the naming guide |

## Licence

AGPL-3.0-only ([LICENSE](LICENSE)), or a commercial licence from the D-Modem Authors ([COMMERCIAL-LICENSE.md](COMMERCIAL-LICENSE.md)). Contributions come with a contributor agreement ([CONTRIBUTING.md](CONTRIBUTING.md)).

If you change modemmenu and offer it to callers, section 13 of the AGPL
tells you to offer them its source code. To do this, give the location
of the source code in a menu item, in the banner or in the message of
the day.
