// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

// What each section and key of modemmenu.conf does, in a sentence or two
// for the sysop: the setup's help box shows it under the bar, and F1 adds
// the sample's own notes and the choices.  The sample's comments are
// written for someone reading the file; these say what the setting is
// for.  TestEverySettingDescribed keeps one for every section and key the
// sample has.

// setupDescs: "kind" for a section, "kind.key" for a key
var setupDescs = map[string]string{
	// [general]
	"general":                      "How callers log in: where modemmenu logs, which user databases it asks, what the caller sees before and after the menu, and how long a call may last.",
	"general.log":                  "Where modemmenu writes its log: syslog, a file's path, or none.  It never goes to the caller's line.",
	"general.auth":                 "Which user databases check a login, in the order they are asked.  A database that does not know the user passes to the next one; a wrong password stops at the first one that knows them.",
	"general.banner":               "The text every caller sees first, before the login prompt.",
	"general.banner_file":          "A file shown instead of the banner (it may hold ANSI colors).  Read at every call.",
	"general.login_prompt":         "The prompt that asks the caller for their user name.",
	"general.password_prompt":      "The prompt that asks the caller for their password.",
	"general.connect_message":      "Shown after the banner when the connect rate is known: {speed} is the rate.  \"\" shows nothing.",
	"general.welcome":              "Shown once a caller has logged in, before the message of the day.  {user}, {date} and the other placeholders work here.",
	"general.motd":                 "The message of the day: shown after the welcome, before the menu.",
	"general.motd_file":            "A file shown after motd as part of the message of the day, read at every login.",
	"general.goodbye":              "Shown at the end of the call, just before modemmenu hangs up.",
	"general.password_echo":        "What the caller sees for each password character they type: a character such as *, or none for nothing at all.",
	"general.attempts":             "How many wrong logins a caller gets before modemmenu hangs up.",
	"general.fail_delay":           "Seconds modemmenu waits after a wrong login before asking again, to slow down guessing.",
	"general.login_timeout":        "Seconds a caller has to log in before modemmenu hangs up.",
	"general.idle_timeout":         "Seconds the menu waits for a choice before it hangs up.",
	"general.user_pattern":         "A regular expression user names must match.  Names that do not are refused before any user database is asked.",
	"general.max_input":            "The longest line a caller may type at a prompt (a password may be twice as long).",
	"general.time_limit":           "The longest a single call may last after login (0: no limit).  Whatever is running is hung up when time runs out.",
	"general.daily_limit":          "The total time a user may spend on the line in a day, over all their calls (0: no limit).",
	"general.state_dir":            "The directory where modemmenu keeps what it remembers between calls: daily usage, the last callers, door nodes and PPP addresses.",
	"general.time_warning":         "When to warn the caller that their time is running out, as times before the limit (5m 1m).  0 turns the warnings off.",
	"general.time_warning_message": "The warning written on the line when time is running out.  {time_left} is in minutes, {seconds_left} in seconds; \\a beeps.",

	// [terminal]
	"terminal":         "How modemmenu works out what kind of terminal the caller has (ANSI, VT52, a Commodore...) before the banner, so the menu and doors can use it.",
	"terminal.detect":  "Whether to ask the caller's terminal what it is before the banner.  no: every caller is taken as a plain ANSI terminal.",
	"terminal.petscii": "Whether to recognise Commodore (PETSCII) callers: auto watches the login for a Commodore's keys, yes treats every caller as one, no never does.",
	"terminal.timeout": "How long to wait for each answer from the caller's terminal while detecting it.",
	"terminal.charset": "Which characters the full-screen menu draws its boxes and shading with.  auto picks by the terminal.",

	// [groups]
	"groups": "Your own groups of users, used by the menu items' groups setting to decide who sees an item.  Each line is a group's name and its members.",

	// [guest]
	"guest":            "A guest account: callers who log in with its name need no password, and only see the items marked for guests.",
	"guest.enabled":    "Turns the guest account on or off.",
	"guest.name":       "The user name a caller types to log in as a guest.",
	"guest.time_limit": "How long a guest may stay on (never more than the general time_limit).",

	// [signup]
	"signup":              "Lets callers create their own account from the login prompt.  New accounts are added to the users file.",
	"signup.enabled":      "Turns sign-up on or off.",
	"signup.name":         "The user name a caller types at the login prompt to sign up.",
	"signup.pending":      "yes: new accounts are locked until the sysop unlocks them in the users file.  no: they can log in at once.",
	"signup.min_password": "The shortest password a new account may have.",
	"signup.file":         "The users file new accounts are written to.  The [file] path by default.",
	"signup.message":      "Shown to a caller who starts signing up.",

	// [callerid]
	"callerid":              "Which phone numbers may call: an allow list, a deny list, and whether callers without caller ID get in.",
	"callerid.allow":        "If set, only these numbers may call (* and ? match like file names).",
	"callerid.deny":         "These numbers are refused (* and ? match like file names).",
	"callerid.anonymous":    "Whether callers who withhold their number (no caller ID) are let in.",
	"callerid.deny_message": "Shown to a caller who is refused by caller ID, before modemmenu hangs up.",

	// [number NAME]
	"number":      "Settings for one caller's phone number: log them in as a user without a password, refuse them, or start an item for them at once.",
	"number.user": "Logs this number in as this user without asking for a password.  Caller ID can be forged, so only where your phone network vouches for it.",
	"number.item": "The menu item started for this number right after login.",
	"number.deny": "yes refuses calls from this number.",

	// [file]
	"file":      "Users kept in a text file of user:password-hash lines.",
	"file.path": "The users file.",
	"file.hash": "How the passwords in the file are stored.  auto tells by each line's prefix; plain takes the rest of the line as the password.",

	// [ldap]
	"ldap":                      "Users checked against an LDAP directory, by logging in to it as the user.",
	"ldap.url":                  "The LDAP server: ldaps://host, or ldap://host:389.",
	"ldap.starttls":             "Whether to switch an ldap:// connection to TLS before sending passwords.",
	"ldap.ca_file":              "The certificate authority that signed the LDAP server's certificate, if it is not one the system trusts.",
	"ldap.insecure_skip_verify": "yes skips checking the server's certificate.  For testing only: anyone in between could read passwords.",
	"ldap.timeout":              "Seconds to wait for the LDAP server.",
	"ldap.bind_dn":              "The user's entry, with {user} in it, to log in as directly.  Leave it out to search for the entry instead.",
	"ldap.search_base":          "Where in the directory to search for the user's entry.",
	"ldap.search_filter":        "The filter that finds the user's entry ({user} is their name).  It can also restrict who may log in.",
	"ldap.search_bind_dn":       "The account the search runs as (empty: anonymously).",
	"ldap.search_bind_password": "The password of search_bind_dn.",
	"ldap.group_base":           "Where groups are looked for, so items' groups can name LDAP groups.  Not set: LDAP groups are not used.",
	"ldap.group_filter":         "The filter that finds a group the caller is a member of ({group}, {dn} and {user}).",
	"ldap.password_change":      "How the Change password item changes an LDAP password: exop for most servers, ad for Active Directory.",

	// [system]
	"system":             "Logins checked against this machine's own accounts through PAM (local users, sssd, Kerberos...).",
	"system.pam_service": "The PAM service name: the file in /etc/pam.d that says how to check the password.",
	"system.min_uid":     "Accounts with a lower user id (system accounts) may not log in.",
	"system.allow_root":  "Whether root may log in.",
	"system.groups":      "If set, only members of these system groups may log in.",

	// [sql]
	"sql":              "Users checked against an SQL database (MySQL or MariaDB, PostgreSQL, SQLite).",
	"sql.driver":       "The kind of database.",
	"sql.dsn":          "How to connect to the database: its address, name and login, in the driver's own form.",
	"sql.query":        "The query that returns the user's stored password (or any row, with hash = none).",
	"sql.args":         "What fills the query's placeholders, in order: user, password.",
	"sql.hash":         "How the stored passwords are hashed.  none: the query checks the password itself and any row means yes.",
	"sql.timeout":      "Seconds to wait for the database.",
	"sql.update_query": "The query the Change password item runs.  Without it, SQL passwords cannot be changed from the menu.",
	"sql.update_args":  "What fills update_query's placeholders, in order: user, password, old_password.",
	"sql.update_hash":  "How a changed password is stored: bcrypt, or none for a query that hashes it itself.",

	// [server]
	"server":                "modemmenu -serve: one modemmenu answering every call on a socket, instead of one started for each call.",
	"server.listen":         "Where -serve listens: a unix socket's path, or tcp:host:port.",
	"server.socket_mode":    "The unix socket's permissions (octal): who may connect to it.",
	"server.socket_group":   "The group that owns the unix socket (Asterisk's, for app_dmodem).",
	"server.header":         "Whether each connection starts with the call's details (caller ID, rate...).  app_dmodem and modemmenu items with header = yes send them.",
	"server.header_timeout": "Seconds to wait for those details on a new connection.",
	"server.trust_user":     "yes: a caller already logged in by another modemmenu is not asked to log in again.  Only for a socket that nothing else can reach.",
	"server.max_sessions":   "How many calls -serve takes at once; more callers are told the lines are busy.",

	// [authd]
	"authd":              "The auth socket: lets the BBS's other logins (telnet, SSH, the web) check passwords against the same user databases as the dial-up login.",
	"authd.listen":       "The auth socket's path.",
	"authd.socket_mode":  "The socket's permissions (octal): who may connect to it.",
	"authd.socket_group": "The group that owns the socket.",
	"authd.allow_users":  "The system users whose programs may ask the socket (the BBS's user).",
	"authd.max_clients":  "How many requests the socket answers at once.",

	// [ppp_pool]
	"ppp_pool":       "The addresses PPP (dial-up networking) callers get, so several lines can run PPP at once, each with its own.",
	"ppp_pool.local": "This server's own address on every PPP link.",
	"ppp_pool.range": "The addresses handed out to callers, first-last.  Pick a subnet none of the host's networks use.",
	"ppp_pool.dns":   "The DNS servers offered to PPP callers (one or two).  This server's PPP address by default.",

	// [autoppp]
	"autoppp":         "Starts PPP straight away for dialers that send PPP instead of logging in (Windows Dial-Up Networking and most others).  The login is checked by PAP.",
	"autoppp.enabled": "Turns AutoPPP on or off.",
	"autoppp.command": "The command run on the line for such a caller: pppd with the modemmenu.so plugin.",
	"autoppp.item":    "The menu item whose access rules (groups, minimum speed, caller IDs...) a PPP caller must pass.",

	// [aol]
	"aol":                 "Lets the classic AOL client (2.5 to 4.0) dial in: modemmenu plays the network it expects, then connects it to the BBS's AOL front end.",
	"aol.enabled":         "Turns AOL dial-in on or off.",
	"aol.host":            "The BBS's AOL front end, host:port.",
	"aol.networks":        "Which networks modemmenu pretends to be for the AOL client.",
	"aol.aolnet_prompt":   "The greeting AOLnet's script waits for.  Every caller sees it before the login prompt.",
	"aol.sprintnet_speed": "The answer to SprintNet's speed question.  11:14 makes the client use its high-speed settings.",

	// [dropfile]
	"dropfile":                  "The door drop files (DOOR32.SYS, DOOR.SYS, DORINFO) written for door games: what they say about the BBS and how the door reaches the caller.",
	"dropfile.bbs_name":         "The BBS's name, as doors show it.",
	"dropfile.sysop":            "The sysop's name, as doors show it.",
	"dropfile.security":         "The security level callers get in the drop files (an item's own security overrides it).",
	"dropfile.emulation":        "What the drop files say the caller's terminal is: auto by the terminal detected, or always ansi or ascii.",
	"dropfile.comm_type":        "How the door talks to the caller, in DOOR32.SYS: 0 on its own input and output (the usual), 1 serial, 2 telnet.",
	"dropfile.comm_handle":      "The handle DOOR32.SYS gives with comm_type (-1 with 0).",
	"dropfile.com_port":         "The COM port DOOR.SYS and DORINFO name (COM0: is local).  Under DOSEMU, the one DOSEMU gives the door.",
	"dropfile.fossil":           "Whether the drop files say a FOSSIL driver is there (DOS doors under DOSEMU).",
	"dropfile.rate":             "The speed the drop files give: a fixed rate doors accept, or connect for the modem's real rate.",
	"dropfile.four_digit_years": "Write dates with four-digit years in DOOR.SYS, for newer doors that want them.",
	"dropfile.max_nodes":        "How many doors may run at once, each on its own node (15 at most with dorinfo#.def).",

	// [menu]
	"menu":            "The menu callers see after they log in: its title, prompt, items, and the words that hang up.",
	"menu.title":      "The menu's title.",
	"menu.prompt":     "The prompt under the menu.  {time_left} shows the minutes left.",
	"menu.items":      "Which items the menu shows, in order.  Not set: every [item] in the file, in file order.",
	"menu.hangup":     "What the caller types to hang up; the first one is shown in the menu.",
	"menu.hotkeys":    "yes: one key picks an item at once, without Enter (every item's key must then be one character).",
	"menu.fullscreen": "auto: callers whose terminal can do it get the menu in a window, picked with the cursor keys.  no: everyone gets the menu as lines of text.",

	// [item NAME]
	"item":                    "A menu item: a program run on the line, a server the caller is connected to, or one of modemmenu's own (change password, last callers, setup).",
	"item.key":                "The key the caller types to pick this item (by default the first letter of its name).",
	"item.aliases":            "More words the caller can type to pick it (its name always works).",
	"item.label":              "The item's line in the menu.",
	"item.help":               "A line saying what the item is, shown under the full-screen menu.",
	"item.mode":               "What the item does: run a program, connect to a server, or one of modemmenu's own.",
	"item.command":            "The program run on the line (by /bin/sh), for mode = exec.  {user} and the other placeholders work here.",
	"item.run_as":             "The system user the program runs as (modemmenu must run as root).",
	"item.tty":                "How the line is set up for the program: raw as it comes, or sane for programs that expect a normal terminal.",
	"item.pass_password":      "Whether the program gets the caller's password ({password}, MODEM_PASSWORD).",
	"item.session_file":       "yes: the call's details go in a JSON file the program can read (MODEM_SESSION_FILE).",
	"item.after":              "What happens when the item ends: hang up, or back to the menu.",
	"item.connect_timeout":    "Seconds to wait for the server to answer.",
	"item.terminal":           "The terminal type telnet tells the server.",
	"item.header":             "yes: sends the call's details to the server first, for modemmenu -serve or the BBS.",
	"item.handoff":            "yes: hands the caller's line itself to the server (unix socket and header = yes), instead of copying it through modemmenu.",
	"item.users":              "If set, only these users see the item (or members of its groups).",
	"item.groups":             "If set, only members of these groups see the item (or its users).",
	"item.min_baud":           "Only callers connected at least this fast see the item.",
	"item.callerids":          "Only callers whose number matches one of these see the item.",
	"item.guest":              "Whether the guest account sees the item.",
	"item.dropfile":           "Door drop files written for the program before it starts and removed after.",
	"item.dropfile_lowercase": "Names the drop files in lower case, for doors on Linux.",
	"item.dropfile_dir":       "Where the drop files are written ({node} is the door's node).  Not set: a new directory each time.",
	"item.door_sys_lines":     "52 for the full DOOR.SYS, or 31 for the old short one some doors want.",
	"item.security":           "The security level this door's drop files give (0: the one in [dropfile]).",
	"item.host":               "The server to connect to.",
	"item.port":               "The server's port.",
	"item.path":               "A unix socket to connect to instead of a host and port.",
	"item.send":               "Sent to the server as soon as it answers, such as a login: {user}\\r{password}\\r.",
	"item.rlogin_client_user": "rlogin's client user name ({password} for BBSes that take the password there).",
	"item.rlogin_server_user": "rlogin's server user name: the caller's user name on the server.",
	"item.rlogin_terminal":    "The terminal type and speed rlogin tells the server.",
	"item.time_warning":       "Whether time warnings are written on the line while this item runs (no for PPP, where they would break the link).",
	"item.min_password":       "The shortest new password the Change password item accepts.",
	"item.count":              "How many callers the Last callers item shows.",
}

// setupDesc: a section's (key "") or key's description, "" none
func setupDesc(kind, key string) string {
	if key == "" {
		return setupDescs[kind]
	}
	return setupDescs[kind+"."+key]
}
