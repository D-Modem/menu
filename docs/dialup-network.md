# The dial-up network

PPP callers get these items:

- Their own address from the `[ppp_pool]` of modemmenu.
- A DNS server that answers for the services of this system.
- Their own network. From this network, they can get to the services of
  the host and to the internet. They cannot get to the LANs of the host,
  unless the sysop opens a range for them.

The files are in [`dialup/`](../dialup). `make install-dialup` installs
them.

```
caller ── modem ── D-Modem ── modemmenu ── pppd ── ppp0 ┐
                                                        │  VRF "dialup" (table 77)
               100.120.224.1: DNS, BBS, front ends ─────┤  100.120.224.0/19
                                                        └─ egress: eth0.77 (VLAN), eth0 (NAT) or none
```

## Addresses: `[ppp_pool]`

```
[ppp_pool]
local = 100.120.224.1
range = 100.120.224.10-100.120.255.254
;dns = 100.120.224.1
```

A PPP command can contain `{ppp_remote}` (or `{ppp_local}`,
`{ppp_dns}`, `{ppp_dns2}`). For such a command, modemmenu takes the
first free address of `range` before pppd starts. It releases the
address when pppd ends:

```
command = exec /usr/sbin/pppd nodetach noauth local lock passive {ppp_local}:{ppp_remote} ms-dns {ppp_dns} ms-dns {ppp_dns2} ipparam {user}
```

- Each address is a lock file in `state_dir/ppp-pool`. modemmenu holds
  the lock while pppd runs. The file contains the name of the user and
  the pid of modemmenu. A modemmenu for each call and all sessions of
  `modemmenu -serve` share these files. If its modemmenu stops
  unexpectedly, the address becomes free automatically.
- PPP gives the addresses (IPCP), not DHCP. `local` is the server end of
  all links. `dns` is the value that `ms-dns` offers (by default,
  `local`). `{ppp_dns2}` is the second server, or the first server
  again. Thus the command always has two servers.
- The scripts of pppd see `MODEM_PPP_LOCAL`, `MODEM_PPP_REMOTE`,
  `MODEM_PPP_DNS` and `MODEM_PPP_DNS2`.
- If the pool is full, modemmenu refuses the call ("all N PPP addresses
  are in use").
- The `[ppp]` profile of D-Modem runs pppd without modemmenu, thus it
  has no pool. Give it a fixed pair of addresses outside `range`
  (100.120.224.2 to .9).

Select a subnet that no network of the host uses. The subnet in the
sample is 100.120.224.0/19 (8,190 addresses). It is in 100.64.0.0/10,
the shared address space of RFC 6598 (carrier-grade NAT). Home and
office LANs do not use this space, thus it does not conflict with them.
Tailscale also gives addresses in 100.64.0.0/10. On a host that runs
Tailscale, use `tailscale status` to look for a conflict, or select a
different range.

## Isolation: `modemmenu-dialup`

`modemmenu-dialup up` reads `/etc/modemmenu/dialup.conf` and configures
the items that follow. `modemmenu-dialup.service` runs it at boot.

- **A VRF**, `dialup`, with routing table 77. The links of the callers
  are in the VRF. Thus the routes of the host, and its LANs, are not
  available to the callers. If a lookup finds no route in the VRF, the
  destination is unreachable. The lookup never continues to the table of
  the host.
- **`dialup0`**, a dummy interface that holds `SERVER_ADDR`
  (100.120.224.1). Thus the DNS server has its address when no caller is
  connected.
- **An egress** (`EGRESS`):
  - `vlan`: `UPLINK.VLAN_ID` (`eth0.77`), in the VRF, with its own
    address and gateway. Dial-up traffic goes out on its own VLAN and
    never shares the LAN of the host. Set `NAT=no` if the router on that
    VLAN itself routes 100.120.224.0/19 back.
  - `uplink`: out through the interface of the host and its gateway,
    with masquerade. This is simpler, and it has the same filters as the
    VLAN.
  - `none`: no internet. Only the services of the host are available.
- **A firewall** (nftables table `modemmenu_dialup`) on the links of the
  callers:
  - To the host: only `HOST_TCP_PORTS` and `HOST_UDP_PORTS` on
    `SERVER_ADDR`, and ping. By default, the ports are DNS, telnet,
    HTTP(S), MSN 1863, Yahoo 5050, OSCAR and AOL 5190. SSH, the web UI of
    the PBX and all other ports of the host are closed to the callers.
  - Callers are isolated from each other. The firewall drops all traffic
    from one caller to `SUBNET` (a different caller). It does this before
    it examines `ALLOW_CIDRS`, and `ALLOW_CIDRS` cannot change it. The
    callers can still meet in the services of the host (the BBS, the
    messengers) on 100.120.224.1.
  - Onward: the firewall drops `BLOCK_CIDRS` (by default, all private
    ranges, link-local and multicast). Thus the callers cannot get to the
    LANs of the host. The exception is `ALLOW_CIDRS`, which the sysop
    opens:

    ```
    ALLOW_CIDRS="192.168.1.20/32"     # one machine on the LAN
    ALLOW_CIDRS="192.168.1.0/24"      # a whole LAN
    ```

    All other destinations (the internet) are open while
    `INTERNET=yes`.
  - The firewall drops a source address outside `SUBNET`. It also drops
    IPv6.
- `net.ipv4.ip_forward=1`, and `tcp_l3mdev_accept`/`udp_l3mdev_accept`.
  Thus the services of the host (which listen in no VRF) answer callers
  on 100.120.224.1.

The `ip-up.d/modemmenu-dialup` script of pppd moves a link into the VRF
if the local address of the link is `SERVER_ADDR`. Before it moves the
link, it adds the link to the list of the firewall. The `ip-down.d`
script removes the name of the link from the list again. The script does
not change other links of pppd. After you edit `dialup.conf`, run
`systemctl restart modemmenu-dialup modemmenu-dns`.
`modemmenu-dialup status` shows the VRF, its routes and the links of the
callers.

## DNS: `modemmenu-dns`

`modemmenu-dns.service` runs dnsmasq in the VRF, on 100.120.224.1 only,
with [`dnsmasq.conf`](../dialup/dnsmasq.conf). This configuration has:

- No DHCP.
- Its own upstream servers. The `127.0.0.53` of the host is not in the
  VRF.
- The names of this system under `.dialup` (`bbs.dialup`).

**Overrides** are all `*.conf` files in `/etc/modemmenu/dns.d/`.
[`services.conf`](../dialup/dns.d/services.conf) has the names of the
old services. Each name has a comment mark. Remove the mark when the
front end of the service is available. The names are for AIM and ICQ
(`login.oscar.aol.com`), AOL (`americaonline.aol.com`), MSN Messenger
(`messenger.hotmail.com`) and Yahoo! Messenger (`scs.msg.yahoo.com` and
other names). Alternatively, a front end can add its own file:

```
# /etc/modemmenu/dns.d/oscar.conf
address=/login.oscar.aol.com/100.120.224.1
```

`address=/name/ip` answers for the name and for all names under it.
Then run `systemctl reload modemmenu-dns`. The clients of the callers
connect to our front ends without a change to a hosts file or to the
registry.

## Install

```
apt install ppp dnsmasq-base nftables iproute2
make install-dialup      # the script, hooks, units, /etc/modemmenu/{dialup.conf,dnsmasq.conf,dns.d/}
$EDITOR /etc/modemmenu/dialup.conf
systemctl enable --now modemmenu-dialup modemmenu-dns
```

Then add the `[ppp_pool]` above to modemmenu.conf (the samples have
it). Also add the `{ppp_...}` placeholders to the PPP commands.

## Limits

- IPv4 only. The firewall drops IPv6 from callers.
- The VRF must have a kernel with `CONFIG_NET_VRF` (the kernels of Debian
  and Ubuntu have it). `ip vrf exec` must have cgroup v2.
- The `listen-address` and `address=` lines of `dnsmasq.conf`, and the
  VRF name in the unit, are for the defaults. If you change `SERVER_ADDR`
  or `VRF`, change them too.
