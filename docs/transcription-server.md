# Transcription server

How to run a voice-note transcription server on a computer of your own (a Mac,
a Linux PC, a server) and have WhatsApp MCP send voice notes to it, whether you
use the desktop app or the server version. This guide is for people
comfortable with a terminal. How transcription works inside is in
[Transcrição](transcricao.md) (in Portuguese). This guide in Portuguese:
[Servidor de transcrição](servidor-de-transcricao.md).

The server is WhatsApp MCP's own command line:
`whatsapp-mcp transcription serve`. It uses the same engine as local
transcription (whisper.cpp with the Whisper large-v3-turbo model) and speaks
OpenAI's transcription API, with a key. It needs no WhatsApp account and none
of the rest of the app: only the command line and the engine.

## When it is worth it

Transcription is heavy. On a computer without a GPU, a voice note takes about
as long as it lasts to become text, or longer on a weak processor (on a server
with 4 weak cores, over 5 minutes for 2 to 3 minutes of speech). On a Mac with
an Apple chip, which uses the GPU (Metal), a 10-second voice note takes about
2 seconds.

So the typical case is this: WhatsApp MCP runs on a server or a mini PC with no
GPU, and a Mac with an Apple chip in the same home or office does the
transcription. The voice note goes to the Mac, comes back as text, and never
leaves your network.

| Where the server runs | Acceleration | Worth it? |
|---|---|---|
| Mac with an Apple chip (M1 or later) | GPU (Metal) | Yes, this is what it was made for |
| Intel Mac | same macOS package; not measured | Maybe; test first |
| Linux (amd64 or arm64) | processor only: the Linux package does not use a GPU | Only if this computer is much stronger than the one running WhatsApp MCP |
| Linux with an NVIDIA card | the package does not use the card | Prefer one of the [alternatives](#alternatives) with CUDA |
| Windows | local transcription is not supported yet ([issue #14](https://github.com/BrOrlandi/whatsapp-mcp-local/issues/14)) | No; this guide does not cover Windows |

What it needs:

- **Disk**: about 600 MB, once (the model is 574 MB, plus the transcription
  program and a minimal `ffmpeg`).
- **Memory**: about 1 GB free while transcribing (whisper.cpp with this model
  uses about 900 MB).
- **Network**: the computer that sends the voice notes must reach the
  server's port (47823 by default). See [Reaching it](#reaching-it).
- **Staying on**: when the server does not answer, WhatsApp MCP transcribes on
  its own computer, if it can ([below](#if-the-server-does-not-answer)).

It transcribes one voice note at a time; a second request waits for the first
to finish. It takes files up to 64 MB, hours of speech in WhatsApp's format.

## 1. Install the command line

Download only the command line, from the latest release. **Do not use
`install.sh` for this**: it also installs wacli and registers the whole of
WhatsApp MCP as a service, which a transcription server does not need.

macOS with an Apple chip:

```sh
mkdir -p ~/.local/bin
curl -fsSL https://github.com/BrOrlandi/whatsapp-mcp-local/releases/latest/download/whatsapp-mcp_darwin_arm64.tar.gz | tar -xz -C ~/.local/bin
xattr -d com.apple.quarantine ~/.local/bin/whatsapp-mcp 2>/dev/null || true
~/.local/bin/whatsapp-mcp version
```

Linux (replace `amd64` with `arm64` on an ARM processor):

```sh
mkdir -p ~/.local/bin
curl -fsSL https://github.com/BrOrlandi/whatsapp-mcp-local/releases/latest/download/whatsapp-mcp_linux_amd64.tar.gz | tar -xz -C ~/.local/bin
~/.local/bin/whatsapp-mcp version
```

Each release's files and its `checksums.txt` are under
[Releases](https://github.com/BrOrlandi/whatsapp-mcp-local/releases). The
examples below use `~/.local/bin/whatsapp-mcp`; if that folder is on your
`PATH`, `whatsapp-mcp` is enough.

**The data folder.** The command line keeps everything in `~/.whatsapp-mcp`
(or in the folder given by `WHATSAPP_MCP_DATA`): the engine in `bin/`, the
model in `models/` and the server's key. On a Mac that **already has the app**
with transcription installed, point at the app's folder to use the engine and
model it already downloaded, without downloading them again:

```sh
export WHATSAPP_MCP_DATA="$HOME/Library/Application Support/WhatsApp MCP"
```

Do this for every command below (and in the service, in section 5).

## 2. Install the engine and the model

```sh
~/.local/bin/whatsapp-mcp transcription install
```

It downloads the transcription program for your system (checked by sha256)
and the model from Hugging Face (also checked), about 600 MB, once. If
`whisper-cli` and `ffmpeg` are already installed on the computer (with
Homebrew, for example), it uses those and only downloads the model. To check:

```sh
~/.local/bin/whatsapp-mcp transcription status
```

The `on this computer:` line should say `installed (metal)` on a Mac with an
Apple chip, or `installed (cpu)`.

## 3. The key and a first test

```sh
~/.local/bin/whatsapp-mcp transcription serve
```

The first time, it creates the key and keeps it in
`transcription-server.token`, in the data folder, readable only by its owner.
The key is never shown on screen or in the log; it says which file holds it
and the addresses it answers on:

```
transcription server (OpenAI API) listening on [::]:47823 (metal)
on the computer that sends the voice notes, point WhatsApp MCP at it, with one of these addresses:
  http://192.168.0.20:47823
token: the one in /Users/you/.whatsapp-mcp/transcription-server.token
```

`Ctrl+C` stops it. On macOS, if asked, allow incoming connections.

The options:

| Option | What it does | Default |
|---|---|---|
| `--listen <address:port>` | where it answers. `127.0.0.1:47823` answers only on this computer (for a proxy in front of it, or a tunnel) | `0.0.0.0:47823`, every network |
| `--token-file <file>` | the file holding the key. If it does not exist or is empty, it is created with a new key | `transcription-server.token` in the data folder |

The `WHATSAPP_MCP_TRANSCRIPTION_TOKEN` variable, when set, wins over the file.
The key must have at least 16 characters. To choose your own:

```sh
openssl rand -hex 24 > ~/transcription.token && chmod 600 ~/transcription.token
~/.local/bin/whatsapp-mcp transcription serve --token-file ~/transcription.token
```

To change the key, delete the file (or change its contents), restart the
server and give the new one to WhatsApp MCP.

### Test it with curl

With the server running, from another terminal (change the key's path and the
audio; any audio `ffmpeg` reads works, such as WhatsApp's `.ogg`):

```sh
KEY=$(cat ~/.whatsapp-mcp/transcription-server.token)

# is it up, and does it take the key?
curl -s -H "Authorization: Bearer $KEY" http://127.0.0.1:47823/v1/health
# {"accel":"metal","engine":"whisper.cpp large-v3-turbo","ready":true,"version":"..."}

# transcribe
curl -s http://127.0.0.1:47823/v1/audio/transcriptions \
  -H "Authorization: Bearer $KEY" \
  -F file=@audio.ogg -F model=whisper-1 -F language=pt
# {"engine":"whisper.cpp large-v3-turbo","text":"..."}
```

It is OpenAI's format: `POST /v1/audio/transcriptions` as
`multipart/form-data`, with `file` (required), `model` (any value),
`language` (without it, the language is detected), `prompt` (text that helps
it spell names) and `response_format` (`json`, `text` or `verbose_json`).
Without the key, the answer is `401` with `invalid_api_key`. `GET /v1/models`
also answers, as on OpenAI. The full protocol is in
[Transcrição](transcricao.md#servidor-de-transcrição).

Since it speaks OpenAI's API, any OpenAI client can use the server, such as
the official SDK with `base_url="http://<server>:47823/v1"` and the key as
`api_key`.

## 4. Connect WhatsApp MCP

On the computer (or server) where WhatsApp MCP runs, give it the server's
address and key. The address is the one `serve` printed
(`http://192.168.0.20:47823`), or your `https` address if it sits behind a
proxy; without a path, the app adds `/v1`.

**In the panel**: *Settings › Audio transcription › Change where to
transcribe*, choose **On an OpenAI-compatible server**, fill in **Address**
and **Key** and click **Save and test**. Under *Advanced*, **Model** can stay
empty: this server takes any name.

**From the terminal**, with the key on the standard input (typed without being
shown, or from a file; never as an argument, which other users of the computer
can see):

```sh
whatsapp-mcp transcription remote http://192.168.0.20:47823
# token (empty: keep the one kept, or none):   ← paste the key and press Enter

whatsapp-mcp transcription remote http://192.168.0.20:47823 < transcription.token   # or from a file
whatsapp-mcp transcription status
```

`status` should say `transcription server http://192.168.0.20:47823:
answering, model whisper-1 (whisper.cpp large-v3-turbo, metal, <version>)`.
With the wrong key, it says the server refused the key. To go back to
transcribing on the computer itself: `whatsapp-mcp transcription local`.

**In the server version (Docker)**, in the `.env` next to `compose.yml`, then
`docker compose up -d`:

```sh
WHATSAPP_MCP_TRANSCRIPTION_URL=https://transcription.example.com
WHATSAPP_MCP_TRANSCRIPTION_TOKEN=the-server-key
# optional:
# WHATSAPP_MCP_TRANSCRIPTION_MODE=remote      (the _URL alone makes it remote)
# WHATSAPP_MCP_TRANSCRIPTION_MODEL=whisper-1  (this server takes any)
# WHATSAPP_MCP_TRANSCRIPTION_CONTEXT=names    (below)
```

With these variables, the panel only shows the setting, without letting you
change it. Every variable is listed in
[Transcrição](transcricao.md#os-três-modos).

### The conversation context

WhatsApp MCP can send, with the voice note, a little of the conversation so
the server spells names and unusual words better. When the voice note leaves
the computer, that is your choice, in *Settings › Audio transcription ›
Change the context*, with `whatsapp-mcp transcription context` or with
`WHATSAPP_MCP_TRANSCRIPTION_CONTEXT`:

| Level | Value | What goes with it |
|---|---|---|
| Nothing (the default) | `none` | only the audio |
| Names only | `names` | the chat's name and the people's names, no messages |
| Names and recent messages | `messages` | the names and the latest text messages before the voice note |

With a server of your own, **Names only** is a good choice: it is what
transcription on the computer itself uses. With recent messages, a voice note
in a language other than the conversation's can come out translated. Details
in [o nível de contexto](transcricao.md#o-nível-de-contexto).

### If the server does not answer

WhatsApp MCP gives the connection 3 seconds to open and the transcription up
to 10 minutes. If the server is off or asleep, refuses the key, answers with an
error or runs out of time, the voice note is transcribed on the computer
itself, **if transcription is installed there**; if it is not, the AI tool gets
an error saying what failed. `whatsapp-mcp transcription status` and the panel
show whether the server answers.

## 5. Keep it running

### macOS (launchd)

A LaunchAgent starts the server when you log in to the Mac and restarts it if
it stops. **It only runs while your user is logged in**, and a sleeping Mac
does not answer: on a Mac that acts as a server, in *System Settings › Battery*
(or *Energy Saver*), keep it from sleeping when the display is off.

Save as
`~/Library/LaunchAgents/com.brorlandi.whatsapp-mcp-transcription.plist`,
replacing `YOU` with your user name (`whoami`):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.brorlandi.whatsapp-mcp-transcription</string>
  <key>ProgramArguments</key>
  <array>
    <string>/Users/YOU/.local/bin/whatsapp-mcp</string>
    <string>transcription</string>
    <string>serve</string>
  </array>
  <!-- Only with the app installed, to use its engine and model; without the app, delete this block. -->
  <key>EnvironmentVariables</key>
  <dict>
    <key>WHATSAPP_MCP_DATA</key><string>/Users/YOU/Library/Application Support/WhatsApp MCP</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>/Users/YOU/Library/Logs/whatsapp-mcp-transcription.log</string>
  <key>StandardErrorPath</key><string>/Users/YOU/Library/Logs/whatsapp-mcp-transcription.log</string>
</dict>
</plist>
```

```sh
plutil -lint ~/Library/LaunchAgents/com.brorlandi.whatsapp-mcp-transcription.plist
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.brorlandi.whatsapp-mcp-transcription.plist
tail ~/Library/Logs/whatsapp-mcp-transcription.log
```

To copy the key without showing it: `pbcopy < ~/.whatsapp-mcp/transcription-server.token`
(or `pbcopy < "$HOME/Library/Application Support/WhatsApp MCP/transcription-server.token"`
with the app).

Restart: `launchctl kickstart -k gui/$(id -u)/com.brorlandi.whatsapp-mcp-transcription`.
Remove: `launchctl bootout gui/$(id -u)/com.brorlandi.whatsapp-mcp-transcription`
and delete the file.

### Linux (systemd)

A service of your own user, without `sudo`, as WhatsApp MCP itself does. Save
as `~/.config/systemd/user/whatsapp-mcp-transcription.service`:

```ini
[Unit]
Description=WhatsApp MCP transcription server
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=%h/.local/bin/whatsapp-mcp transcription serve
# For another port, or only for a proxy on this computer:
# ExecStart=%h/.local/bin/whatsapp-mcp transcription serve --listen 127.0.0.1:47823
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
```

```sh
systemctl --user daemon-reload
systemctl --user enable --now whatsapp-mcp-transcription.service
loginctl enable-linger "$USER"   # also runs without you logged in, and starts with the computer
systemctl --user status whatsapp-mcp-transcription.service
```

Logs: `journalctl --user -u whatsapp-mcp-transcription -f`. Restart:
`systemctl --user restart whatsapp-mcp-transcription`. Remove:
`systemctl --user disable --now whatsapp-mcp-transcription` and delete the
file.

With a firewall, open the port only to your network, for example with ufw:
`sudo ufw allow from 192.168.0.0/24 to any port 47823 proto tcp`.

### Docker

The server version's image, `ghcr.io/brorlandi/whatsapp-mcp-local`, runs the
transcription server too. **In Docker it only uses the processor**, even on a
Mac (a container cannot reach the Mac's GPU); it is worth it on a strong Linux
machine, not to gain speed on a Mac. A `compose.yml` just for it:

```yaml
services:
  transcription:
    image: ghcr.io/brorlandi/whatsapp-mcp-local:latest
    command: ["transcription", "serve"]
    environment:
      # Only up to 1.9.6: without this line, every transcription fails with "stat /data/tmp".
      # From 1.9.7 on it is not needed (and does no harm).
      TMPDIR: /tmp
    ports:
      - "47823:47823"            # behind a proxy on this computer: "127.0.0.1:47823:47823"
    volumes:
      - transcription:/data
    healthcheck:
      disable: true              # the image's healthcheck is WhatsApp MCP's, not this server's
    restart: unless-stopped

volumes:
  transcription:
```

```sh
docker compose run --rm transcription transcription install   # the engine and model, once, in the volume
docker compose up -d
docker compose logs transcription
docker compose exec transcription cat /data/mcp/transcription-server.token   # the key
```

The address in the log is the one inside the container; use the computer's
(`http://<computer-ip>:47823`).

## Reaching it

The server speaks plain HTTP. The voice note and the key travel in the clear
over the network, so pick the path by where the sender is:

- **On the same network (home, office)**: use the network address `serve`
  prints, `http://192.168.0.20:47823`. Give the server's computer a fixed IP
  on the router (a DHCP reservation), so the address does not change.
- **On another network, over a VPN**: with [Tailscale](https://tailscale.com)
  (or another VPN) on both computers, use the VPN address, such as
  `http://my-mac.example.ts.net:47823`. Traffic is encrypted by the VPN, and
  the port is not open to the internet. It is the simplest way for WhatsApp
  MCP on a rented server to send voice notes to a Mac at home.
- **On the internet**: only behind HTTPS. Keep the server answering only on its
  own computer (`--listen 127.0.0.1:47823`) and put a reverse proxy in front.
  With [Caddy](https://caddyserver.com), which handles the certificate on its
  own, the whole `Caddyfile` is:

  ```
  transcription.example.com {
      reverse_proxy 127.0.0.1:47823
  }
  ```

  The address in WhatsApp MCP becomes `https://transcription.example.com`.
  With another proxy (nginx, for example), raise the request body limit
  (`client_max_body_size 64m`) and the response timeout
  (`proxy_read_timeout 600s`), because a long voice note on a processor takes
  minutes.

Why not open plain HTTP to the internet: anyone on the way can read your voice
notes and the key, and with the key use your computer to transcribe.

## Updating

Download the command line again (the same command as in
[section 1](#1-install-the-command-line)) and restart the server
(`launchctl kickstart -k ...`, `systemctl --user restart ...` or, in Docker,
`docker compose pull && docker compose up -d`). The engine and model stay; no
need to run `install` again.

**Use at least 1.9.6.** Up to 1.9.5, parts of long voice notes were lost in
transcription (from an 8-minute voice note, a third to a half went missing,
without warning), and it is the server that transcribes: updating only the app
on the other side does not fix it. The server's version shows in `/v1/health`
(`version`) and in `whatsapp-mcp transcription status` on the computer that
sends the voice notes.

## Common problems

| What you see | What to do |
|---|---|
| `transcription is not installed on this computer` on start | Run `whatsapp-mcp transcription install` with the same data folder (`WHATSAPP_MCP_DATA`) the service uses |
| `address already in use` | Another program (or another `serve`) uses the port. Stop it, or use `--listen 0.0.0.0:<another-port>` |
| `the token must have at least 16 characters` | The key in the file or in `WHATSAPP_MCP_TRANSCRIPTION_TOKEN` is too short |
| WhatsApp MCP says the server refused the key | The key you gave is not the one in the file the server uses. Check that the service does not use another data folder, or `WHATSAPP_MCP_TRANSCRIPTION_TOKEN` |
| WhatsApp MCP says the server does not answer | From the sending computer: `curl -s -H "Authorization: Bearer $KEY" http://<server>:47823/v1/health`. No answer: is the server up? Did the Mac go to sleep? Does the macOS firewall (or ufw) block the port? Is `--listen` `127.0.0.1` (local only)? |
| The app warns the address is not a transcription server | The address leads to another program. Check the port (47823) and the path (none, or ending in `/v1`) |
| In Docker: `stat /data/tmp: no such file or directory` | The image is 1.9.6 or older: update it, or add `TMPDIR: /tmp` to `compose.yml` (above) |
| Transcription is slow | Check the acceleration in `/v1/health` (`accel`): `cpu` means no GPU, and a voice note takes about as long as it lasts |
| Long voice notes come out cut | The server runs a version before 1.9.6: [update it](#updating) |

Logs: on macOS, `~/Library/Logs/whatsapp-mcp-transcription.log` (with the
LaunchAgent above); on Linux, `journalctl --user -u whatsapp-mcp-transcription`;
in Docker, `docker compose logs transcription`. The server logs little beyond
its start; a failure's details come back in the answer, and WhatsApp MCP shows
the last one in `whatsapp-mcp transcription status` and in the panel.

## Alternatives

WhatsApp MCP takes any server that speaks OpenAI's transcription API, so other
programs work in place of this one. This project's takes WhatsApp's audio as it
comes, applies the language and context the same way local transcription
does, and requires a key. The others:

- **[whisper.cpp](https://github.com/ggml-org/whisper.cpp)**'s
  `whisper-server`: run it with
  `--inference-path /v1/audio/transcriptions --convert -l auto` (`--convert`
  to take WhatsApp's `.ogg`, with the system's `ffmpeg`; `-l auto` because,
  without it, it assumes English). Built with CUDA, it uses an NVIDIA card. It
  asks for no key: keep it behind a VPN or a proxy that asks for one.
- **[Speaches](https://github.com/speaches-ai/speaches)**, with
  faster-whisper, in Docker, with images for NVIDIA GPUs.
- **[LocalAI](https://github.com/mudler/LocalAI)**, a server for many models
  with OpenAI's API, transcription included.
- **[WhisperKit](https://github.com/argmaxinc/argmax-oss-swift)**, by Argmax,
  made for Apple chips, with a local OpenAI-compatible server.

See each one's documentation for how to turn on the OpenAI API and the key.
The address that goes into WhatsApp MCP is the server's root or the one ending
in `/v1`.
