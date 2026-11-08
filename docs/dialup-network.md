# The dial-up network

PPP callers get an address of their own from modemmenu's `[ppp_pool]`,
a DNS server that answers for the services this system provides, and a
network of their own: they reach the host's services, the internet, and
none of the host's LANs unless the sysop opens a range for them.  The
files are in [`dialup/`](../dialup); `make install-dialup` installs them.

```
caller ── modem ── D-Modem ── modemmenu ── pppd ── ppp0 ┐
                                                        │  VRF "dialup" (table 77)
                       10.77.0.1: DNS, BBS, front ends ─┤  10.77.0.0/24
                                                        └─ egress: eth0.77 (VLAN), eth0 (NAT) or none
```

## Addresses: `[ppp_pool]`

```
[ppp_pool]
local = 10.77.0.1
range = 10.77.0.10-10.77.0.250
;dns = 10.77.0.1
```

A PPP command that names `{ppp_remote}` (or `{ppp_local}`, `{ppp_dns}`,
`{ppp_dns2}`) takes the first free address of `range` before pppd
starts, and gives it back when pppd ends:

```
command = exec /usr/sbin/pppd nodetach noauth local lock passive {ppp_local}:{ppp_remote} ms-dns {ppp_dns} ms-dns {ppp_dns2} ipparam {user}
```

- Each address is a lock file in `state_dir/ppp-pool`, held while pppd
  runs and holding the user's name and modemmenu's pid.  A modemmenu per
  call and every session of `modemmenu -serve` share them, and an
  address frees itself when its modemmenu dies.
- PPP gives the addresses (IPCP), not DHCP.  `local` is the server's end
  of every link; `dns` is what `ms-dns` offers (`local` by default).
  `{ppp_dns2}` is the second server, or the first again, so the command
  always has two.
- pppd's scripts see `MODEM_PPP_LOCAL`, `MODEM_PPP_REMOTE`,
  `MODEM_PPP_DNS` and `MODEM_PPP_DNS2`.
- A full pool refuses the call ("all N PPP addresses are in use").
- D-Modem's own `[ppp]` profile runs pppd without modemmenu, so it has
  no pool: give it a fixed pair outside `range` (10.77.0.2 to .9).

Pick a subnet none of the host's networks use.  10.77.0.0/24 is the
sample's.

## Isolation: `modemmenu-dialup`

`modemmenu-dialup up` (run at boot by `modemmenu-dialup.service`) sets
up, from `/etc/modemmenu/dialup.conf`:

- **A VRF**, `dialup`, with routing table 77.  The callers' links live in
  it, so the host's routes, and its LANs, are not theirs.  A lookup that
  finds nothing there is unreachable; it never falls through to the
  host's table.
- **`dialup0`**, a dummy interface holding `SERVER_ADDR` (10.77.0.1), so
  the DNS server has its address while no caller is on.
- **An egress** (`EGRESS`):
  - `vlan`: `UPLINK.VLAN_ID` (`eth0.77`), in the VRF, with its own
    address and gateway.  Dial-up traffic leaves on a VLAN of its own and
    never shares the host's LAN.  `NAT=no` when the router on that VLAN
    routes 10.77.0.0/24 back itself.
  - `uplink`: out of the host's interface through its gateway,
    masqueraded.  Simpler, and filtered like the VLAN.
  - `none`: no internet; the host's services only.
- **A firewall** (nftables table `modemmenu_dialup`) on the callers'
  links:
  - to the host: only `HOST_TCP_PORTS` and `HOST_UDP_PORTS` on
    `SERVER_ADDR` (DNS, telnet, HTTP(S), MSN 1863, Yahoo 5050, OSCAR and
    AOL 5190 by default), and ping.  SSH, the PBX's web UI and every other
    port of the host are closed to them.
  - callers are isolated from each other: anything from one caller to
    `SUBNET` (another caller) is dropped, before and whatever
    `ALLOW_CIDRS` says.  They still meet in the host's services (the BBS,
    the messengers) on 10.77.0.1.
  - onward: `BLOCK_CIDRS` are dropped (every private range, link-local
    and multicast, by default), so the host's LANs are out of reach,
    except `ALLOW_CIDRS`, which the sysop opens:

    ```
    ALLOW_CIDRS="192.168.1.20/32"     # one machine on the LAN
    ALLOW_CIDRS="192.168.1.0/24"      # a whole LAN
    ```

    The rest, the internet, is open while `INTERNET=yes`.
  - a source address outside `SUBNET` is dropped, and IPv6 too.
- `net.ipv4.ip_forward=1`, and `tcp_l3mdev_accept`/`udp_l3mdev_accept`,
  so the host's services (listening in no VRF) answer callers on
  10.77.0.1.

pppd's `ip-up.d/modemmenu-dialup` moves a link whose local address is
`SERVER_ADDR` into the VRF, putting it on the firewall's list first;
`ip-down.d` takes its name off again.  pppd's other links are left
alone.  After editing `dialup.conf`, `systemctl restart
modemmenu-dialup modemmenu-dns`.  `modemmenu-dialup status` shows the
VRF, its routes and the callers' links.

## DNS: `modemmenu-dns`

`modemmenu-dns.service` runs dnsmasq inside the VRF, on 10.77.0.1 only,
with [`dnsmasq.conf`](../dialup/dnsmasq.conf): no DHCP, upstream
servers of its own (the host's `127.0.0.53` is not in the VRF), and the
names of this system under `.dialup` (`bbs.dialup`).

**Overrides** are every `*.conf` in `/etc/modemmenu/dns.d/`.
[`services.conf`](../dialup/dns.d/services.conf) has the old services'
names ready to uncomment as their front ends come up: AIM and ICQ
(`login.oscar.aol.com`), AOL (`americaonline.aol.com`), MSN Messenger
(`messenger.hotmail.com`) and Yahoo! Messenger (`scs.msg.yahoo.com` and
the rest).  A front end can drop its own file instead:

```
# /etc/modemmenu/dns.d/oscar.conf
address=/login.oscar.aol.com/10.77.0.1
```

`address=/name/ip` answers the name and every name under it.  Then
`systemctl reload modemmenu-dns`.  Callers' clients connect to our front
ends with no hosts file or registry change.

## Install

```
apt install ppp dnsmasq-base nftables iproute2
make install-dialup      # the script, hooks, units, /etc/modemmenu/{dialup.conf,dnsmasq.conf,dns.d/}
$EDITOR /etc/modemmenu/dialup.conf
systemctl enable --now modemmenu-dialup modemmenu-dns
```

Then give modemmenu.conf the `[ppp_pool]` above (the samples have it)
and the PPP commands their `{ppp_...}` placeholders.

## Limits

- IPv4 only.  IPv6 from callers is dropped.
- The VRF needs a kernel with `CONFIG_NET_VRF` (Debian's and Ubuntu's
  have it) and `ip vrf exec` needs cgroup v2.
- `dnsmasq.conf`'s `listen-address`, `address=` lines and the unit's VRF
  name are written for the defaults: change them with `SERVER_ADDR` and
  `VRF`.
