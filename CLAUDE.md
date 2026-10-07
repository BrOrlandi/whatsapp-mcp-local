# WhatsApp MCP Local

The local version of WhatsApp MCP: a desktop app (and a command line) that runs on the person's
own computer, over wacli. It is a rewrite from scratch, not a "v2" of the server version
(`BrOrlandi/whatsapp-mcp`, Evolution API on a VPS), which stays maintained for running 24 hours.
Nothing keeps the old "v2" name: the command line's folder is `~/.whatsapp-mcp`, its service
`com.brorlandi.whatsapp-mcp` (`whatsapp-mcp.service` on Linux).

## Site

`site/` is the static landing page at https://whatsapp-mcp.brorlandi.xyz (Vercel project
`whatsapp-mcp-site`, Root Directory `site`). A push to `main` that changes `site/` publishes it;
pushes that don't are skipped by the Ignored Build Step (`git diff --quiet HEAD^ HEAD -- .`).
Other branches get preview deployments. A manual deploy runs from the repository root
(`vercel deploy --prod`), not from `site/`. It covers both versions: the downloads point at
this repository's latest release by the fixed asset names, and `/servidor` is a separate page
for the server project, whose links all point at `BrOrlandi/whatsapp-mcp`.

The "Veja como funciona" video on the home page is a Remotion project in `video/` (narrated with
Gemini TTS, subtitles burned in); `npm run publish` there renders it into `site/assets/video/`.
It shows the app's screens and names its features, so a change to the setup flow or the tools
may call for a new cut (`video/README.md`).

## wacli version

The app ships one wacli, pinned in `build/wacli.env` (the version and the sha256 of each system,
from that release's `checksums.txt`); it is what talks to WhatsApp, through whatsmeow. Bruno
watches the releases of `openclaw/wacli` on GitHub. When building a feature or changing the
code, check whether a newer wacli is out:

```sh
. build/wacli.env; echo "pinned v$WACLI_VERSION, latest $(gh release view -R openclaw/wacli --json tagName -q .tagName)"
```

If it is newer, read its release notes and tell Bruno. A release that fixes the connection after
a change on WhatsApp's side (a whatsmeow update, pairing, sync, sending) is worth a patch release
of its own that only bumps `build/wacli.env`; anything else can wait for the next release. Before
bumping, run the tests and check the Status tab and the chat previews: the database schema can
change between versions (`docs/dependencias.md#wacli`).

## Release & Changelog

Structured convention: `.claude/release.json` (read by the `release` skill). Process:
`docs/desenvolvimento.md#publicar`.

- **One release unit**: the desktop app and the command line ship together. The version is the
  git tag (`vX.Y.Z`, annotated); builds stamp it with `-X main.version` (`VERSION=X.Y.Z` in the
  `scripts/build-*.sh`), so there is no version file to bump.
- **Changelog**: `CHANGELOG.md`, Keep a Changelog, pt-BR, written for users. Each entry names the
  wacli version inside the app (`build/wacli.env`). The GitHub release body is that entry plus
  the first-run caveats (Windows SmartScreen, `chmod +x` for the AppImage).
- **Publish**: always by hand, on Bruno's Mac; CI only runs the tests, and there is no release
  workflow to bring back. `scripts/release.sh X.Y.Z` builds the tag into `dist/vX.Y.Z/` (the
  signed and notarised `.dmg`, the Windows installer and the Linux AppImage and `.deb` in Docker,
  the command line, `checksums.txt`); then push `main` and the tag, and `gh release create
  --latest` with those files. The Windows installer is not signed, by decision. Asset names are
  fixed (`WhatsApp-MCP.dmg`, `WhatsApp-MCP-Setup.exe`, `WhatsApp-MCP-<arch>.AppImage`): the app's
  updater looks them up. The transcription programs ship in separate `sidecars-N` releases,
  never marked latest, pinned in `internal/sidecar/manifest.json`.
- **Landing page downloads**: every release must reach the download page
  (https://whatsapp-mcp.brorlandi.xyz/download), which is where the README's download button
  goes, never to a GitHub release. Its buttons point at `releases/latest/download/<fixed name>`,
  and `site/assets/site.js` reads the latest release from the GitHub API for the version, the
  sizes and the `.deb` links (their names carry the version). After publishing, open the page and
  check that every system offers the new version. If a release adds, renames or removes an asset,
  update the links and `data-asset` patterns in `site/download/index.html` and redeploy the site.
- Bump from conventional commits (breaking → major, `feat` → minor, otherwise patch), confirmed
  with the user; while the version is 0.x, minor marks a release with new features.

## Git workflow

<!-- claude-skill:commit default-branch-policy=direct -->

Work lands directly on the default branch. Commit to it without asking for confirmation, and do not
propose creating a feature branch first.
