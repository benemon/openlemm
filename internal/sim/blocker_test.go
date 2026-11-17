package sim

import (
	"image"
	"testing"
)

// fieldAt returns the 3 × 3 cells with top-left block (c, r), row by row.
func fieldAt(g *Game, c, r int) (f [9]uint8) {
	for k := range f {
		f[k] = g.grid[r+k/3][c+k%3]
	}
	return
}

var blockField = [9]uint8{2, 10, 3, 2, 10, 3, 2, 10, 3}

// TestBlockerAssign's oracle is the blocker rule and the refusal table: the field is the 3 × 3
// blocks from ((x − 4) ÷ 4, (y − 6) ÷ 4), each row written 2, 10, 3 over whatever was there; it is
// refused when a cell holds 1, 2, 3 or 10 (steel does not refuse) or the lemming has a behaviour
// other than digging, building, bashing, mining or shrugging; the blocker faces right.
func TestBlockerAssign(t *testing.T) {
	g := clickGame(Lemming{X: 101, Y: 50, Dir: -1, State: Digging})
	g.grid[11][24] = steel
	if !g.Assign(0, Blocker) {
		t.Fatalf("blocker refused")
	}
	l := g.Lemmings[0]
	if f := fieldAt(g, 24, 11); f != blockField || l.Dir != 1 || l.State != Blocking || l.Frame != 0 || g.Skills[Blocker] != 4 {
		t.Errorf("field %v, dir %d, state %#x, frame %d, %d left", f, l.Dir, l.State, l.Frame, g.Skills[Blocker])
	}
	if l.saved[0] != steel {
		t.Errorf("saved %v, want steel first", l.saved)
	}

	for _, c := range []struct {
		name string
		cell uint8
		l    Lemming
	}{
		{"exit", exit + 16*3, Lemming{X: 100, Y: 50}},
		{"left column", fieldLeft, Lemming{X: 100, Y: 50}},
		{"right column", fieldRight, Lemming{X: 100, Y: 50}},
		{"centre column", fieldCentre, Lemming{X: 100, Y: 50}},
		{"falling", 0, Lemming{X: 100, Y: 50, State: Falling}},
		{"blocking", 0, Lemming{X: 100, Y: 50, State: Blocking}},
	} {
		g := clickGame(c.l)
		g.grid[13][26] = c.cell
		if g.Assign(0, Blocker) || g.Skills[Blocker] != 5 || g.grid[11][24] != 0 {
			t.Errorf("%s: blocker accepted", c.name)
		}
	}
}

// TestBlockerFalls's oracle is the blocker rule: a blocker on solid ground stands still, its frame
// counting; one that loses its ground becomes a walker at frame 0 where it stands, its saved cells
// put back and its position not stored, so the water cell restored under its feet does not act that
// update.
func TestBlockerFalls(t *testing.T) {
	g := clickGame(Lemming{X: 100, Y: 50, Dir: 1})
	g.grid[12][25] = water
	g.Assign(0, Blocker)
	g.Step()
	if l := g.Lemmings[0]; l.X != 100 || l.Y != 50 || l.State != Blocking || l.Frame != 1 {
		t.Fatalf("standing blocker: %+v", l)
	}
	g.terrain[50*mapW+100] = 0
	g.Step()
	want := [9]uint8{4: water}
	if l := g.Lemmings[0]; l.State != 0 || l.Frame != 0 || l.X != 100 || l.Y != 50 || fieldAt(g, 24, 11) != want {
		t.Errorf("blocker without ground: %+v, field %v; want walking at frame 0, %v", l, fieldAt(g, 24, 11), want)
	}
}

// TestBlockerExplodes's oracle is the blocker rule and the trigger table: a blocker given a bomber
// keeps its field through "Oh no!"; dropping onto its own centre column it turns and moves 4
// pixels; in its first exploding update, the one after it becomes exploding, the saved cells are
// written at the field of its current position, so the old field stays.
func TestBlockerExplodes(t *testing.T) {
	g := clickGame(Lemming{X: 100, Y: 50, Dir: 1})
	g.fill(image.Rect(24, 11, 27, 14), steel)
	g.Assign(0, Blocker)
	g.Assign(0, Bomber)
	for g.Lemmings[0].State&OhNo == 0 {
		g.Step()
	}
	for x := range mapW {
		g.terrain[50*mapW+x] = 0
		g.terrain[58*mapW+x] = 8
	}
	for g.Lemmings[0].State&Exploding == 0 {
		g.Step()
	}
	g.Step()
	if l := g.Lemmings[0]; l.X != 96 || l.Y != 58 {
		t.Fatalf("exploded at (%d, %d), want (96, 58)", l.X, l.Y)
	}
	old, now := fieldAt(g, 24, 11), fieldAt(g, 23, 13)
	if [6]uint8(old[:6]) != [6]uint8(blockField[:6]) || now != [9]uint8{9, 9, 9, 9, 9, 9, 9, 9, 9} {
		t.Errorf("after exploding: blocks (24-26, 11-13) %v, (23-25, 13-15) %v", old, now)
	}
}

// TestBlockerTurns's oracle is the trigger-check table: cell 2 turns a lemming facing right, 3 one
// facing left; 10 turns it and moves it 4 pixels, and back again if the pixel 8 or 2 rows above its
// feet is solid there.
func TestBlockerTurns(t *testing.T) {
	for _, c := range []struct {
		name         string
		cell         uint8
		dir          int
		wall         int // row above the feet made solid at x 96, or 0
		wantX, wantD int
	}{
		{"left column facing right", fieldLeft, 1, 0, 100, -1},
		{"left column facing left", fieldLeft, -1, 0, 100, -1},
		{"right column facing left", fieldRight, -1, 0, 100, 1},
		{"right column facing right", fieldRight, 1, 0, 100, 1},
		{"centre", fieldCentre, 1, 0, 96, -1},
		{"centre, solid 8 above", fieldCentre, 1, 42, 100, 1},
		{"centre, solid 2 above", fieldCentre, 1, 48, 100, 1},
		{"centre, solid 5 above", fieldCentre, 1, 45, 96, -1},
	} {
		g := clickGame()
		g.grid[50/4][100/4] = c.cell
		if c.wall > 0 {
			g.terrain[c.wall*mapW+96] = 8
		}
		l := Lemming{X: 100, Y: 50, Dir: c.dir, State: Falling}
		g.trigger(&l)
		if l.X != c.wantX || l.Dir != c.wantD || l.State != Falling {
			t.Errorf("%s: x %d, dir %d, state %#x; want %d, %d, falling", c.name, l.X, l.Dir, l.State, c.wantX, c.wantD)
		}
	}
}

// TestStrayWrite's oracle is the original game's stray position write bug: a lemming starting to
// exit writes its x into the record two places later, over the cached ahead cell and the saved top
// row of a blocker field (x mod 256, x ÷ 256, 0, 0); that blocker's removal then writes x ÷ 256, 0,
// 0 into the field's top row.
func TestStrayWrite(t *testing.T) {
	g := clickGame(Lemming{X: 600, Y: 50, Dir: 1}, Lemming{X: 900, Y: 50, Dir: 1}, Lemming{X: 100, Y: 50, Dir: 1})
	g.fill(image.Rect(24, 11, 27, 14), steel)
	g.Assign(2, Blocker)
	g.grid[50/4][601/4] = exit
	g.Step()
	b := g.Lemmings[2]
	if g.Lemmings[0].State != Exiting || b.ahead != 601&255 || [3]uint8(b.saved[:3]) != [3]uint8{601 >> 8, 0, 0} {
		t.Fatalf("after the exit: lemming 0 state %#x; blocker ahead %d, saved %v", g.Lemmings[0].State, b.ahead, b.saved)
	}
	g.terrain[50*mapW+100] = 0
	g.Step()
	if f := fieldAt(g, 24, 11); f != [9]uint8{2, 0, 0, 9, 9, 9, 9, 9, 9} {
		t.Errorf("field after removal %v", f)
	}
}

// TestStrayOhNo's oracle is the original game's stray position write bug: the "Oh no!" write
// happens at the lemming's "Oh no!" frame 5, only while the nuke is not active; a lemming whose
// fuse runs out while falling never enters "Oh no!" and writes nothing.
func TestStrayOhNo(t *testing.T) {
	for _, c := range []struct {
		name  string
		l     Lemming
		nuke  bool
		write bool
	}{
		{"oh no frame 5", Lemming{X: 300, Y: 50, Dir: 1, State: OhNo, Frame: 4}, false, true},
		{"oh no frame 4", Lemming{X: 300, Y: 50, Dir: 1, State: OhNo, Frame: 3}, false, false},
		{"oh no, nuked", Lemming{X: 300, Y: 50, Dir: 1, State: OhNo, Frame: 4}, true, false},
		{"fuse out falling", Lemming{X: 300, Y: 40, Dir: 1, State: Falling, fuse: 1}, false, false},
	} {
		g := clickGame(c.l, Lemming{X: 900, Y: 50, Dir: 1}, Lemming{X: 1200, Y: 50, Dir: 1, State: Blocking})
		g.nuke, g.nukeAt = c.nuke, 99
		g.Step()
		if got := g.Lemmings[2].ahead == 300&255; got != c.write {
			t.Errorf("%s: wrote %v, want %v (ahead %d)", c.name, got, c.write, g.Lemmings[2].ahead)
		}
	}
}
