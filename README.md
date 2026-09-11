# Kaizen CLI

Command-line interface for Kaizen board management.

**This repository distributes the Kaizen CLI. It does not contain its source.**
The source is maintained privately; every release below is built and published
from there automatically.

## Install

```sh
brew install senseylabs/tap/kaizen-cli
```

The formula is named `kaizen-cli`; the binary it installs is `kaizen`. Upgrade
with `brew upgrade kaizen-cli`.

Prebuilt archives for macOS (Intel and Apple Silicon) and Linux amd64 are
attached to every [release](https://github.com/senseylabs/kaizen-cli/releases/latest),
alongside a checksums file.

## Verifying a download

```sh
VERSION=0.6.0
curl -sSfLO https://github.com/senseylabs/kaizen-cli/releases/download/v${VERSION}/kaizen-cli_${VERSION}_checksums.txt
curl -sSfLO https://github.com/senseylabs/kaizen-cli/releases/download/v${VERSION}/kaizen-cli_${VERSION}_darwin_arm64.tar.gz
sha256sum --check --ignore-missing kaizen-cli_${VERSION}_checksums.txt
```

## `go install` is not supported

`go install github.com/senseylabs/kaizen-cli@latest` will not build a current
version: this repository carries no Go source. Use Homebrew or a direct download.

## Support

[Open an issue](https://github.com/senseylabs/kaizen-cli/issues).

## Licence

MIT — see [LICENSE](LICENSE). Applies to the released binaries and to every
previously published version of the source.
