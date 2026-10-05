#!/usr/bin/env bash
# Imports the Developer ID Application certificate into a keychain of its own
# on a CI Mac, so codesign can use it without a prompt.
#
# MACOS_CERT_P12       the certificate and its key, exported as .p12, base64
# MACOS_CERT_PASSWORD  the .p12's password
set -euo pipefail
keychain="$RUNNER_TEMP/signing.keychain-db"
password="$(openssl rand -base64 24)"
echo "$MACOS_CERT_P12" | base64 --decode > "$RUNNER_TEMP/cert.p12"
security create-keychain -p "$password" "$keychain"
security set-keychain-settings -lut 21600 "$keychain"
security unlock-keychain -p "$password" "$keychain"
security import "$RUNNER_TEMP/cert.p12" -P "$MACOS_CERT_PASSWORD" -A -t cert -f pkcs12 -k "$keychain"
security set-key-partition-list -S apple-tool:,apple: -k "$password" "$keychain" >/dev/null
security list-keychains -d user -s "$keychain" $(security list-keychains -d user | tr -d '"')
rm -f "$RUNNER_TEMP/cert.p12"
security find-identity -v -p codesigning "$keychain"
