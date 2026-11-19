package sim

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestFuzzTraces replays every play<N>-<id>.csv recording in the fixtures' fuzz directory at play
// index N, with its input, final map and sound requests (music or effects mode) where they exist.
// A recording's first difference ends its subtest with a DIVERGE line; a recording that matches
// logs a MATCH line.
func TestFuzzTraces(t *testing.T) {
	dir := filepath.Join(os.Getenv("OPENLEMM_FIXTURES"), "fuzz")
	files, _ := filepath.Glob(filepath.Join(dir, "play*-*.csv"))
	if os.Getenv("OPENLEMM_FIXTURES") == "" || len(files) == 0 {
		t.Skip("no fuzz recordings")
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".csv")
		t.Run(name, func(t *testing.T) {
			n, _, _ := strings.Cut(strings.TrimPrefix(name, "play"), "-")
			play, err := strconv.Atoi(n)
			if err != nil {
				t.Fatalf("%s: no play index", name)
			}
			want := map[string][][3]int{}
			sound := "absent"
			if _, err := os.Stat(filepath.Join(dir, name+".sound")); err == nil {
				want = readSounds(t, filepath.Join(dir, name+".sound"))
				for mode := range want {
					if (mode != "music" && mode != "effects") || len(want) > 1 {
						t.Fatalf("UNSUPPORTED %s sound mode %s", name, mode)
					}
					sound = mode
				}
			}
			var got [][3]int
			collect := func(g *Game) {
				// The first Step runs in music mode; it makes no mode-dependent request.
				g.Effects = sound == "effects"
				for _, s := range g.Sounds {
					got = append(got, [3]int{g.Start(), s.Sample, s.Pitch})
				}
			}
			g := replay(t, filepath.Join("fuzz", name+".csv"), play, collect)
			updates := g.start
			m := "absent"
			if _, err := os.Stat(filepath.Join(dir, name+".map")); err == nil {
				if compareMap(t, g, filepath.Join("fuzz", name+".map")); t.Failed() {
					return
				}
				m = "ok"
			}
			if sound != "absent" {
				// The recording writes a request one update late, so none made in its final
				// update is in it: the engine's requests there are not stepped or compared.
				if compareSounds(t, name, got, want[sound]); t.Failed() {
					return
				}
			}
			t.Logf("MATCH %s updates=%d map=%s sound=%s", name, updates, m, sound)
		})
	}
}
