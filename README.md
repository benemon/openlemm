# OpenLemm

[![CI](https://github.com/benemon/openlemm/actions/workflows/ci.yml/badge.svg)](https://github.com/benemon/openlemm/actions/workflows/ci.yml)
[![CodeQL](https://github.com/benemon/openlemm/actions/workflows/codeql.yml/badge.svg)](https://github.com/benemon/openlemm/actions/workflows/codeql.yml)
[![Scorecard](https://github.com/benemon/openlemm/actions/workflows/scorecard.yml/badge.svg)](https://github.com/benemon/openlemm/actions/workflows/scorecard.yml)
[![govulncheck](https://github.com/benemon/openlemm/actions/workflows/govulncheck.yml/badge.svg)](https://github.com/benemon/openlemm/actions/workflows/govulncheck.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/benemon/openlemm/badge)](https://securityscorecards.dev/viewer/?uri=github.com/benemon/openlemm)

> **Independent community project.** OpenLemm is not affiliated with, maintained, supported or
> endorsed by Sony Interactive Entertainment or the original game's developers and publishers.
> Lemmings is a trademark of Sony Interactive Entertainment.

OpenLemm plays the 1991 Acorn Archimedes release of Lemmings from your own copy of the original
disc. It reproduces the original's rules, timings and quirks, including its bugs.

**This repository contains no game data.** You need your own disc.

## Download

Download the archive for your system from the [releases page](../../releases/latest):

| System | Archive |
|---|---|
| macOS, Apple silicon | `openlemm_<version>_darwin_arm64.tar.gz` |
| macOS, Intel | `openlemm_<version>_darwin_amd64.tar.gz` |
| Windows, x64 | `openlemm_<version>_windows_amd64.zip` |
| Windows, Arm | `openlemm_<version>_windows_arm64.zip` |
| Linux, x86-64 | `openlemm_<version>_linux_amd64.tar.gz` |
| Linux, Arm64 | `openlemm_<version>_linux_arm64.tar.gz` |

Each archive holds the `openlemm` program and the [libopenmpt](https://lib.openmpt.org/) library
it plays music with, with libopenmpt's licence; nothing else needs installing. Keep the library in
the same folder as the program. You only need a disc image of the original Archimedes release
(`.adf` or `.jfd`).

The macOS programs are not signed by Apple, so macOS refuses to open them at first. Clear the
quarantine flag from the whole extracted folder, library included, in Terminal:

```sh
xattr -dr com.apple.quarantine openlemm_<version>_darwin_arm64
```

Right-clicking `openlemm` in Finder and choosing **Open** also starts it, but may leave the
library blocked, in which case the game plays without music until the command above is run.

On macOS you can install with [Homebrew](https://brew.sh) instead, which also installs libopenmpt
and needs no quarantine step:

```sh
brew install benemon/tap/openlemm
```

Each release is signed and comes with build provenance; see [SECURITY.md](SECURITY.md) for how to
verify a download.

OpenLemm checks every file on the disc and refuses a release it does not know. If your disc isn't
recognised, please [open an issue](../../issues) with the error message.

## Playing

Put your disc image (`.adf` or `.jfd`) in the same folder as `openlemm`, or in your own folder:
`~/Documents/OpenLemm` on macOS, `Documents\OpenLemm` on Windows, `~/.local/share/openlemm` on
Linux. Then start `openlemm`. It also looks in the folder you start it from, and
`openlemm -disc /path/to/your/disc.adf` plays a disc image anywhere else.

- Push the pointer against the screen edge to scroll (right button for faster), or click the minimap.
- Cmd-Backspace (macOS) or Ctrl-Backspace (Windows, Linux) releases the mouse; click the window to
  capture it again.
- F11 or Cmd/Ctrl-F toggles full screen.
- F1 / F2 change the volume, F5 toggles sound, Escape ends the level. On a Mac keyboard, hold fn
  with the F keys.
- `-pan=false` plays sound effects centred, as the original does.

## Building from source

Prerequisites:

- Go 1.27 or later.
- [libopenmpt](https://lib.openmpt.org/) (on macOS, `brew install libopenmpt`).
- What Ebitengine needs on your platform: on Linux, a C compiler and the X11, OpenGL and ALSA
  development headers. See [Ebitengine's install guide](https://ebitengine.org/en/documents/install.html)
  for each operating system.

Then:

```sh
go install github.com/benemon/openlemm/cmd/openlemm@latest
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Licence

GPL-3.0-or-later. See [LICENSE](LICENSE).
