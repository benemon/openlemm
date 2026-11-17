package sim

import (
	"testing"
)

// TestDigOpenSpaceIsColourZero checks the digger's fall-out: the digger falls out only when the 5
// pixels x-2 … x+2 on its row are colour 0. Its oracle is that rule: colours 1-7 are not solid, but
// they are not open space either.
func TestDigOpenSpaceIsColourZero(t *testing.T) {
	for c := uint8(0); c < 8; c++ {
		g := emptyGame()
		g.Lemmings = []Lemming{{X: 100, Y: 50, Dir: 1, Frame: 7, State: Digging}}
		for x := 98; x <= 102; x++ {
			g.terrain[50*mapW+x] = c
		}
		g.Step()
		l := g.Lemmings[0]
		if wantFall := c == 0; (l.State == Falling) != wantFall || l.Y != 51 {
			t.Errorf("row of colour %d: state %#x y %d; want falling %v, y 51", c, l.State, l.Y, wantFall)
		}
	}
}

// TestDigSteelStop checks that digging onto a steel block ends it; the lemming walks with frame 0,
// facing the same way. Its oracle is that rule.
func TestDigSteelStop(t *testing.T) {
	g := emptyGame()
	for i := range g.terrain {
		g.terrain[i] = 8
	}
	g.grid[51/4][100/4] = steel
	g.Lemmings = []Lemming{{X: 100, Y: 50, Dir: -1, Frame: 7, State: Digging}}
	g.Step()
	if l := g.Lemmings[0]; l.State != 0 || l.Frame != 0 || l.Dir != -1 || l.Y != 51 {
		t.Errorf("state %#x frame %d dir %d y %d; want walking, frame 0, dir -1, y 51", l.State, l.Frame, l.Dir, l.Y)
	}
}
