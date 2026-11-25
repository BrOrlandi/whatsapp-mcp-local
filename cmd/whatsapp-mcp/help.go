package main

import "strings"

// commandHelp is what `whatsapp-mcp <command> --help` (and `whatsapp-mcp help
// <command>`) prints: the command line is documented here, not on the site,
// so each entry says what the command does, its arguments and flags, and an
// example. Aliases point at the same entry through helpAliases.
var commandHelp = map[string]string{
	"serve": `Usage: whatsapp-mcp serve

Runs WhatsApp MCP in this terminal until Ctrl+C: it keeps wacli syncing
WhatsApp and serves the MCP and the control panel on localhost. The log goes
to this terminal. To keep it running at login instead, use
` + "`whatsapp-mcp service install`" + `, which runs this same command as a service.

Running whatsapp-mcp with no command is the same as serve.

The port comes from WHATSAPP_MCP_PORT, else from ` + "`whatsapp-mcp port`" + `, else 47821.

Example:
  whatsapp-mcp serve
  WHATSAPP_MCP_PORT=48000 whatsapp-mcp serve
`,

	"service": `Usage: whatsapp-mcp service install [--no-open]
       whatsapp-mcp service uninstall
       whatsapp-mcp service stop|start|restart

Runs WhatsApp MCP in the background, starting at login and restarting if it
stops: a launchd agent on macOS (com.brorlandi.whatsapp-mcp), a systemd user
service on Linux (whatsapp-mcp.service).

  install     write the service and start it. It keeps this program's path,
              WACLI_BIN, WACLI_STORE_DIR, WHATSAPP_MCP_DATA and, when set,
              WHATSAPP_MCP_TOKEN and WHATSAPP_MCP_LANG. On Linux it also turns
              on systemd's linger, so the service keeps running after you log
              out of SSH. Then it opens the panel in the browser to connect
              WhatsApp; on a server, or with --no-open, it tells you to run
              ` + "`whatsapp-mcp pair`" + ` instead.
  uninstall   stop the service and remove it. WhatsApp stays connected and the
              data stays in place; install brings it back as it was.
  stop        stop it, as if the computer were off: messages are not received
              until start. It still starts at the next login.
  start       start it again.
  restart     stop and start it, for example after changing an environment
              variable.

Run install again after moving this program. In Docker there is no service:
the container is the service (docker compose up -d, stop, restart).

Example:
  whatsapp-mcp service install --no-open
  whatsapp-mcp service restart
`,

	"status": `Usage: whatsapp-mcp status [--json]

Says where things stand: the version, which WhatsApp number is connected, the
health of the sync, the MCP address and what to do next.

  --json   the daemon's full status as one JSON object on stdout

Exit codes, for scripts: 0 ready (WhatsApp connected, the AI tools can use
it), 4 not yet (the next step says why: connect, scan the QR code, history
still arriving), 3 WhatsApp MCP is not running.

Example:
  whatsapp-mcp status
  until whatsapp-mcp status --json >/dev/null; do sleep 5; done
`,

	"pair": `Usage: whatsapp-mcp pair [--phone <number>] [--timeout 5m] [--no-wait]
                         [--qr-file qr.png] [--invert] [--json]

Connects WhatsApp to this computer. It draws a QR code in the terminal and
keeps it current (WhatsApp replaces it about every 20 seconds) until the phone
reads it: on the phone, WhatsApp › Settings › Linked devices › Link a device.
It ends once the phone has linked; Ctrl+C cancels the connection.

  --phone <number>   instead of the QR code, get a code to type on the phone
                     (Link with phone number instead). The number with the
                     country code, in any format: '+55 11 91234-5678'.
  --timeout <time>   give up after this long (exit 5), for example 5m or 90s.
                     No limit by default.
  --no-wait          show the first QR code or code and exit, leaving the
                     connection open for another program to follow with
                     ` + "`whatsapp-mcp status`" + `.
  --qr-file <path>   also save each QR code as a PNG image there.
  --invert           draw the QR code for dark text on a light background,
                     when the terminal has no colours (NO_COLOR, a pipe).
  --json             one JSON object per line: {"event":"qr",...},
                     {"event":"code",...}, then {"event":"paired",...}
                     ({"event":"already_paired"} when it already was).

If a number is already connected it says so and changes nothing; to connect
another one, run ` + "`whatsapp-mcp logout`" + ` first. Alias: connect.

Exit codes: 0 connected, 4 the connection failed or was cancelled, 5 timeout,
130 Ctrl+C.

Example:
  whatsapp-mcp pair
  whatsapp-mcp pair --phone '+55 11 91234-5678'
  whatsapp-mcp pair --no-wait --qr-file /tmp/qr.png
`,

	"logout": `Usage: whatsapp-mcp logout [--yes] [--json]

Disconnects WhatsApp from this computer: it leaves the phone's linked
devices, and the AI tools stop reaching it. It asks for confirmation first;
to connect again, run ` + "`whatsapp-mcp pair`" + `.

  --yes    do not ask (needed when there is no terminal, or with --json)
  --json   {"ok":true,"paired":false} on stdout

Alias: disconnect.

Example:
  whatsapp-mcp logout --yes
`,

	"port": `Usage: whatsapp-mcp port [<port>] [--json]

Shows the port the MCP and the panel answer on (47821 by default), or changes
it. The new port is saved in the data folder and the service restarts on it;
the AI tools set up with the old address need the new one (see
` + "`whatsapp-mcp config`" + `).

  <port>   a number from 1024 to 65535, free on this computer
  --json   {"port":47821,"source":"default","mcp_url":"..."}; source is
           default, config (set by this command) or env

When WHATSAPP_MCP_PORT is set it wins, and this command cannot change the port
(exit 6): change the variable instead.

Example:
  whatsapp-mcp port
  whatsapp-mcp port 48000
`,

	"token": `Usage: whatsapp-mcp token [show|new|set <token>|clear] [--json]

The key the MCP and the API's actions ask for. Without one, any program on
this computer can use the MCP; with one, each request needs the header
Authorization: Bearer <token>. Changing it restarts the service, and the AI
tools need the new configuration (see ` + "`whatsapp-mcp config`" + `).

  (nothing)      say whether there is a token, without showing it
  show           print the token
  new            create a random token and print it
  set <token>    use this token: at least 16 characters, no spaces; - reads
                 it from standard input
  clear          remove the token
  --json         {"enabled":true,"source":"file"} (and "token" for show and new)

The token is kept in the data folder, readable only by you. When
WHATSAPP_MCP_TOKEN is set it wins, and only show works here (exit 6
otherwise).

Example:
  whatsapp-mcp token new
  echo "$MY_TOKEN" | whatsapp-mcp token set -
`,

	"password": `Usage: whatsapp-mcp password hash|set [--json]

The panel's password for the server mode, where the panel is reached at the
server's public address (WHATSAPP_MCP_PUBLIC_URL) and asks for it. The
password is asked twice without showing it, or read as one line from a pipe;
it is never an argument, and never printed.

  hash   print its bcrypt hash, for WHATSAPP_MCP_PANEL_PASSWORD_HASH (in a
         compose.yaml, write each $ of the hash as $$)
  set    keep its hash in the data folder, readable only by you; the
         variable wins when both exist (exit 6 here then)
  --json {"hash":"$2a$..."} for hash, {"ok":true} for set

A new password signs out everyone signed in to the panel. At least 10
characters.

Example:
  whatsapp-mcp password hash
  whatsapp-mcp password set
`,

	"webhooks": `Usage: whatsapp-mcp webhooks [list] [--json]
       whatsapp-mcp webhooks add <url> [--events message,reaction,receipt,poll_vote,online]
                                 [--include-own] [--chats <jid>,<jid>] [--json]
       whatsapp-mcp webhooks remove|enable|disable|test <id> [--json]

Webhooks post each new WhatsApp message to a URL of yours, as it arrives: for
notifications, automatic replies or a log of your own. Each delivery is signed
with a secret, in the X-WhatsApp-MCP-Signature header. The format of what is
posted: http://127.0.0.1:47821/webhooks/documentacao (with your port).

  list            the webhooks, with their id, state and deliveries (default)
  add <url>       create one; it prints the secret once, keep it
    --events        which events to post: message, reaction, receipt, online
                    (online: WhatsApp is back on this computer, after it
                    started, woke up or came back online)
                    (default: message)
    --include-own   also post the messages you send
    --chats         only these chats, by JID (from the AI tools or the panel)
  remove <id>     delete it
  enable <id>     turn it on again (after it was turned off, by you or after
                  failing too often)
  disable <id>    turn it off, keeping it
  test <id>       post a test event now and say how it went (exit 1 if it
                  was not delivered)

Alias: webhook.

Example:
  whatsapp-mcp webhooks add https://example.com/hook --events message,reaction
  whatsapp-mcp webhooks test 3f9a1c0b7d2e
`,

	"logs": `Usage: whatsapp-mcp logs [-n 100] [-f]

Shows the log of the WhatsApp MCP service: what happened with the sync, the
connection and the AI tools' requests. On macOS it is
~/Library/Logs/whatsapp-mcp.log; on Linux, the service's journal
(journalctl --user -u whatsapp-mcp).

  -n <lines>       how many of the last lines to show (default 100)
  -f, --follow     keep showing new lines as they are written, until Ctrl+C

Under ` + "`whatsapp-mcp serve`" + ` the log goes to that terminal, and in Docker to the
container's output (docker compose logs -f).

Example:
  whatsapp-mcp logs -n 300
  whatsapp-mcp logs -f
`,

	"update": `Usage: whatsapp-mcp update [--check] [--json]

Downloads and installs the latest release of this program, in place, and
restarts the service. WhatsApp stays connected.

  --check   only say whether a newer version is out, without installing it
  --json    {"current":"1.7.0","latest":"1.7.1","available":true}
            (and "updated":true after installing)

In Docker, update the image instead (docker compose build --pull &&
docker compose up -d).

Example:
  whatsapp-mcp update --check
  whatsapp-mcp update
`,

	"config": `Usage: whatsapp-mcp config

Prints how to set up the AI tools to use this WhatsApp MCP, with this
computer's address, port and token already filled in:

  Claude Code      the ` + "`claude mcp add`" + ` command to run
  Claude Desktop   the JSON to add to its configuration file, and where that
                   file is (Chat and Cowork use the bridge command)

Any other MCP client connects over HTTP to the address that
` + "`whatsapp-mcp status`" + ` shows (http://127.0.0.1:47821/mcp by default), with the
header Authorization: Bearer <token> when there is a token.

Example:
  whatsapp-mcp config
`,

	"transcription": `Usage: whatsapp-mcp transcription install
       whatsapp-mcp transcription status
       whatsapp-mcp transcription local
       whatsapp-mcp transcription remote <address> [--model <model>]
       whatsapp-mcp transcription remote off
       whatsapp-mcp transcription openai [--model <model>]
       whatsapp-mcp transcription context none|names|messages|auto
       whatsapp-mcp transcription serve [--listen <host:port>] [--token-file <file>]

Voice notes are transcribed in one of three modes:

  local    on this computer, with whisper.cpp (install sets it up)
  remote   on any server that speaks OpenAI's transcription API, at any
           address (on the internet or your network): you give its address
           and its key. For example a computer running transcription serve,
           whisper.cpp's whisper-server, speaches or a hosted service.
  openai   by OpenAI, with your API key (whisper-1 by default;
           gpt-4o-transcribe and gpt-4o-mini-transcribe also work). The
           audio goes to OpenAI.

install sets up transcription on this computer: it downloads whisper.cpp and
its model (about 600 MB) into the data folder. It says so when this system is
not supported.

local, remote and openai choose the mode. remote and openai read the token or
the API key from the standard input (typed without being shown, or piped),
never from an argument; empty keeps the one already kept. When the server or
OpenAI fails (off, asleep, slow, refused), voice notes are transcribed here,
if install was run here. The variables WHATSAPP_MCP_TRANSCRIPTION_MODE,
_URL, _TOKEN, _MODEL and _OPENAI_KEY win over these (Docker).

context chooses, in remote and openai only, how much of the conversation
goes with each voice note to the server or to OpenAI, to help it spell names
and odd words:
  none       only the audio (the default)
  names      the chat's name and the names of the people in it, no messages
  messages   the names and the latest text messages before the voice note;
             messages in another language can make the audio come out
             translated
  auto       the default, none
Anything but none leaves this computer with the audio. Switching to remote or
openai goes back to auto. In local the context is always names, which stay
here, and is not a choice: context then says so and changes nothing (a level
already chosen waits for remote or openai). Voice notes already transcribed
are kept as they were; transcribe_audio with refresh redoes one.
WHATSAPP_MCP_TRANSCRIPTION_CONTEXT (none, names or messages) wins over it in
remote and openai; in local it is ignored, and the log says so.

serve makes this computer one such server, speaking OpenAI's API
(POST /v1/audio/transcriptions, GET /v1/models): a Mac with
Apple Silicon transcribes in seconds what a server's processor takes minutes
on (on Linux and in Docker it runs on the processor only). It needs install
first, and listens on
0.0.0.0:47823 by default. Every request needs the token, as
Authorization: Bearer <token>: WHATSAPP_MCP_TRANSCRIPTION_TOKEN, else the one
in --token-file (default transcription-server.token in the data folder),
made the first time and readable only by you. It never prints the token. It
runs until stopped. Setting it up, keeping it running (launchd, systemd,
Docker) and reaching it from elsewhere:
https://github.com/BrOrlandi/whatsapp-mcp-local/blob/main/docs/transcription-server.md

status says the mode, where voice notes go and whether the server answers.

Examples:
  whatsapp-mcp transcription install
  whatsapp-mcp transcription serve
  whatsapp-mcp transcription remote http://192.168.0.10:47823 < token-file
  whatsapp-mcp transcription openai --model gpt-4o-mini-transcribe
  whatsapp-mcp transcription context none
  whatsapp-mcp transcription status
`,

	"open": `Usage: whatsapp-mcp open

Opens the control panel in the browser, at http://127.0.0.1:47821/ (with your
port). On a server or over SSH there is no browser to open: reach the panel
through an SSH tunnel (ssh -L 47821:127.0.0.1:47821 <server>), or use the
commands, since everything the panel does has one.

Example:
  whatsapp-mcp open
`,

	"bridge": `Usage: whatsapp-mcp bridge [--url <address> [--key-file <file>]]

An MCP server on standard input and output that forwards every request to the
running WhatsApp MCP. It is for AI tools that only start MCP servers as local
programs, such as Claude Desktop's Chat and Cowork: ` + "`whatsapp-mcp config`" + `
prints the configuration that runs it. You do not run it yourself.

It reaches the daemon at WHATSAPP_MCP_PORT (or the saved port) and sends
WHATSAPP_MCP_TOKEN (or the saved token).

With --url it forwards to a WhatsApp MCP server instead, at its public address
(https://whatsapp.example.com/mcp), with an API key made there
(` + "`whatsapp-mcp apikey create`" + ` or the panel's connections page). The key comes
from WHATSAPP_MCP_API_KEY or from the file --key-file names, never from an
argument; nothing of WhatsApp needs to be installed on this computer.

Example, in Claude Desktop's configuration:
  "command": "whatsapp-mcp", "args": ["bridge", "--url", "https://whatsapp.example.com/mcp"],
  "env": {"WHATSAPP_MCP_API_KEY": "wamcp_key_..."}
`,

	"server": `Usage: whatsapp-mcp server [status] [--json]
       whatsapp-mcp server setup [--url <address>] [--listen <host:port>] [--trusted-proxy <list>]
                                 [--new-password] [--no-service] [--json]
       whatsapp-mcp server off [--no-service]

The server mode on a computer without Docker: a second listener, for a proxy
or a tunnel with HTTPS, where the panel asks for its password and agents on
other computers come in with a key or a login (docs/servidor.md). With
Docker, install-docker.sh does all of this instead.

  status    whether the server mode is on, and its address
  setup     asks the public address (or --url) and, the first time or with
            --new-password, the panel's password twice without showing it
            (or one line from a pipe; never an argument). It keeps them in
            the data folder and installs the service, which then serves the
            public listener too
  off       the local listener only, as before

  --listen         where the public listener answers (default 127.0.0.1:47822)
  --trusted-proxy  the proxy's addresses, CIDR ranges or names, comma
                   separated; by default a proxy on this computer (loopback)
  --no-service     keep the settings without touching the service

The variables WHATSAPP_MCP_PUBLIC_URL, _PUBLIC_LISTEN, _TRUSTED_PROXY and
_PANEL_PASSWORD_HASH win over what setup kept.

Example:
  whatsapp-mcp server setup --url https://whatsapp.example.com
`,

	"healthcheck": `Usage: whatsapp-mcp healthcheck

Exits 0 when the daemon answers on this computer, 3 when it does not: the
Docker image's healthcheck. It does not look at WhatsApp; status does.
`,

	"apikey": `Usage: whatsapp-mcp apikey [list] [--json]
       whatsapp-mcp apikey create --name <name> [--read-only] [--json]
       whatsapp-mcp apikey revoke <id> [--json]

The keys AI tools on other computers use to reach the MCP at the server's
public address (the server mode, WHATSAPP_MCP_PUBLIC_URL), sent as
Authorization: Bearer <key>. For tools that log in (Claude in the browser or
on the phone, ChatGPT), the login with the panel's password needs no key.

  list      the keys, with their id, name, access and last use
  create    make a key and print it, the only time it is shown; --read-only
            lets it read and search, never send, delete or change groups
  revoke    stop a key at once

A key nobody uses within 24 hours of its creation stops working. Only a hash
of each key is kept, in the data folder.

Example:
  whatsapp-mcp apikey create --name "Claude Code no notebook"
  whatsapp-mcp apikey revoke k_1a2b3c4d5e6f
`,

	"version": `Usage: whatsapp-mcp version

Prints this program's version. Also: whatsapp-mcp --version, -v.
`,

	"help": `Usage: whatsapp-mcp help [<command>]

Lists the commands, or explains one. Also: whatsapp-mcp <command> --help.
`,
}

var helpAliases = map[string]string{
	"connect":    "pair",
	"disconnect": "logout",
	"webhook":    "webhooks",
	"--version":  "version",
	"-v":         "version",
	"--help":     "help",
	"-h":         "help",
}

// helpFor is the help of a command, or "" when there is none.
func helpFor(cmd string) string {
	if alias, ok := helpAliases[cmd]; ok {
		cmd = alias
	}
	return commandHelp[cmd]
}

// wantsHelp reports whether the arguments ask for help: -h, --help or -help
// anywhere before a lone --.
func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		switch strings.TrimLeft(a, "-") {
		case "h", "help":
			if strings.HasPrefix(a, "-") {
				return true
			}
		}
	}
	return false
}
