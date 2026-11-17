package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/benemon/openlemm/internal/level"
)

// readInts reads a fixture CSV after its header line as integers.
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
