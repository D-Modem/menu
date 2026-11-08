# modemmenu: the login and menu of a dial-up line (Go 1.22 or later)
#
#   make                 build, with PAM when libpam's headers are there,
#                        and the pppd plugin when pppd's are (ppp-dev)
#   make PAM=0           without PAM (PAM=1: with, or fail)
#   make PPPD=0          without the pppd plugin (PPPD=1: with, or fail)
#   make test            the tests
#   make install         into $(BINDIR), samples into $(CONF_DIR), the
#                        plugin into pppd's plugin directory
#   make install-freepbx into $(BINDIR), freepbx/'s samples into
#                        /etc/modemmenu and modemmenu.service into systemd
#   make install-dialup  the PPP callers' network (docs/dialup-network.md):
#                        modemmenu-dialup, pppd's hooks, the units, and
#                        dialup.conf, dnsmasq.conf, dns.d/ in /etc/modemmenu

GO ?= go
BINDIR ?= /usr/local/bin
CONF_DIR ?= /etc/asterisk
PAM ?= $(shell [ -e /usr/include/security/pam_appl.h ] && echo 1 || echo 0)
TAGS := $(if $(filter 1,$(PAM)),-tags pam)
CC ?= cc
CFLAGS ?= -O2 -g
PPPD ?= $(shell [ -e /usr/include/pppd/pppd.h ] && echo 1 || echo 0)
PPPD_VERSION ?= $(shell sed -n 's/^\#define VERSION[^"]*"\(.*\)"/\1/p' /usr/include/pppd/patchlevel.h 2>/dev/null)
PPPD_PLUGIN_DIR ?= /usr/lib/pppd/$(PPPD_VERSION)

all: modemmenu $(if $(filter 1,$(PPPD)),pppd/modemmenu.so)

# AutoPPP's PAP logins: pppd's plugin, for the pppd of these headers
pppd/modemmenu.so: pppd/modemmenu.c
	$(CC) $(CFLAGS) -Wall -fPIC -shared -o $@ $<

# -buildvcs=false: nothing reads the VCS stamp, and git refuses a checkout
# owned by another user (sudo make in your own clone), which fails the build
modemmenu: FORCE
	$(GO) build -buildvcs=false $(TAGS) -o $@ ./cmd/modemmenu

test:
	$(GO) vet $(TAGS) ./...
	$(GO) test $(TAGS) ./...

install: all
	install -D -m 755 modemmenu $(DESTDIR)$(BINDIR)/modemmenu
	[ -e $(DESTDIR)$(CONF_DIR)/modemmenu.conf ] || \
		install -D -m 640 samples/modemmenu.conf.sample $(DESTDIR)$(CONF_DIR)/modemmenu.conf
	[ -e $(DESTDIR)$(CONF_DIR)/modemmenu.users ] || \
		install -D -m 600 samples/modemmenu.users.sample $(DESTDIR)$(CONF_DIR)/modemmenu.users
	[ $(PAM) != 1 ] || [ -e $(DESTDIR)/etc/pam.d/modemmenu ] || \
		install -D -m 644 samples/modemmenu.pam.sample $(DESTDIR)/etc/pam.d/modemmenu
	[ $(PPPD) != 1 ] || \
		install -D -m 644 pppd/modemmenu.so $(DESTDIR)$(PPPD_PLUGIN_DIR)/modemmenu.so

# FreePBX (freepbx/README.md): the files in /etc/modemmenu, out of
# fwconsole chown's way; files already there are kept
FREEPBX_CONF_DIR ?= /etc/modemmenu
install-freepbx: all
	install -D -m 755 modemmenu $(DESTDIR)$(BINDIR)/modemmenu
	[ -e $(DESTDIR)$(FREEPBX_CONF_DIR)/modemmenu.conf ] || \
		install -D -m 644 freepbx/modemmenu.conf $(DESTDIR)$(FREEPBX_CONF_DIR)/modemmenu.conf
	[ -e $(DESTDIR)$(FREEPBX_CONF_DIR)/modemmenu.users ] || \
		install -D -m 600 freepbx/modemmenu.users $(DESTDIR)$(FREEPBX_CONF_DIR)/modemmenu.users
	[ -e $(DESTDIR)/etc/systemd/system/modemmenu.service ] || \
		install -D -m 644 freepbx/modemmenu.service $(DESTDIR)/etc/systemd/system/modemmenu.service
	[ $(PPPD) != 1 ] || \
		install -D -m 644 pppd/modemmenu.so $(DESTDIR)$(PPPD_PLUGIN_DIR)/modemmenu.so

clean:
	rm -f modemmenu pppd/modemmenu.so

FORCE:
.PHONY: all test install install-freepbx install-dialup clean FORCE

# the PPP callers' network: files already in /etc/modemmenu are kept
SBINDIR ?= /usr/local/sbin
install-dialup:
	install -D -m 755 dialup/modemmenu-dialup $(DESTDIR)$(SBINDIR)/modemmenu-dialup
	install -D -m 755 dialup/ip-up $(DESTDIR)/etc/ppp/ip-up.d/modemmenu-dialup
	install -D -m 755 dialup/ip-down $(DESTDIR)/etc/ppp/ip-down.d/modemmenu-dialup
	install -D -m 644 dialup/modemmenu-dialup.service $(DESTDIR)/etc/systemd/system/modemmenu-dialup.service
	install -D -m 644 dialup/modemmenu-dns.service $(DESTDIR)/etc/systemd/system/modemmenu-dns.service
	[ -e $(DESTDIR)$(FREEPBX_CONF_DIR)/dialup.conf ] || \
		install -D -m 644 dialup/dialup.conf.sample $(DESTDIR)$(FREEPBX_CONF_DIR)/dialup.conf
	[ -e $(DESTDIR)$(FREEPBX_CONF_DIR)/dnsmasq.conf ] || \
		install -D -m 644 dialup/dnsmasq.conf $(DESTDIR)$(FREEPBX_CONF_DIR)/dnsmasq.conf
	[ -e $(DESTDIR)$(FREEPBX_CONF_DIR)/dns.d/services.conf ] || \
		install -D -m 644 dialup/dns.d/services.conf $(DESTDIR)$(FREEPBX_CONF_DIR)/dns.d/services.conf
