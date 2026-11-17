package sim

import (
	"image"
	"testing"
)

// TestMinerOneWay checks the miner's steel check for a miner in solid terrain: a leftward-only
// one-way area (effect 7) stops a right-facing miner at its first forward move (frame 3), turned
// round and walking, and lets a left-facing one through; a rightward-only one (effect 8) never
// stops it either way round, the original-game bug. Its oracle is that rule; no traced level has a
// one-way area yet.
func TestMinerOneWay(t *testing.T) {
	for _, c := range []struct {
		effect           uint8
		dir, state, x, d int
	}{
		{oneWayLeft, 1, 0, 102, -1},
		{oneWayLeft, -1, Mining, 92, -1},
		{oneWayRight, -1, Mining, 92, -1},
		{oneWayRight, 1, Mining, 108, 1},
	} {
		g := emptyGame()
		for i := range g.terrain {
			g.terrain[i] = 8
		}
		g.fill(image.Rect(0, 0, gridW, gridH), c.effect)
		g.Lemmings = []Lemming{{X: 100, Y: 50, Dir: c.dir, State: Mining}}
		for range 48 {
			if g.Lemmings[0].State != Mining {
				break
			}
			g.Step()
		}
		if l := g.Lemmings[0]; l.State != c.state || l.X != c.x || l.Dir != c.d {
			t.Errorf("effect %d facing %d: state %#x x %d dir %d; want %#x, %d, %d", c.effect, c.dir, l.State, l.X, l.Dir, c.state, c.x, c.d)
		}
	}
}

// TestMinerEdge checks the miner at the map edge: each 1-pixel step of frames 3 and 15 fails at x <
// 16 or x ≥ 1608, before frame 3's y + 1; the miner turns round and walks with frame 0, keeping the
// x reached. Its oracle is that rule.
func TestMinerEdge(t *testing.T) {
	for _, c := range []struct{ x, d, frame, x2 int }{
		{17, -1, 2, 15},
		{1606, 1, 2, 1608},
		{17, -1, 14, 15},
		{1607, 1, 14, 1608},
	} {
		g := solidGame()
		g.Lemmings = []Lemming{{X: c.x, Y: 50, Dir: c.d, Frame: c.frame, State: Mining}}
		g.Step()
		if l := g.Lemmings[0]; l.X != c.x2 || l.Y != 50 || l.Dir != -c.d || l.State != 0 || l.Frame != 0 {
			t.Errorf("x %d dir %d frame %d: x %d y %d dir %d state %#x frame %d; want x %d y 50 dir %d walking frame 0",
				c.x, c.d, c.frame, l.X, l.Y, l.Dir, l.State, l.Frame, c.x2, -c.d)
		}
	}
}

// TestStored checks which updates store the position (and so check triggers): mining on frames 0, 3
// and 15; bashing on sub-frames 11-15; climbing every update; hoisting on frames 1-4 and 0. Its
// oracle is that rule.
func TestStored(t *testing.T) {
	for _, c := range []struct {
		state int
		from  int // frame before the update
		want  bool
	}{
		{Mining, 23, true}, {Mining, 0, false}, {Mining, 1, false}, {Mining, 2, true},
		{Mining, 3, false}, {Mining, 14, true}, {Mining, 15, false},
		{Bashing, 0, false}, {Bashing, 1, false}, {Bashing, 4, false}, {Bashing, 10, true},
		{Bashing, 14, true}, {Bashing, 15, false}, {Bashing, 26, true}, {Bashing, 31, false},
		{Climbing, 0, true}, {Climbing, 4, true},
		{Hoisting, 0, true}, {Hoisting, 3, true}, {Hoisting, 4, false}, {Hoisting, 6, false},
		{Hoisting, 7, true},
	} {
		g := solidGame()
		// Open a corridor so climbing, hoisting and bashing move without meeting terrain.
		for y := 0; y < 50; y++ {
			for x := 80; x < 130; x++ {
				g.terrain[y*mapW+x] = 0
			}
		}
		l := &Lemming{X: 100, Y: 50, Dir: 1, Frame: c.from, State: c.state}
		if got := g.move(l); got != c.want {
			t.Errorf("state %#x frame %d → %d: stored %v, want %v", c.state, c.from, l.Frame, got, c.want)
		}
	}
}
