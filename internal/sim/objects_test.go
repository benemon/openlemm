package sim

import (
	"testing"
)

// floorGame is a game with a solid floor on map row 50 and a walker at (100, 50) facing right,
// about to step onto block (25, 12), which holds cell c.
func floorGame(c uint8) *Game {
	g := emptyGame()
	for x := range mapW {
		g.terrain[50*mapW+x] = 8
	}
	g.grid[12][25] = c
	g.Lemmings = []Lemming{{X: 100, Y: 50, Dir: 1}}
	return g
}

// TestWaterAndFire checks water and fire: meeting water in update T makes the lemming drowning
// only, frame 0; it moves 1 pixel per update unless the pixel 8 ahead at foot level (row y) is
// solid, and is removed at T + 16. Fire makes it burning only; it does not move and is removed at T
// + 14. Neither counts as saved. Its oracle is that rule.
func TestWaterAndFire(t *testing.T) {
	for _, c := range []struct {
		name   string
		cell   uint8
		wall   bool // a solid pixel 8 ahead of the drowning lemming
		state  int
		life   int // updates from T to removal
		moveTo int // x at T + life - 1
	}{
		{"water", water, false, Drowning, 16, 116},
		{"water, solid 8 ahead", water, true, Drowning, 16, 101},
		{"fire", fire, false, Burning, 14, 101},
	} {
		g := floorGame(c.cell)
		g.Step() // update T: the walker steps to x 101 onto the cell
		// Row y is open ahead, so only the pixel set here stops the drowning lemming.
		for x := 102; x < mapW; x++ {
			g.terrain[50*mapW+x] = 0
		}
		if c.wall {
			g.terrain[50*mapW+109] = 8
		}
		l := &g.Lemmings[0]
		if l.State != c.state || l.Frame != 0 || l.X != 101 {
			t.Fatalf("%s at T: state %#x frame %d x %d; want %#x, 0, 101", c.name, l.State, l.Frame, l.X, c.state)
		}
		for range c.life - 1 {
			g.Step()
		}
		if l.Removed || l.X != c.moveTo {
			t.Errorf("%s at T + %d: removed %v x %d; want in play, x %d", c.name, c.life-1, l.Removed, l.X, c.moveTo)
		}
		g.Step()
		if !l.Removed || g.Saved != 0 {
			t.Errorf("%s at T + %d: removed %v saved %d; want removed, 0 saved", c.name, c.life, l.Removed, g.Saved)
		}
	}
}

// TestSplatOnWater checks splatting with the trigger check table: a fatal landing keeps the falling
// flag, so the lemming cannot exit, while water still acts on it on the landing update. Its oracle
// is that rule.
func TestSplatOnWater(t *testing.T) {
	for _, c := range []struct {
		cell uint8
		want int
	}{{water, Drowning}, {exit, Falling | Splatting}} {
		g := floorGame(c.cell)
		g.Lemmings[0] = Lemming{X: 101, Y: 49, Dir: 1, State: Falling, fall: 61}
		g.Step()
		if l := g.Lemmings[0]; l.State != c.want {
			t.Errorf("fatal landing on cell %d: state %#x, want %#x", c.cell, l.State, c.want)
		}
	}
}

// TestTrap checks traps: an idle trap removes the lemming meeting it at once and is busy for its
// frame count in updates, letting others through; on update T + frame count it is idle again before
// lemmings move and can kill in that update. Its oracle is that rule.
func TestTrap(t *testing.T) {
	const slot, frames = 3, 5
	g := floorGame(trap + 16*slot)
	g.trapLen[slot] = frames
	g.Lemmings = append(g.Lemmings, make([]Lemming, frames)...)
	for i := 1; i <= frames; i++ {
		g.Lemmings[i] = Lemming{X: 100, Y: 50, Dir: 1, Removed: true}
	}
	g.Step() // update T: lemming 0 meets the trap
	if !g.Lemmings[0].Removed || g.Saved != 0 {
		t.Fatalf("at T: lemming 0 removed %v; want removed", g.Lemmings[0].Removed)
	}
	// A new walker steps onto the trap in each update T + i; the trap's 4-pixel-wide cell holds
	// up to 4 of them.
	for i := 1; i <= frames; i++ {
		g.Lemmings[i].Removed = false
		g.Step()
		removed := 0
		for _, l := range g.Lemmings[1 : i+1] {
			if l.Removed {
				removed++
			}
		}
		if want := map[bool]int{false: 0, true: 1}[i == frames]; removed != want {
			t.Errorf("at T + %d: %d more lemmings removed, want %d", i, removed, want)
		}
	}
}

// TestTrapFrame checks the trap animation the viewer draws: frame 0 while idle; started by a
// kill in update T, frame i in update T + i until the trap is idle again at T + frame count.
func TestTrapFrame(t *testing.T) {
	g := floorGame(trap + 16*3)
	g.trapLen[3] = 5
	g.Step()
	for i := range 6 {
		if got, want := g.TrapFrame(3), i%5; got != want {
			t.Errorf("T + %d: frame %d, want %d", i, got, want)
		}
		g.Step()
	}
}
