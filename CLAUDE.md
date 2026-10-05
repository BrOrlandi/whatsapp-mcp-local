# WhatsApp MCP

## Release & Changelog

Structured convention: `.claude/release.json` (read by the `release` skill). Process and
signing secrets: `docs/desenvolvimento.md#publicar`.

- **One release unit**: the desktop app and the command line ship together. The version is the
  git tag (`vX.Y.Z`, annotated); builds stamp it with `-X main.version` (`VERSION=X.Y.Z` in the
  `scripts/build-*.sh`), so there is no version file to bump.
- **Changelog**: `CHANGELOG.md`, Keep a Changelog, pt-BR, written for users. Each entry names the
  wacli version inside the app (`build/wacli.env`). The GitHub release body is that entry plus
  the first-run caveats (Windows SmartScreen, `chmod +x` for the AppImage).
- **Publish**: a tag `v*` runs `.github/workflows/release.yml`, which builds every system and
  publishes the release with `checksums.txt`. A release already published by hand (built locally
  with the same scripts) is left alone. Asset names are fixed (`WhatsApp-MCP.dmg`,
  `WhatsApp-MCP-Setup.exe`, `WhatsApp-MCP-<arch>.AppImage`): the app's updater looks them up.
- Bump from conventional commits (breaking → major, `feat` → minor, otherwise patch), confirmed
  with the user; while the version is 0.x, minor marks a release with new features.
