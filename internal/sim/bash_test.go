package sim

import (
	"testing"
)

// TestBashStep checks one basher step (frame 10 → 11): the edge turn, the ground search, the
// fall-out and the steel and one-way stops at block column x ÷ 4 + 2d, block row y ÷ 4 − 2; and
// that frame 10 does not step. Its oracle is that rule; no traced basher steps.
func TestBashStep(t *testing.T) {
	for _, c := range []struct {
		name      string
		x, d      int
		ground    int // row of the one solid pixel under the new x, or -1
		cell      uint8
		x2, y, d2 int
		state     int
	}{
		{"right edge", 1607, 1, 50, 0, 1608, 50, -1, 0},
		{"left edge", 16, -1, 50, 0, 15, 50, 1, 0},
		{"level", 100, 1, 50, 0, 101, 50, 1, Bashing},
		{"down 2", 100, 1, 52, 0, 101, 52, 1, Bashing},
		{"fall", 100, 1, -1, 0, 101, 53, 1, Falling},
		{"steel", 100, 1, 50, steel, 101, 50, -1, 0},
		{"leftward one-way facing right", 100, 1, 50, oneWayLeft, 101, 50, -1, 0},
		{"leftward one-way facing left", 100, -1, 50, oneWayLeft, 99, 50, -1, Bashing},
		{"rightward one-way facing left", 100, -1, 50, oneWayRight, 99, 50, 1, 0},
		{"rightward one-way facing right", 100, 1, 50, oneWayRight, 101, 50, 1, Bashing},
	} {
		g := emptyGame()
		nx := c.x + c.d
		if c.ground >= 0 {
			g.terrain[c.ground*mapW+nx] = 8
		}
		g.grid[50/4-2][nx/4+2*c.d] = c.cell
		g.Lemmings = []Lemming{{X: c.x, Y: 50, Dir: c.d, Frame: 10, State: Bashing, fall: 30}}
		g.Step()
		l := g.Lemmings[0]
		if l.X != c.x2 || l.Y != c.y || l.Dir != c.d2 || l.State != c.state {
			t.Errorf("%s: x %d y %d dir %d state %#x; want %d, %d, %d, %#x", c.name, l.X, l.Y, l.Dir, l.State, c.x2, c.y, c.d2, c.state)
		}
		if c.state == Falling && (l.Frame != 0 || l.fall != 0) {
			t.Errorf("%s: frame %d fall count %d; want 0, 0", c.name, l.Frame, l.fall)
		}
	}

	g := emptyGame()
	g.Lemmings = []Lemming{{X: 100, Y: 50, Dir: 1, Frame: 9, State: Bashing}}
	g.Step()
	if l := g.Lemmings[0]; l.X != 100 || l.State != Bashing {
		t.Errorf("frame 10: x %d state %#x; want 100, bashing (no step)", l.X, l.State)
	}
}
