package sim

import (
	"image"
	"testing"

	"github.com/benemon/openlemm/internal/level"
)

// doorGame is a 5-minute level at release rate 50 with one trapdoor and n lemmings.
func doorGame(n int) *Game {
	return New(&level.Level{
		Map:         image.NewPaletted(image.Rect(0, 0, mapW, mapH), nil),
		Minutes:     5,
		ReleaseRate: 50,
		Lemmings:    n,
		Objects:     []level.Object{{X: 600, Y: 20, Entrance: true, Slot: 0}},
	}, &Tables{})
}

// press runs one update with the left button held over panel button b (-1: the playfield).
func press(g *Game, b int, left bool) {
	g.Input = Input{X: 0, Y: 0, Playfield: b < 0, Button: b, Left: left}
	g.Step()
}

// releases runs g until it has released n lemmings and returns the updates at which each came out.
func releases(g *Game, n int) []int {
	var at []int
	for len(at) < n && g.Update < 1000 {
		press(g, -1, false)
		if len(g.Lemmings) > len(at) {
			at = append(at, g.Update)
		}
	}
	return at
}

// TestRateButtons checks the release rate buttons: each update the left button is held over + or −
// changes the rate by 1, within the level's rate and 99, without latching; a change sets the
// interval to (99 − rate) ÷ 2 + 4 from the next release, leaving the countdown running; paused, the
// buttons do nothing. Its oracle is that rule.
func TestRateButtons(t *testing.T) {
	g := doorGame(3)
	for range 3 {
		press(g, RateUp, true)
	}
	press(g, RateDown, true)
	if lo, r := g.Rate(); lo != 50 || r != 52 {
		t.Errorf("after + + + −: rate %d (minimum %d), want 52 (50)", r, lo)
	}
	if at := releases(g, 2); at[0] != 50 || at[1] != 50+(99-52)/2+4 {
		t.Errorf("released at %v, want [50 %d]", at, 50+(99-52)/2+4)
	}
	for range 5 {
		press(g, RateDown, true)
	}
	if _, r := g.Rate(); r != 50 {
		t.Errorf("held − past the minimum: rate %d, want 50", r)
	}
	g.rate = 98
	press(g, RateUp, true)
	press(g, RateUp, true)
	if _, r := g.Rate(); r != 99 {
		t.Errorf("held + past 99: rate %d, want 99", r)
	}
	// A press held on a rate button is not latched: moved onto a skill button, it selects.
	press(g, int(Miner)+2, true)
	if g.Selected() != Miner {
		t.Errorf("press dragged from a rate button: selected %d, want %d", g.Selected(), Miner)
	}
	// A latched press moved onto a rate button still changes the rate.
	press(g, RateDown, true)
	if _, r := g.Rate(); r != 98 {
		t.Errorf("latched press dragged onto −: rate %d, want 98", r)
	}

	g = doorGame(3)
	press(g, Pause, true)
	press(g, RateUp, false)
	press(g, RateUp, true)
	if _, r := g.Rate(); r != 50 {
		t.Errorf("paused: rate %d, want 50", r)
	}
	if at := releases(g, 1); at != nil {
		t.Errorf("paused: released at %v", at)
	}
}

// TestPause checks the pause button and the update order: a fresh press on the pause button toggles
// it once; while paused the release, start counter, object animation, lemmings, nuke and clock
// stop, and skill, nuke and playfield clicks are ignored. Its oracle is that rule.
func TestPause(t *testing.T) {
	g := doorGame(3)
	for range 60 {
		press(g, -1, false)
	}
	press(g, Pause, true)
	l := g.Lemmings[0]
	start, update := g.start, g.Update
	m, s := g.Clock()
	press(g, Pause, true) // held: no second toggle
	press(g, int(Digger)+2, false)
	press(g, int(Digger)+2, true)
	press(g, Nuke, false)
	press(g, Nuke, true)
	for range 100 {
		press(g, -1, false)
	}
	if !g.paused || g.Lemmings[0] != l || len(g.Lemmings) != 1 || g.start != start ||
		g.Selected() != Climber || g.Nuked() || g.NukeRecord != 0 || g.Update != update+105 {
		t.Errorf("paused: paused %v, lemming %+v (was %+v), %d released, start %d (was %d), selected %d, nuked %v, update %d",
			g.paused, g.Lemmings[0], l, len(g.Lemmings), g.start, start, g.Selected(), g.Nuked(), g.Update)
	}
	if m2, s2 := g.Clock(); m2 != m || s2 != s {
		t.Errorf("paused: clock %d:%02d, was %d:%02d", m2, s2, m, s)
	}
	press(g, Pause, true)
	if g.paused {
		t.Errorf("second fresh press: still paused")
	}
	press(g, -1, false)
	if g.Lemmings[0] == l {
		t.Errorf("unpaused: lemming did not move")
	}
}

// TestNukeClick checks the nuke click: each click records the start counter and triggers the nuke
// when the counter is 0-5 updates past the previous click's record; the record is not reset between
// levels. Its oracle is that rule.
func TestNukeClick(t *testing.T) {
	g := doorGame(1)
	for g.start < 9 {
		press(g, -1, false)
	}
	press(g, Nuke, true) // start 10: the first click
	if g.Nuked() || g.NukeRecord != 10 {
		t.Fatalf("first click: nuked %v, record %d; want false, 10", g.Nuked(), g.NukeRecord)
	}
	for g.start < 15 {
		press(g, -1, false)
	}
	press(g, Nuke, true) // 6 past the record: a new first click
	if g.Nuked() || g.NukeRecord != 16 {
		t.Fatalf("click 6 updates later: nuked %v, record %d; want false, 16", g.Nuked(), g.NukeRecord)
	}
	press(g, -1, false)
	for g.start < 20 {
		press(g, -1, false)
	}
	press(g, Nuke, true) // 5 past
	if !g.Nuked() {
		t.Fatalf("click 5 updates later: not nuked")
	}

	// The next level keeps the record: one click 0-5 updates past it nukes.
	next := doorGame(1)
	next.NukeRecord = g.NukeRecord
	for next.start < 22 {
		press(next, -1, false)
	}
	press(next, Nuke, true)
	if !next.Nuked() {
		t.Errorf("single click 2 updates past the last level's record: not nuked")
	}
	// From program start the record is 0, so a single click in updates 1-5 nukes.
	first := doorGame(1)
	for first.start < 2 {
		press(first, -1, false)
	}
	press(first, Nuke, true)
	if !first.Nuked() {
		t.Errorf("single click in update 3 with record 0: not nuked")
	}
}

// TestNukeStep checks the nuke's step: none in the update of the click; then one step per update in
// release order, skipping removed lemmings at once, using up an update on a fused, exploding or
// splatting one and giving any other a 79-update fuse; no more releases; the level finishes with
// none in play. Its oracle is that rule.
func TestNukeStep(t *testing.T) {
	g := doorGame(10)
	g.Lemmings = []Lemming{
		{X: 100, Y: 50, Removed: true},
		{X: 100, Y: 50, State: Splatting, Frame: 1},
		{X: 100, Y: 50, State: Exiting},
		{X: 100, Y: 50, State: Drowning, Removed: true},
		{X: 100, Y: 50},
	}
	g.nuke, g.nukeAt = true, -1
	g.countdown = 1
	g.Step() // the click's update
	if len(g.Lemmings) != 5 {
		t.Fatalf("%d lemmings after the nuke, want no more released", len(g.Lemmings))
	}
	fuses := func() (f [5]int) {
		for i, l := range g.Lemmings {
			f[i] = l.fuse
		}
		return
	}
	if f := fuses(); f != [5]int{} {
		t.Fatalf("update of the click: fuses %v, want none", f)
	}
	g.Step() // skips 0, uses the update on the splatter
	g.Step() // the exiting lemming
	if f := fuses(); f != [5]int{0, 0, 79, 0, 0} {
		t.Errorf("two updates later: fuses %v, want the exiting lemming's only", f)
	}
	g.Step() // skips 3, fuses 4
	if f := fuses(); f[4] != 79 {
		t.Errorf("three updates later: fuses %v, want lemming 4's", f)
	}
}

// TestNukeFinishes checks the level-finished test with the nuke: no lemming in play finishes the
// level once the nuke is active, though not all were released.
func TestNukeFinishes(t *testing.T) {
	g := doorGame(10)
	g.Lemmings = []Lemming{{X: 100, Y: 50, Removed: true}}
	g.inPlay = 0
	g.nuke = true
	g.Step()
	if !g.Finished {
		t.Errorf("nuked, none in play: not finished")
	}
}
