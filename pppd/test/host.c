/*
 * Copyright (C) 2026 Neo-Desktop
 * SPDX-License-Identifier: AGPL-3.0-only
 *
 * host.c - pppd's side of the plugin interface, for testing modemmenu.so
 * where pppd cannot run (no PPP in the kernel): it exports what pppd
 * exports to plugins (the hooks, maxconnect, add_options and the log
 * functions, unversioned, as pppd does), loads the plugin and logs in
 * with PAP as pppd would.
 *
 *   host modemmenu.so user password [socket]
 *
 * prints "auth=N maxconnect=N address=N" (and the plugin's log lines).
 */
#include <dlfcn.h>
#include <stdarg.h>
#include <stdio.h>
#include <string.h>

#include <pppd/pppd.h>

int maxconnect;
int (*pap_check_hook)(void);
int (*pap_auth_hook)(char *, char *, char **, struct wordlist **, struct wordlist **);
int (*allowed_address_hook)(u_int32_t);
static option_t *opts;

/* pppd's add_options: the plugin's options kept */
void add_options(option_t *o) { opts = o; }

/* a log line of the plugin's, printed with its level */
static void logline(const char *level, const char *fmt, va_list ap)
{
	char buf[1024];
	vsnprintf(buf, sizeof(buf), fmt, ap);
	printf("LOG %s: %s\n", level, buf);
}
/* pppd's log functions (info, notice, warn, error, dbglog), each printing
   through logline */
#define LOGFN(name) void name(char *fmt, ...) \
	{ va_list ap; va_start(ap, fmt); logline(#name, fmt, ap); va_end(ap); }
LOGFN(info)
LOGFN(notice)
LOGFN(warn)
LOGFN(error)
LOGFN(dbglog)

/* the plugin loaded and initialized, one PAP login, the result printed */
int main(int argc, char **argv)
{
	void *h;
	char *version, *msg = NULL;
	void (*init)(void);
	int r;

	if (argc < 4) {
		fprintf(stderr, "host modemmenu.so user password [socket]\n");
		return 2;
	}
	h = dlopen(argv[1], RTLD_NOW | RTLD_GLOBAL);
	if (!h) {
		fprintf(stderr, "%s\n", dlerror());
		return 2;
	}
	version = dlsym(h, "pppd_version");
	init = (void (*)(void))dlsym(h, "plugin_init");
	if (!version || !init || strcmp(version, VERSION)) {
		fprintf(stderr, "not a plugin for pppd %s\n", VERSION);
		return 2;
	}
	init();
	if (argc > 4) {
		/* the modemmenu-socket option, as pppd sets an o_string */
		for (option_t *o = opts; o && o->name; o++)
			if (!strcmp(o->name, "modemmenu-socket"))
				*(char **)o->addr = argv[4];
	}
	if (!pap_check_hook || !pap_check_hook()) {
		printf("no PAP\n");
		return 1;
	}
	printf("address before=%d\n", allowed_address_hook(0x0a000002));
	r = pap_auth_hook(argv[2], argv[3], &msg, NULL, NULL);
	printf("auth=%d maxconnect=%d address=%d\n", r, maxconnect,
	       allowed_address_hook(0x0a000002));
	return 0;
}
