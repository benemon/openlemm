package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/benemon/openlemm/internal/level"
)

// readInts reads a fixture CSV after its header line as integers, an empty field as 0.
func readInts(t *testing.T, name string) [][]int {
	t.Helper()
	b, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	rows, err := csv.NewReader(b).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	var out [][]int
	for _, r := range rows[1:] {
		v := make([]int, len(r))
		for k, s := range r {
			if s == "" {
				continue
			}
			if v[k], err = strconv.Atoi(s); err != nil {
				t.Fatalf("%s %v: %v", name, r, err)
			}
		}
		out = append(out, v)
	}
	return out
}

// TestPointerTrace plays play12-pointer through the flow from its pre-level screen, moving the
// mouse by the change in the pointer the trace's .csv.input records for each update (and further
// where the capture pushed the mouse on into an edge) with the buttons it records, and compares per
// update the pointer, the view, the view the sim's step used, the chosen lemming and the lemmings
// of the play12-pointer recording.
func TestPointerTrace(t *testing.T) {
	dir := os.Getenv("OPENLEMM_FIXTURES")
	if dir == "" {
		t.Skip("OPENLEMM_FIXTURES not set")
	}
	base := filepath.Join(dir, "play12-pointer.csv")
	pre := readInts(t, base+".pre")[0]
	inputs := readInts(t, base+".input")
	lemmings := map[int][][]int{}
	for _, r := range readInts(t, base) {
		lemmings[r[0]] = append(lemmings[r[0]], r)
	}

	f := testFlow(t)
	f.sess.level = 12
	l, err := level.LoadPlay(f.disc, 12)
	if err != nil {
		t.Fatal(err)
	}
	f.lvl = l
	f.enter(preLevel)
	f.mx, f.my = float64(pre[0]), float64(pre[1])
	run(t, f, input{}, 40)
	run(t, f, input{left: true}, 1)
	until(t, f, input{}, playing, 200)

	px, py := pre[0], pre[1]
	for _, r := range inputs {
		u, x, y, buttons, scroll, chosen := r[0], r[1], r[2], r[3], r[4], r[5]
		in := input{dx: float64(x - px), dy: float64(y - py), left: buttons&1 != 0}
		px, py = x, y
		switch x {
		case 0:
			in.dx -= 50
		case screenW - 1:
			in.dx += 50
		}
		switch y {
		case 0:
			in.dy -= 50
		case 206:
			in.dy += 50
		}
		run(t, f, in, 1)
		in.dx, in.dy = 0, 0
		run(t, f, in, 2)

		p := f.play
		if p.sim.Update != u {
			t.Fatalf("update %d: sim at update %d", u, p.sim.Update)
		}
		if int(f.mx) != x || int(f.my) != y {
			t.Errorf("update %d: pointer (%v, %v), trace (%d, %d)", u, f.mx, f.my, x, y)
		}
		if p.scroll != scroll || p.sim.Input.X != x+scroll {
			t.Errorf("update %d: view %d, sim's pointer column %d; trace view %d", u, p.scroll, p.sim.Input.X, scroll)
		}
		if c, _ := p.sim.Chosen(); c+1 != chosen {
			t.Errorf("update %d: chosen %d, trace %d", u, c, chosen-1)
		}
		if n := len(lemmings[u]); n != len(p.sim.Lemmings) {
			t.Fatalf("update %d: %d lemmings, trace %d", u, len(p.sim.Lemmings), n)
		}
		for _, w := range lemmings[u] {
			m := p.sim.Lemmings[w[1]]
			removed := 0
			if m.Removed {
				removed = 1
			}
			if got := [6]int{m.X, m.Y, m.Dir, m.Frame, m.State, removed}; got != [6]int(w[2:8]) {
				t.Fatalf("update %d lemming %d: %v, trace %v", u, w[1], got, w[2:8])
			}
		}
	}
}

// TestFuzzScroll plays each fuzz recording whose .csv.input has pass columns through the play
// update path, one update per row with the row's pointer, buttons and keys (Escape as a press), and
// compares per row the view, the volume setting, the music volume and the speaker toggle, and, at
// the updates its .panel file lists, the status line's type name as the original displayed it.
func TestFuzzScroll(t *testing.T) {
	dir := os.Getenv("OPENLEMM_FIXTURES")
	files, _ := filepath.Glob(filepath.Join(dir, "fuzz", "play*-*.csv.input"))
	if dir == "" || len(files) == 0 {
		t.Skip("no fuzz recordings")
	}
	f := testFlow(t)
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(b), "update,x,y,buttons,scroll,chosen,cx,cy,pass,paused,keys,setting,music,speaker\n") {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(file), ".csv.input")
		t.Run(name, func(t *testing.T) {
			n, _, _ := strings.Cut(strings.TrimPrefix(name, "play"), "-")
			play, err := strconv.Atoi(n)
			if err != nil {
				t.Fatalf("%s: no play index", name)
			}
			l, err := level.LoadPlay(f.disc, play)
			if err != nil {
				t.Fatal(err)
			}
			p := newPlay(l, f.tables, play)
			names := map[int]string{}
			if b, err := os.ReadFile(filepath.Join(dir, "fuzz", name+".panel")); err == nil {
				for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n")[1:] {
					u, typ, _ := strings.Cut(line, ",")
					n, err := strconv.Atoi(u)
					if err != nil {
						t.Fatalf("%s.panel %q: %v", name, line, err)
					}
					names[n] = typ
				}
			}
			escape := false
			for _, r := range readInts(t, file) {
				u, x, y, buttons, scroll, pass, keys := r[0], r[1], r[2], r[3], r[4], r[8], r[10]
				in := input{left: buttons&1 != 0, right: buttons&2 != 0, f1: keys&1 != 0, f2: keys&2 != 0, f5: keys&8 != 0,
					escape: keys&4 != 0 && !escape}
				escape = keys&4 != 0
				p.update(in, x, y+viewTop, nil, false)
				in.escape = false
				p.update(in, x, y+viewTop, nil, false)
				p.update(in, x, y+viewTop, nil, false)
				speaker := 0
				if p.sim.Quiet {
					speaker = 1
				}
				got := [4]int{p.scroll, p.sim.Volume, p.sim.MusicVolume(), speaker}
				if want := [4]int{scroll, r[11], r[12], r[13]}; p.sim.Update != pass || got != want {
					t.Fatalf("DIVERGE %s update=%d pass=%d (sim pass %d): view, setting, music, speaker %v, trace %v", name, u, pass, p.sim.Update, got, want)
				}
				if typ, ok := names[u]; ok {
					if got := p.panelState().Type; got != typ {
						t.Fatalf("DIVERGE %s update=%d: type name %q, displayed %q", name, u, got, typ)
					}
					delete(names, u)
				}
			}
			if len(names) > 0 {
				t.Fatalf("%s: .panel updates %v not in the input", name, names)
			}
			t.Logf("MATCH %s", name)
		})
	}
}
