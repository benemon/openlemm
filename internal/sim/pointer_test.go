package sim

import (
	"image"
	"testing"

	"github.com/benemon/openlemm/internal/level"
)

// emptyGame has an empty map and no tables.
func emptyGame() *Game {
	return New(&level.Level{Map: image.NewPaletted(image.Rect(0, 0, mapW, mapH), nil)}, &Tables{})
}

// TestChoose's oracle is the rule for choosing the lemming: a lemming is under (px, py) when x − 5
// ≤ px ≤ x + 4 and y − 11 ≤ py ≤ y; removed lemmings are never chosen; those only digging,
// building, blocking, bashing, mining or shrugging are preferred, otherwise any; the latest in
// release order wins; the right button chooses from the second group only.
func TestChoose(t *testing.T) {
	walker := Lemming{X: 100, Y: 50, Dir: 1}
	digger := Lemming{X: 100, Y: 50, Dir: 1, State: Digging}
	removed := Lemming{X: 100, Y: 50, Dir: 1, Removed: true}
	faller := Lemming{X: 100, Y: 50, Dir: 1, State: Falling}
	digFall := Lemming{X: 100, Y: 50, Dir: 1, State: Digging | Falling}

	for _, c := range []struct {
		name          string
		ls            []Lemming
		px, py        int
		right         bool
		chosen, count int
	}{
		{"left edge in", []Lemming{walker}, 95, 45, false, 0, 1},
		{"left edge out", []Lemming{walker}, 94, 45, false, -1, 0},
		{"right edge in", []Lemming{walker}, 104, 45, false, 0, 1},
		{"right edge out", []Lemming{walker}, 105, 45, false, -1, 0},
		{"top in", []Lemming{walker}, 100, 39, false, 0, 1},
		{"top out", []Lemming{walker}, 100, 38, false, -1, 0},
		{"feet in", []Lemming{walker}, 100, 50, false, 0, 1},
		{"below feet out", []Lemming{walker}, 100, 51, false, -1, 0},
		{"removed never", []Lemming{walker, removed}, 100, 45, false, 0, 1},
		{"latest of the rest", []Lemming{walker, faller}, 100, 45, false, 1, 2},
		{"working preferred", []Lemming{digger, walker}, 100, 45, false, 0, 2},
		{"right skips working", []Lemming{digger, walker}, 100, 45, true, 1, 2},
		{"right, only working", []Lemming{digger}, 100, 45, true, -1, 1},
		{"digging and falling is not working", []Lemming{digFall, walker}, 100, 45, false, 1, 2},
		{"latest working", []Lemming{digger, walker, digger}, 100, 45, false, 2, 3},
	} {
		g := emptyGame()
		g.Lemmings = c.ls
		g.read = Input{X: c.px, Y: c.py, Right: c.right, Button: -1}
		g.choose()
		if chosen, count := g.Chosen(); chosen != c.chosen || count != c.count {
			t.Errorf("%s: chosen %d, count %d; want %d, %d", c.name, chosen, count, c.chosen, c.count)
		}
	}
}

// clickGame has a solid floor on row 50 and the given lemmings standing on it at x 100.
func clickGame(ls ...Lemming) *Game {
	g := emptyGame()
	for x := range mapW {
		g.terrain[50*mapW+x] = 8
	}
	g.Lemmings = append([]Lemming(nil), ls...)
	g.Skills = [8]int{5, 5, 5, 5, 5, 5, 5, 5}
	return g
}

// steps runs the game with the pointer on (100, 45) over the given panel button, or over the
// playfield when it is -1, and the left button as given per update.
func steps(g *Game, button int, left ...bool) {
	for _, down := range left {
		g.Input = Input{X: 100, Y: 45, Playfield: button < 0, Button: button, Left: down}
		g.Step()
	}
}

// TestClick's oracle is the pointer and selection rule and the refusal table: a press on a skill
// button selects it, even with none left; a fresh press over the playfield assigns the selected
// skill to the lemming chosen at the end of the previous update, and latches even with none chosen;
// holding the button does not assign again until it has been up for an update; a refusal uses
// nothing.
func TestClick(t *testing.T) {
	g := clickGame(Lemming{X: 100, Y: 50, Dir: 1})
	steps(g, int(Floater)+2, true, false)
	if g.Selected() != Floater {
		t.Fatalf("selected %d after a press on button %d", g.Selected(), Floater)
	}
	steps(g, -1, false) // the lemming is chosen at the end of this update
	steps(g, -1, true)
	if !g.Lemmings[0].Floater() || g.Skills[Floater] != 4 {
		t.Errorf("click: floater %v, floaters left %d; want true, 4", g.Lemmings[0].Floater(), g.Skills[Floater])
	}

	// A refused floater (already one) uses nothing.
	steps(g, -1, false, true)
	if g.Skills[Floater] != 4 {
		t.Errorf("refused click: floaters left %d, want 4", g.Skills[Floater])
	}

	// A held button assigns once.
	g = clickGame(Lemming{X: 100, Y: 50, Dir: 1})
	steps(g, int(Bomber)+2, true, false)
	steps(g, -1, false, true, true, true)
	if g.Skills[Bomber] != 4 {
		t.Errorf("held button: bombers left %d, want 4", g.Skills[Bomber])
	}
	// A press started on a skill button and dragged onto the playfield is already latched.
	g = clickGame(Lemming{X: 100, Y: 50, Dir: 1})
	steps(g, int(Digger)+2, true)
	steps(g, -1, true, true)
	if g.Skills[Digger] != 5 {
		t.Errorf("press dragged from a skill button: diggers left %d, want 5", g.Skills[Digger])
	}
	// A playfield press with no lemming under the pointer latches: dragged onto one, it assigns
	// nothing.
	g = clickGame(Lemming{X: 100, Y: 50, Dir: 1})
	steps(g, int(Digger)+2, true, false)
	g.Input = Input{X: 300, Y: 45, Playfield: true, Button: -1}
	g.Step()
	g.Input.Left = true
	g.Step()
	steps(g, -1, true, true)
	if g.Skills[Digger] != 5 {
		t.Errorf("press dragged onto a lemming: diggers left %d, want 5", g.Skills[Digger])
	}
	// A skill button with none left still selects and latches.
	g = clickGame(Lemming{X: 100, Y: 50, Dir: 1})
	g.Skills[Miner] = 0
	steps(g, int(Digger)+2, true, false)
	steps(g, int(Miner)+2, true)
	steps(g, -1, true, true)
	if g.Selected() != Miner || g.Skills[Digger] != 5 {
		t.Errorf("press on an empty skill: selected %d, diggers left %d; want %d, 5", g.Selected(), g.Skills[Digger], Miner)
	}
	// A press off the playfield and not on a skill button assigns nothing and is not latched.
	g = clickGame(Lemming{X: 100, Y: 50, Dir: 1})
	steps(g, int(Digger)+2, true, false)
	g.Input = Input{X: 100, Y: 45, Button: -1, Left: true}
	g.Step()
	g.Step()
	if g.Skills[Digger] != 5 {
		t.Errorf("press off the playfield: diggers left %d, want 5", g.Skills[Digger])
	}
	steps(g, -1, true)
	if g.Skills[Digger] != 4 {
		t.Errorf("press dragged from the panel: diggers left %d, want 4", g.Skills[Digger])
	}
}

// TestClickRetry's oracle is the refusal table: on a behaviour refusal of a digger, basher, miner
// or builder the click retries once with the latest lemming under the pointer that is not only
// working; the "also refused" tests (steel, one-way, y ≤ 8) never retry; when the retry lemming is
// the one refused it fails the same way.
func TestClickRetry(t *testing.T) {
	for _, c := range []struct {
		name      string
		ls        []Lemming
		skill     Skill
		left      int
		got, want int // lemming checked, and its state wanted
	}{
		{"builder refused by building, retries on the walker",
			[]Lemming{{X: 100, Y: 50, Dir: 1}, {X: 100, Y: 50, Dir: 1, State: Building}},
			Builder, 4, 0, Building},
		{"no retry for the steel refusal",
			[]Lemming{{X: 100, Y: 50, Dir: 1}, {X: 100, Y: 50, Dir: 1, State: Shrugging, own: steel}},
			Digger, 5, 0, 0},
		{"no retry for floaters",
			[]Lemming{{X: 100, Y: 50, Dir: 1}, {X: 100, Y: 50, Dir: 1, State: Falling, floater: true}},
			Floater, 5, 0, 0},
		{"retry on the same lemming fails",
			[]Lemming{{X: 100, Y: 50, Dir: 1, State: Falling}},
			Digger, 5, 0, Falling},
	} {
		g := clickGame()
		steps(g, int(c.skill)+2, true, false)
		g.Lemmings = append([]Lemming(nil), c.ls...)
		g.read = Input{X: 100, Y: 45, Playfield: true, Button: -1}
		g.choose()
		g.Input = g.read
		g.Input.Left = true
		g.pointer()
		if g.Skills[c.skill] != c.left || g.Lemmings[c.got].State != c.want {
			t.Errorf("%s: %d left, lemming %d state %#x; want %d, %#x",
				c.name, g.Skills[c.skill], c.got, g.Lemmings[c.got].State, c.left, c.want)
		}
	}
}
