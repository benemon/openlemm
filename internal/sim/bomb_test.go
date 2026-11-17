package sim

import (
	"testing"
)

// solidGame is a game whose whole map is colour 8, with all-zero masks: a crater clears its whole
// 16 × 22 rectangle.
func solidGame() *Game {
	g := emptyGame()
	for i := range g.terrain {
		g.terrain[i] = 8
	}
	return g
}

// TestCrater checks the bomber's crater: it is made in the update after the lemming becomes
// exploding, not before; its top-left is (x − 8, y − 14), moved to stay inside the map; none is
// made when the cached cell under the lemming is steel, exit or water; and of two lemmings with
// their first exploding update in one update only the later one makes a crater. Its oracle is
// that rule; the traced craters are all well inside the map, on cells of effect 0, one per update.
func TestCrater(t *testing.T) {
	g := solidGame()
	g.Lemmings = []Lemming{{X: 2, Y: 5, Dir: 1, Frame: 15, State: OhNo}}
	g.Step()
	if !g.solid(2, 0) {
		t.Errorf("crater made in the update the lemming became exploding")
	}
	g.Step()
	if g.solid(15, 21) || g.solid(0, 0) || !g.solid(16, 0) || !g.solid(0, 22) {
		t.Errorf("crater at (2, 5): not the rectangle (0, 0)-(16, 22)")
	}

	for _, c := range []uint8{steel, exit, water} {
		g := solidGame()
		g.Lemmings = []Lemming{{X: 100, Y: 50, Dir: 1, Frame: 15, State: OhNo, own: c}}
		g.Step()
		g.Step()
		if !g.solid(100, 40) {
			t.Errorf("own cell %d: crater made", c)
		}
	}

	g = solidGame()
	g.Lemmings = []Lemming{
		{X: 100, Y: 50, Dir: 1, Frame: 15, State: OhNo},
		{X: 200, Y: 50, Dir: 1, Frame: 15, State: OhNo},
	}
	g.Step()
	g.Step()
	if !g.solid(100, 40) || g.solid(200, 40) {
		t.Errorf("two explosions: crater under lemming 0 %v, lemming 1 %v; want only lemming 1",
			!g.solid(100, 40), !g.solid(200, 40))
	}
}

// TestFuseRunsOut checks that when the fuse runs out a falling, floating or burning lemming gets
// exploding added without moving and makes a crater in the next update; a drowning one gets exploding added too but drowning outranks
// it, so it makes no crater and is removed 16 updates later; any other, exiting included, gets "Oh
// no!" added. Every flag is kept. Its oracle is that rule.
func TestFuseRunsOut(t *testing.T) {
	for _, c := range []struct {
		state, want int
		crater      bool
	}{
		{0, OhNo, false},
		{Exiting, OhNo | Exiting, false},
		{Building, OhNo | Building, false},
		{Falling, Falling | Exploding, true},
		{Floating, Floating | Exploding, true},
		{Burning, Burning | Exploding, true},
		{Drowning, Drowning | Exploding, false},
	} {
		g := solidGame()
		g.Lemmings = []Lemming{{X: 100, Y: 50, Dir: 1, State: c.state, Frame: 5, fuse: 1}}
		g.Step()
		l := &g.Lemmings[0]
		if l.State != c.want || l.Frame != 0 || l.X != 100 || l.Y != 50 {
			t.Errorf("state %#x: became %#x frame %d at (%d, %d); want %#x frame 0 at (100, 50)",
				c.state, l.State, l.Frame, l.X, l.Y, c.want)
		}
		if !g.solid(100, 40) {
			t.Errorf("state %#x: crater in the update the fuse ran out", c.state)
		}
		g.Step()
		if crater := !g.solid(100, 40); crater != c.crater {
			t.Errorf("state %#x: crater %v, want %v", c.state, crater, c.crater)
		}
		if c.state == Drowning {
			for range 14 {
				g.Step()
			}
			if l.Removed {
				t.Errorf("drowning: removed 15 updates after the fuse ran out")
			}
			g.Step()
			if !l.Removed || l.State&Drowning == 0 {
				t.Errorf("drowning: removed %v state %#x 16 updates after; want removed while drowning", l.Removed, l.State)
			}
		}
	}
}

// TestOhNoKeepsFlags checks that a kept flag's behaviour never runs again: through 16 "Oh no!"
// updates a builder lays no brick and an exiting lemming is not saved, and on the 16th "Oh no!" is
// swapped for exploding with the flag still kept. Its oracle is that rule.
func TestOhNoKeepsFlags(t *testing.T) {
	for _, state := range []int{Building, Exiting} {
		g := emptyGame()
		for x := range mapW {
			g.terrain[50*mapW+x] = 8
		}
		g.Lemmings = []Lemming{{X: 100, Y: 50, Dir: 1, State: state, Frame: 5, fuse: 1, bricks: 12}}
		for range 17 {
			g.Step()
		}
		l := g.Lemmings[0]
		if l.State != state|Exploding || l.Frame != 0 || l.X != 100 || l.Y != 50 || l.Removed || g.Saved != 0 {
			t.Errorf("state %#x: %#x frame %d at (%d, %d), removed %v, saved %d; want %#x frame 0 at (100, 50)",
				state, l.State, l.Frame, l.X, l.Y, l.Removed, g.Saved, state|Exploding)
		}
		for i, c := range g.terrain[:50*mapW] {
			if c != 0 {
				t.Fatalf("state %#x: pixel (%d, %d) drawn", state, i%mapW, i/mapW)
			}
		}
	}
}

// TestOhNoDrop checks the "Oh no!" update: on solid ground it stays without storing its position;
// otherwise it drops onto ground 1 or 2 rows down, or 3 rows, and its trigger is checked, so
// dropping onto an exit cell starts exiting. On the 16th update it explodes without moving. Its
// oracle is that rule.
func TestOhNoDrop(t *testing.T) {
	for _, c := range []struct {
		ground, y int // the one solid row under x 100, or -1
	}{{50, 50}, {51, 51}, {52, 52}, {-1, 53}} {
		g := emptyGame()
		if c.ground >= 0 {
			g.terrain[c.ground*mapW+100] = 8
		}
		g.grid[50/4][100/4] = exit
		g.grid[51/4][100/4] = exit
		g.grid[52/4][100/4] = exit
		g.grid[53/4][100/4] = exit
		g.Lemmings = []Lemming{{X: 100, Y: 50, Dir: 1, State: OhNo}}
		g.Step()
		l := g.Lemmings[0]
		wantState := Exiting
		if c.y == 50 {
			wantState = OhNo // its position is not stored, so the exit is not checked
		}
		if l.Y != c.y || l.State != wantState {
			t.Errorf("ground row %d: y %d state %#x; want %d, %#x", c.ground, l.Y, l.State, c.y, wantState)
		}
	}

	g := emptyGame()
	g.Lemmings = []Lemming{{X: 100, Y: 50, Dir: 1, State: OhNo, Frame: 15}}
	g.Step()
	if l := g.Lemmings[0]; l.State != Exploding || l.Y != 50 {
		t.Errorf("16th update: state %#x y %d; want exploding at y 50", l.State, l.Y)
	}
}
