# modemmenu: the login and menu of a dial-up line (Go 1.22 or later)
#
#   make                 build
#   make test            the tests
#   make install         into $(BINDIR), samples into $(CONF_DIR)

GO ?= go
BINDIR ?= /usr/local/bin
CONF_DIR ?= /etc/asterisk

all: modemmenu

modemmenu: FORCE
	$(GO) build -o $@ .

test:
	$(GO) vet ./...
	$(GO) test ./...

install: modemmenu
	install -D -m 755 modemmenu $(DESTDIR)$(BINDIR)/modemmenu
	[ -e $(DESTDIR)$(CONF_DIR)/modemmenu.conf ] || \
		install -D -m 640 modemmenu.conf.sample $(DESTDIR)$(CONF_DIR)/modemmenu.conf
	[ -e $(DESTDIR)$(CONF_DIR)/modemmenu.users ] || \
		install -D -m 600 modemmenu.users.sample $(DESTDIR)$(CONF_DIR)/modemmenu.users

clean:
	rm -f modemmenu

FORCE:
.PHONY: all test install clean FORCE
