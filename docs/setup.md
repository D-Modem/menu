# The sysop's setup

`modemmenu -setup` changes `modemmenu.conf` on a full screen, as the BBS
setup programs of the 1990s did: a menu bar of pull-down menus, a blue
desk, a window for each section cascading over the one it was opened
from, the help of the line under the bar in a box below, and the keys on
a bar at the foot.

![The full-screen menu and the setup](https://raw.githubusercontent.com/D-Modem/menu/main/.github/screens.png)

```
modemmenu -setup -config /etc/asterisk/modemmenu.conf
```

- The first window has every section, under Login, Users, Menu,
  Dial-up and Server (the same as the bar's menus).  Enter opens one: its
  keys and their values, `(not set)` and the sample's value for those the
  file leaves out.
- **Enter** changes the value under the bar: typed in a box (cursor keys,
  Home, End, Del, ^U), or picked from a list for a key the sample gives
  the choices of (`yes`/`no`, `petscii = auto|yes|no`...).  **Space** steps
  through the choices; **Del** takes the key out (its default again).  An
  empty box is "not set"; type `""` for an empty value.
- `[item NAME]` and `[number NAME]` are lists: add one, open it, rename
  it or delete it.  An item renamed or deleted is renamed or taken out of
  `[menu] items` too.  `[groups]` takes groups of the sysop's own names.
- **F1** shows the whole help of the line, **/** (or F12, or Alt and a
  letter) the menus, **Esc** goes back a window, **F10** saves, **F9**
  quits (asking to save what changed).  `^L` draws the screen again.
- The help of every key is the sample's ([reference](reference.md)), built
  into the program.

Saving checks the file first, as `modemmenu -check` does; a file that
does not check is saved only when asked again ("Save anyway").  The file
keeps its comments, blank lines and order: a changed key is changed on
its own line (a comment after it kept), a new key goes after the last of
its section, a new section at the end.  The file is written beside the
old one and renamed over it, its mode and owner kept.  Each call reads
the file anew, so the next call has the changes; `[server]` is read when
`modemmenu -serve` starts.

With no file yet, the setup starts from the sample and saves it there.

## On the line

A sysop who calls in can have the setup on the menu, as an item of
`mode = setup`:

```ini
[item setup]
key = Z
label = Setup
mode = setup
groups = sysops
```

It must have `users` or `groups` (modemmenu refuses one without), and is
never a guest's.  It needs an ANSI terminal of 40x14 at least (80x24 is
better); others are told so.  The idle timeout and the call's time limit
hold in it; what was not saved when the call ends is lost.  The file is
written as the user modemmenu runs as, which must be able to write it.

## Terminals

The screens are drawn in UTF-8 where the terminal shows it, in code page
437 for SyncTERM and the ANSI terminals of BBSs, in a VT's line-drawing
set (`ESC ( 0`) for a VT or xterm without UTF-8, and in ASCII when told
(`[terminal] charset`).  A DEC terminal itself (no colors) gets bold and
reverse for the colors.  Only the cells that change are sent: moving
the bar costs a hundred bytes or two.  At the console, `-setup` takes
UTF-8 from the locale (`LANG`), else the VT set for `TERM=vt*` or
`xterm*`, code page 437 for `TERM=linux` or `ansi`.

**See also:** [Configuration](configuration.md#the-full-screen-menu)
