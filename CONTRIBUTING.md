# Contributing to OpenLemm

Contributions are welcome: bug reports, fixes and new behaviour. Everything is contributed under
the project's licence, GPL-3.0-or-later; there is no contributor licence agreement.

## Ground rules

- Contribute no code, data or disassembly of the original game, and no files or excerpts extracted
  from a disc: no disc images, palettes, graphics, levels, samples or music copied in as tables, and
  no screenshots of the original. Fingerprints (SHA-256) of disc files are fine.
- Base a change on your own disc's data or on what the original visibly does in an emulator.
- Game text appears in tests only when it is plainly visible on screen to a player of the original
  and the test needs it. Levels are named by play position (Fun 1, Tricky 14), never by title.
- The project's name, logo and icons never use the original game's name, logos or box art.
- A change to behaviour comes with a test that fails without it. Say in the pull request what the
  behaviour was compared against (an emulator observation, a trace, a frame grab or the disc's
  bytes) and how you saw the test fail.

## Layout

| Path | Contents |
|---|---|
| `cmd/openlemm` | the program: game flow, drawing, sound and input (Ebitengine) |
| `internal/disc` | reading ADFS and JFD images, release identification, ByteKiller unpacking |
| `internal/level` | level records, the level-order table, terrain and object drawing |
| `internal/screen` | the in-level panel, splash screens, title menu and between-level screens |
| `internal/sim` | the simulation; it does not import Ebitengine |

The simulation keeps the original's map size, update order and constants. Presentation (scaling,
filters, window size, interpolation) never feeds back into it.

## Building and testing

Prerequisites are listed in the README under [Building from source](README.md#building-from-source).

```sh
go build ./...
go vet ./...
go test ./...
go run ./cmd/openlemm -disc /path/to/your/disc.adf
go run ./cmd/openlemm -disc /path/to/your/disc.adf -record 77   # play level record 0-99 at once
```

Without data, every test that needs the disc or recordings of the original skips and the rest
run. Tests read their data from three environment variables:

| Variable | Contents |
|---|---|
| `OPENLEMM_DISC` | your disc image (ADF or JFD) |
| `OPENLEMM_JFD` | your disc as a JASPP JFD, for the JFD reader's test |
| `OPENLEMM_FIXTURES` | a directory of recordings of the original game |

```sh
OPENLEMM_DISC=/path/to/your/disc.adf go test ./...
```

The fixtures directory holds recordings made from the original running in an emulator. They are
derived from the game and are not distributed; tests that need them skip without them. Its layout:

- `*.csv`: update-by-update traces of every lemming in one level, with `*.csv.inputs` (skill
  assignments) or `*.csv.input` (pointer and button input) beside them, and `*.map` the terrain
  map at the end of the trace;
- `screens/L000.json` to `L119.json`: the whole displayed screen at each level's start, as state
  and hashes;
- `flow/*.json`: the screens between levels and the input that led to each, as state and hashes.

## Gates

Before sending a change:

- The commands under [Building and testing](#building-and-testing) pass, with `OPENLEMM_DISC` set.
- `gofmt -l .` prints nothing.
