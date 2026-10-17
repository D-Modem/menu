/*
 * Copyright (C) 2026 Neo-Desktop
 * SPDX-License-Identifier: AGPL-3.0-only
 *
 * modemmenu.so - a pppd plugin: the peer's PAP login checked by
 * modemmenu, on the auth socket of the call (modemmenu's AutoPPP runs
 * pppd with it in MODEMMENU_AUTH_SOCKET, or the modemmenu-socket option
 * names it).  One JSON line each way:
 *
 *   {"user": "alice", "password": "...", "service": "ppp"}
 *   {"ok": true, "auth": "file", "seconds_left": 3540}
 *
 * The time left becomes pppd's maxconnect.  PAP only: CHAP would need
 * the passwords kept where they can be read back.
 *
 *   plugin modemmenu.so require-pap refuse-chap refuse-mschap refuse-mschap-v2
 */
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <sys/un.h>
#include <unistd.h>

#include <pppd/pppd.h>

char pppd_version[] = VERSION;

static char *sock_path;
static int logged_in;

static option_t options[] = {
	{ "modemmenu-socket", o_string, &sock_path,
	  "modemmenu's auth socket (default: $MODEMMENU_AUTH_SOCKET)" },
	{ NULL }
};

/* the socket's path: the option, else modemmenu's environment */
static const char *path(void)
{
	return sock_path && *sock_path ? sock_path : getenv("MODEMMENU_AUTH_SOCKET");
}

/* PAP logins come here when there is a socket */
static int check(void)
{
	return path() != NULL;
}

/* s as a JSON string into out (size n); -1 if it does not fit */
static int json_string(char *out, size_t n, const char *s)
{
	size_t o = 0;
	if (o + 1 >= n)
		return -1;
	out[o++] = '"';
	for (; *s; s++) {
		unsigned char c = *s;
		char esc[8];
		const char *add = esc;
		if (c == '"' || c == '\\')
			snprintf(esc, sizeof(esc), "\\%c", c);
		else if (c < 0x20)
			snprintf(esc, sizeof(esc), "\\u%04x", c);
		else {
			esc[0] = c;
			esc[1] = 0;
		}
		if (o + strlen(add) + 2 >= n)
			return -1;
		memcpy(out + o, add, strlen(add));
		o += strlen(add);
	}
	out[o++] = '"';
	out[o] = 0;
	return 0;
}

/* the request to the socket, its reply line into reply; -1 on failure */
static int ask(const char *user, const char *passwd, char *reply, size_t n)
{
	struct sockaddr_un sun = { .sun_family = AF_UNIX };
	struct timeval tv = { .tv_sec = 60 };
	char u[1024], p[1024], req[2200];
	size_t got = 0;
	int fd, len;

	if (strlen(path()) >= sizeof(sun.sun_path) ||
	    json_string(u, sizeof(u), user) || json_string(p, sizeof(p), passwd))
		return -1;
	strcpy(sun.sun_path, path());
	len = snprintf(req, sizeof(req),
		       "{\"user\":%s,\"password\":%s,\"service\":\"ppp\"}\n", u, p);
	fd = socket(AF_UNIX, SOCK_STREAM, 0);
	if (fd < 0)
		return -1;
	setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));
	if (connect(fd, (struct sockaddr *)&sun, sizeof(sun)) ||
	    write(fd, req, len) != len) {
		warn("modemmenu: %s: %m", path());
		close(fd);
		return -1;
	}
	while (got + 1 < n) {
		ssize_t r = read(fd, reply + got, n - 1 - got);
		if (r <= 0)
			break;
		got += r;
		if (memchr(reply, '\n', got))
			break;
	}
	close(fd);
	reply[got] = 0;
	return got ? 0 : -1;
}

/* the PAP login: 1 right, 0 wrong (no fallback to pap-secrets) */
static int auth(char *user, char *passwd, char **msgp,
		struct wordlist **paddrs, struct wordlist **popts)
{
	char reply[512], *at;

	if (ask(user, passwd, reply, sizeof(reply))) {
		warn("modemmenu: no answer for %s", user);
		return 0;
	}
	if (!strstr(reply, "\"ok\":true")) {
		notice("modemmenu: login %s refused", user);
		return 0;
	}
	at = strstr(reply, "\"seconds_left\":");
	if (at) {
		long secs = strtol(at + strlen("\"seconds_left\":"), NULL, 10);
		/* the call's time limit: the link ends with it */
		if (secs >= 0) {
			maxconnect = secs > 0 ? (int)secs : 1;
			info("modemmenu: %s has %ld s", user, secs);
		}
	}
	logged_in = 1;
	return 1;
}

/* the peer's address, once logged in: as pppd's options give it (put
   local:remote on pppd's command line, so that the peer cannot pick
   one) */
static int address(u_int32_t addr)
{
	return logged_in ? 1 : -1;
}

void plugin_init(void)
{
	add_options(options);
	pap_check_hook = check;
	pap_auth_hook = auth;
	allowed_address_hook = address;
	info("modemmenu: PAP logins checked by modemmenu");
}
