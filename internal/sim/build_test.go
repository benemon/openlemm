package sim

import (
	"image"
	"testing"
)

// TestBuildStep checks the builder's step (frame 15 → 0) against its step table, one case per row,
// from x 100, y 50 facing right unless stated. Its oracle is that table; the traced builders all
// lay 12 bricks and shrug.
func TestBuildStep(t *testing.T) {
	for _, c := range []struct {
		name       string
		x, y, d    int
		bricks     int
		solid      image.Point // one solid pixel, or (0, 0) for none
		x2, y2, d2 int
		state      int
	}{
		{"1: right edge", 1607, 50, 1, 5, image.Point{}, 1608, 49, -1, 0},
		{"1: left edge", 17, 50, -1, 5, image.Point{}, 16, 49, 1, 0},
		{"2: wall at first step", 100, 50, 1, 5, image.Pt(101, 48), 101, 49, -1, 0},
		{"3: wall at second step", 100, 50, 1, 5, image.Pt(102, 48), 102, 49, -1, 0},
		{"4: last brick", 100, 50, 1, 1, image.Pt(104, 40), 102, 49, 1, Shrugging},
		{"5: head", 100, 50, 1, 5, image.Pt(104, 40), 102, 49, -1, 0},
		{"5: head facing left", 100, 50, -1, 5, image.Pt(96, 40), 98, 49, 1, 0},
		{"6: edge after second step", 1606, 50, 1, 5, image.Point{}, 1608, 49, -1, 0},
		{"7: y < 9", 100, 9, 1, 5, image.Point{}, 102, 8, 1, 0},
		{"held at row 8", 100, 8, 1, 5, image.Point{}, 102, 8, -1, 0},
		{"keeps building", 100, 50, 1, 5, image.Point{}, 102, 49, 1, Building},
	} {
		g := emptyGame()
		if c.solid != (image.Point{}) {
			g.terrain[c.solid.Y*mapW+c.solid.X] = 8
		}
		g.Lemmings = []Lemming{{X: c.x, Y: c.y, Dir: c.d, Frame: 15, State: Building, bricks: c.bricks}}
		g.Step()
		l := g.Lemmings[0]
		if l.X != c.x2 || l.Y != c.y2 || l.Dir != c.d2 || l.State != c.state {
			t.Errorf("%s: x %d y %d dir %d state %#x; want %d, %d, %d, %#x", c.name, l.X, l.Y, l.Dir, l.State, c.x2, c.y2, c.d2, c.state)
		}
	}
}

// TestBrickOrsColour checks the builder's brick: each of its 6 pixels on row y − 1 (x … x + 5
// facing right) is OR-ed with 8, so colours 1-7 become 9-15. Its oracle is that rule; the traced
// bricks only overlap colours 0 and 8-15.
func TestBrickOrsColour(t *testing.T) {
	g := emptyGame()
	in := [6]uint8{0, 1, 5, 7, 8, 14}
	copy(g.terrain[49*mapW+100:], in[:])
	g.Lemmings = []Lemming{{X: 100, Y: 50, Dir: 1, Frame: 8, State: Building, bricks: 12}}
	g.Step()
	for i, c := range in {
		if got := g.terrain[49*mapW+100+i]; got != c|8 {
			t.Errorf("x %d: colour %d became %d, want %d", 100+i, c, got, c|8)
		}
	}
}
