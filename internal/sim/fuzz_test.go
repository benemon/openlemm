package sim

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestFuzzTraces replays every play<N>-<id>.csv recording in the fixtures' fuzz directory at play
// index N, with its input, final map and music-mode sound requests where they exist. A recording's
// first difference ends its subtest with a DIVERGE line; a recording that matches logs a MATCH line.
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
					if mode != "music" {
						t.Fatalf("UNSUPPORTED %s sound mode %s", name, mode)
					}
				}
				sound = "ok"
			}
			var got [][3]int
			collect := func(g *Game) {
				for _, s := range g.Sounds {
					got = append(got, [3]int{g.Update - 1, s.Sample, s.Pitch})
				}
			}
			g := replay(t, filepath.Join("fuzz", name+".csv"), play, collect)
			updates := g.Update
			m := "absent"
			if _, err := os.Stat(filepath.Join(dir, name+".map")); err == nil {
				if compareMap(t, g, filepath.Join("fuzz", name+".map")); t.Failed() {
					return
				}
				m = "ok"
			}
			if sound == "ok" {
				g.Step()
				collect(g)
				if compareSounds(t, name, got, want["music"]); t.Failed() {
					return
				}
			}
			t.Logf("MATCH %s updates=%d map=%s sound=%s", name, updates, m, sound)
		})
	}
}
