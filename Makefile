# modemmenu: the login and menu of a dial-up line (Go 1.22 or later)
#
#   make                 build, with PAM when libpam's headers are there
#   make PAM=0           without PAM (PAM=1: with, or fail)
#   make test            the tests
#   make install         into $(BINDIR), samples into $(CONF_DIR)

GO ?= go
BINDIR ?= /usr/local/bin
CONF_DIR ?= /etc/asterisk
PAM ?= $(shell [ -e /usr/include/security/pam_appl.h ] && echo 1 || echo 0)
TAGS := $(if $(filter 1,$(PAM)),-tags pam)

all: modemmenu

modemmenu: FORCE
	$(GO) build $(TAGS) -o $@ .

test:
	$(GO) vet $(TAGS) ./...
	$(GO) test $(TAGS) ./...

install: modemmenu
	install -D -m 755 modemmenu $(DESTDIR)$(BINDIR)/modemmenu
	[ -e $(DESTDIR)$(CONF_DIR)/modemmenu.conf ] || \
		install -D -m 640 modemmenu.conf.sample $(DESTDIR)$(CONF_DIR)/modemmenu.conf
	[ -e $(DESTDIR)$(CONF_DIR)/modemmenu.users ] || \
		install -D -m 600 modemmenu.users.sample $(DESTDIR)$(CONF_DIR)/modemmenu.users
	[ $(PAM) != 1 ] || [ -e $(DESTDIR)/etc/pam.d/modemmenu ] || \
		install -D -m 644 modemmenu.pam.sample $(DESTDIR)/etc/pam.d/modemmenu

clean:
	rm -f modemmenu

FORCE:
.PHONY: all test install clean FORCE
