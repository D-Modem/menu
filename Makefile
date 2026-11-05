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
	$(GO) build -buildvcs=false $(TAGS) -o $@ .

test:
	$(GO) vet $(TAGS) ./...
	$(GO) test $(TAGS) ./...

install: all
	install -D -m 755 modemmenu $(DESTDIR)$(BINDIR)/modemmenu
	[ -e $(DESTDIR)$(CONF_DIR)/modemmenu.conf ] || \
		install -D -m 640 modemmenu.conf.sample $(DESTDIR)$(CONF_DIR)/modemmenu.conf
	[ -e $(DESTDIR)$(CONF_DIR)/modemmenu.users ] || \
		install -D -m 600 modemmenu.users.sample $(DESTDIR)$(CONF_DIR)/modemmenu.users
	[ $(PAM) != 1 ] || [ -e $(DESTDIR)/etc/pam.d/modemmenu ] || \
		install -D -m 644 modemmenu.pam.sample $(DESTDIR)/etc/pam.d/modemmenu
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
.PHONY: all test install install-freepbx clean FORCE
