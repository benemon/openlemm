package sim

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

// traceSounds derives, from a trace of the original game and its input files, the sound requests
// the game's music-mode sound triggers made: each one in the update whose moves led to the state
// change the trace shows at the next update, or, for clicks, in the update of the click. Requests
// the traces cannot show (refusals, rate ticks) are not derived.
// Forced assignments (name+".inputs") bypassed the game's assignment code, so they made no sound;
// replay's Assign calls come after the hook, so their requests are not collected either.
func traceSounds(t *testing.T, name string) []Sound {
	dir := os.Getenv("OPENLEMM_FIXTURES")
	type rec struct{ x, dir, frame, state, removed int }
	prev := map[int]rec{}
	warn := map[int]int{} // frame-10 updates seen in the current building spell
	out := []Sound{{15, 0x16, 0x0E, -1}, {35, 0x15, 0x0C, -1}}
	// A lemming's request carries its x as the trace shows it at the next update: none of
	// these events moves the lemming after the request, and a lemming leaving by the bottom
	// keeps the x it had before.
	add := func(u, s, p, x int) { out = append(out, Sound{u, s, p, x}) }
	for _, r := range readCSV(t, filepath.Join(dir, name)) {
		var v [8]int
		for k, f := range r {
			v[k], _ = strconv.Atoi(f)
		}
		u, n := v[0], v[1]
		c := rec{v[2], v[4], v[5], v[6], v[7]}
		p, seen := prev[n]
		prev[n] = c
		if !seen {
			continue
		}
		gained := func(b int) bool { return c.state&b != 0 && p.state&b == 0 }
		switch {
		case c.removed == 1 && p.removed == 0:
			if p.state&(Exiting|Drowning|Splatting|Exploding) == 0 {
				add(u-1, 0x1E, 0x1A, c.x) // off the bottom, a trap, or a burner removed
			}
			continue
		case c.removed == 1:
			continue
		}
		switch {
		case gained(Exiting):
			add(u-1, 0x1B, 0x0C, c.x)
		case gained(Splatting):
			add(u-1, 0x19, 0x13, c.x)
		case c.state&Exploding != 0 && c.state&Drowning == 0 && c.frame == 1 && p.frame == 0:
			add(u-1, 0x1D, 0x10, c.x) // the first exploding update
		case gained(Drowning):
			add(u-1, 0x1E, 0x1A, c.x)
		case c.state&OhNo != 0 && c.frame == 5 && p.frame == 4:
			add(u-1, 0x18, 0x0E, c.x)
		case c.state == 0 && c.dir != p.dir && p.state == Mining,
			c.state == 0 && c.dir != p.dir && p.state == Bashing && c.x >= 16 && c.x < 1608,
			c.state == 0 && p.state == Digging:
			add(u-1, 0x1A, 0x0C, c.x)
		}
		if c.state != Building {
			warn[n] = 0
		} else if c.frame == 10 && p.frame != 10 {
			// Cycle k lays its brick with 13 − k left before the step.
			if warn[n]++; warn[n] >= 10 {
				add(u-1, 0x1C, 0x06, c.x)
			}
		}
		if (c.state == Blocking || c.state == Bashing) && p.state != c.state && c.frame == 0 &&
			hasInput(name) {
			add(u, 0x17, 0x13, c.x) // assigned by a click: only assignment starts these at frame 0 here
		}
	}
	if hasInput(name) {
		rows := readCSV(t, filepath.Join(dir, name+".input"))
		last := 0
		for _, r := range rows {
			u, _ := strconv.Atoi(r[0])
			x, _ := strconv.Atoi(r[1])
			y, _ := strconv.Atoi(r[2])
			b, _ := strconv.Atoi(r[3])
			if b&1 != 0 && last&1 == 0 && y+16 >= 200 && x >= 32 && x < 160 {
				add(u, 0x14, 0x0E+x/16-2, -1)
			}
			last = b
		}
	}
	return out
}

func hasInput(name string) bool {
	_, err := os.Stat(filepath.Join(os.Getenv("OPENLEMM_FIXTURES"), name+".input"))
	return err == nil
}

func sortSounds(s []Sound) {
	slices.SortFunc(s, func(a, b Sound) int {
		if a.Update != b.Update {
			return a.Update - b.Update
		}
		if a.Sample != b.Sample {
			return a.Sample - b.Sample
		}
		return a.Pitch - b.Pitch
	})
}

// TestTraceSounds compares the sim's sound requests over every trace with those derived from the
// trace by traceSounds. Its oracle is the trace's state changes read through the sound triggers.
func TestTraceSounds(t *testing.T) {
	for _, c := range []struct {
		name string
		play int
	}{
		{"play0-noinput.csv", 0}, {"play0-digger.csv", 0}, {"play1-floaters.csv", 1},
		{"play2-builder-bomber.csv", 2}, {"play3-climbers-miner.csv", 3}, {"play4-bashers.csv", 4},
		{"play5-bombers.csv", 5}, {"play6-builders.csv", 6}, {"play2-blocker-click.csv", 2},
		{"play4-bashers-click.csv", 4},
	} {
		t.Run(c.name, func(t *testing.T) {
			var got []Sound
			replay(t, c.name, c.play, func(g *Game) { got = append(got, g.Sounds...) })
			want := traceSounds(t, c.name)
			sortSounds(got)
			sortSounds(want)
			if !slices.Equal(got, want) {
				t.Errorf("sounds differ:\n got %v\nwant %v", got, want)
			}
			t.Logf("%d sounds", len(got))
		})
	}
}

// soundsOf runs one Step and returns its requests without their update numbers.
func soundsOf(g *Game) [][2]int {
	g.Step()
	var s [][2]int
	for _, r := range g.Sounds {
		s = append(s, [2]int{r.Sample, r.Pitch})
	}
	return s
}

// TestPanelSounds's oracle is the panel's sound triggers: a skill button plays &14 at &0E + skill,
// not while paused; a rate change plays &17 at new rate ÷ 3 + 1, nothing at the limit; the nuke
// &18.
func TestPanelSounds(t *testing.T) {
	g := doorGame(1)
	g.Input = Input{Button: int(Miner) + 2, Left: true}
	if s := soundsOf(g); !slices.Equal(s, [][2]int{{0x14, 0x0E + 6}}) {
		t.Errorf("miner button: %v", s)
	}
	g.Input = Input{Button: RateUp, Left: true}
	if s := soundsOf(g); !slices.Equal(s, [][2]int{{0x17, 51/3 + 1}}) {
		t.Errorf("rate up to 51: %v", s)
	}
	g.Input = Input{Button: RateDown, Left: true}
	soundsOf(g)
	if s := soundsOf(g); s != nil {
		t.Errorf("rate down at the minimum: %v", s)
	}
	g.Input = Input{Button: Pause}
	soundsOf(g)
	g.Input = Input{Button: Pause, Left: true}
	soundsOf(g)
	g.Input = Input{Button: int(Digger) + 2}
	soundsOf(g)
	g.Input.Left = true
	if s := soundsOf(g); s != nil {
		t.Errorf("skill button while paused: %v", s)
	}

	// The record starts at 0, so a single click in update 1 nukes.
	g = doorGame(1)
	g.Input = Input{Button: Nuke, Left: true}
	if s := soundsOf(g); !slices.Equal(s, [][2]int{{0x18, 0x0E}}) || !g.Nuked() {
		t.Errorf("nuke: %v", s)
	}
}

// TestStartSounds's oracle is the level start's sound triggers: &16 when the start counter reaches
// 15 and &15 at 35, both delayed by a pause; music from 55 in music mode only.
func TestStartSounds(t *testing.T) {
	g := doorGame(1)
	var at []Sound
	for range 20 {
		g.Step()
		at = append(at, g.Sounds...)
	}
	g.Input = Input{Button: Pause, Left: true}
	g.Step()
	g.Input = Input{}
	for range 10 {
		g.Step()
	}
	g.Input = Input{Button: Pause, Left: true}
	for range 70 {
		g.Step()
		at = append(at, g.Sounds...)
		g.Input = Input{}
		if g.start == 54 && g.MusicOn() || g.start == 55 && !g.MusicOn() {
			t.Errorf("start counter %d: music on %v", g.start, g.MusicOn())
		}
	}
	if want := []Sound{{15, 0x16, 0x0E, -1}, {35 + 11, 0x15, 0x0C, -1}}; !slices.Equal(at, want) {
		t.Errorf("sounds %v, want %v", at, want)
	}
	g.Effects = true
	if g.MusicOn() {
		t.Errorf("effects mode: music on")
	}
}

// TestStopSounds's oracle is the steel and edge sound triggers: the clink &1A at &0C for a digger
// refused on steel, a digger stopped by steel, a basher stopped by steel and a miner at the map
// edge; none for a basher turning at the map edge.
func TestStopSounds(t *testing.T) {
	clink := [][2]int{{0x1A, 0x0C}}
	g := clickGame(Lemming{X: 100, Y: 50, Dir: 1, own: steel})
	if g.Assign(0, Digger) || !slices.Equal(g.Sounds, []Sound{{0, 0x1A, 0x0C, 100}}) {
		t.Errorf("digger refused on steel: %v", g.Sounds)
	}

	g = clickGame(Lemming{X: 100, Y: 50, Dir: 1, State: Digging, Frame: 7})
	g.grid[51/4][100/4] = steel
	if s := soundsOf(g); !slices.Equal(s, clink) {
		t.Errorf("digger reaching steel: %v", s)
	}

	g = clickGame(Lemming{X: 100, Y: 50, Dir: 1, State: Bashing, Frame: 10})
	g.grid[42/4][108/4] = steel
	if s := soundsOf(g); !slices.Equal(s, clink) || g.Lemmings[0].State != 0 {
		t.Errorf("basher reaching steel: %v, state %#x", s, g.Lemmings[0].State)
	}
	g = clickGame(Lemming{X: 1607, Y: 50, Dir: 1, State: Bashing, Frame: 10})
	if s := soundsOf(g); s != nil || g.Lemmings[0].State != 0 {
		t.Errorf("basher at the map edge: %v, state %#x", s, g.Lemmings[0].State)
	}
	g = clickGame(Lemming{X: 1607, Y: 50, Dir: 1, State: Mining, Frame: 2})
	if s := soundsOf(g); !slices.Equal(s, clink) || g.Lemmings[0].State != 0 {
		t.Errorf("miner at the map edge: %v, state %#x", s, g.Lemmings[0].State)
	}
}

// TestModeSounds's oracle is the trap, water and fire sound triggers: in music mode a trap or water
// plays &1E at &1A at once and fire nothing until the burner is removed; in effects mode a trap
// plays its own sound, water &0A at &10, fire &08 at &11, and a drowner's removal &01 at &12.
func TestModeSounds(t *testing.T) {
	for _, c := range []struct {
		name    string
		effects bool
		cell    uint8
		state   int
		frame   int
		want    [][2]int
	}{
		{"trap, music", false, trap + 16*2, 0, 0, [][2]int{{0x1E, 0x1A}}},
		{"trap, effects", true, trap + 16*2, 0, 0, [][2]int{{7, 0x10}}},
		{"water, music", false, water, 0, 0, [][2]int{{0x1E, 0x1A}}},
		{"water, effects", true, water, 0, 0, [][2]int{{0x0A, 0x10}}},
		{"fire, music", false, fire, 0, 0, nil},
		{"fire, effects", true, fire, 0, 0, [][2]int{{0x08, 0x11}}},
		{"burner removed, music", false, 0, Burning, 13, [][2]int{{0x1E, 0x1A}}},
		{"burner removed, effects", true, 0, Burning, 13, nil},
		{"drowner removed, music", false, 0, Drowning, 15, nil},
		{"drowner removed, effects", true, 0, Drowning, 15, [][2]int{{0x01, 0x12}}},
	} {
		g := clickGame(Lemming{X: 100, Y: 50, Dir: 1, State: c.state, Frame: c.frame})
		g.Effects = c.effects
		g.trapLen[2], g.trapSound[2] = 8, [2]int{7, 0x10}
		g.grid[50/4][101/4] = c.cell
		if s := soundsOf(g); !slices.Equal(s, c.want) {
			t.Errorf("%s: %v, want %v", c.name, s, c.want)
		}
	}
}

// TestVolumeKeys's oracle is the music volume keys rule: keys are scanned F1, F2, Escape, F5; F1 or
// F2 moves the setting by 3 (clamped 0-255), sets the music volume to it and ends the scan.
func TestVolumeKeys(t *testing.T) {
	g := doorGame(1)
	g.Input = Input{VolDown: true, VolUp: true, Escape: true}
	for range 2 {
		g.Step()
	}
	if g.Volume != 249 || g.MusicVolume() != 249 || g.TimeUp {
		t.Errorf("F1 and F2 with Escape: volume %d, music %d, time up %v; want 249, 249, false", g.Volume, g.MusicVolume(), g.TimeUp)
	}
	g.Input = Input{VolUp: true, Escape: true}
	for range 3 {
		g.Step()
	}
	if g.Volume != 255 || g.TimeUp {
		t.Errorf("F2 past 255 with Escape: %d, time up %v", g.Volume, g.TimeUp)
	}
	g.Volume = 2
	g.Input = Input{VolDown: true}
	g.Step()
	if g.Volume != 0 {
		t.Errorf("F1 past 0: %d", g.Volume)
	}
}

// endAfter runs a level at setting v to the update in which time-up is first seen, then holds keys
// in for hold updates and returns how many updates after the flag the level ends, up to 200.
func endAfter(v int, in Input, hold int) int {
	g := doorGame(1)
	g.Volume = v
	g.Input = Input{Escape: true}
	g.Step()
	g.Input = Input{}
	g.Step() // the flag is first seen in this update
	n := 0
	for !g.Over && n < 200 {
		if n < hold {
			g.Input = in
		} else {
			g.Input = Input{}
		}
		g.Step()
		n++
	}
	return n
}

// TestEndingLength's oracle is the ending sequence rule: the level ends max(⌈v ÷ 16⌉, 11) updates
// after the flag is first seen, v the music volume then; F1 or F2 resets the music volume to the
// setting before each update's fall, so holding F2 keeps the level from ending; the setting itself
// is not lowered.
func TestEndingLength(t *testing.T) {
	for _, c := range []struct {
		v, want int
	}{{255, 16}, {200, 13}, {100, 11}, {0, 11}} {
		if n := endAfter(c.v, Input{}, 0); n != c.want {
			t.Errorf("volume %d: over after %d updates, want %d", c.v, n, c.want)
		}
	}
	if n := endAfter(255, Input{VolUp: true}, 150); n != 150+16 {
		t.Errorf("F2 held 150 updates: over after %d updates, want %d", n, 150+16)
	}
	// Held F1 from 255 lowers the setting 3 per update and resets the music to it before the
	// next update's fall of 16, so the level ends in the update whose fall starts from a setting of
	// 16 or less: 15, after 80 presses, in update 81.
	if n := endAfter(255, Input{VolDown: true}, 200); n != 81 {
		t.Errorf("F1 held: over after %d updates, want 81", n)
	}
	g := doorGame(1)
	g.Volume = 120
	g.Input = Input{Escape: true}
	for !g.Over {
		g.Step()
		g.Input = Input{}
	}
	if g.Volume != 120 || g.MusicVolume() != 0 {
		t.Errorf("after the ending: setting %d, music %d; want 120, 0", g.Volume, g.MusicVolume())
	}
}

// TestSpeakerKey's oracle is the speaker key rule: F5 toggles the speaker when seen down after
// having been seen up; while F1 or F2 is held F5 is not looked at, so it neither toggles nor counts
// as released; the first press turns the speaker off.
func TestSpeakerKey(t *testing.T) {
	g := doorGame(1)
	var got []bool
	for _, in := range []Input{
		{Speaker: true}, {Speaker: true}, // one press, held: off
		{}, {Speaker: true}, // released, pressed: on
		{Speaker: true, VolUp: true}, {VolUp: true}, // released while F2 held: not seen
		{Speaker: true},                    // so this press is not fresh
		{}, {Speaker: true, VolDown: true}, // pressed while F1 held: not seen
		{Speaker: true}, // seen down after up: off
	} {
		g.Input = in
		g.Step()
		got = append(got, g.Quiet)
	}
	want := []bool{true, true, true, false, false, false, false, false, false, true}
	if !slices.Equal(got, want) {
		t.Errorf("quiet %v, want %v", got, want)
	}
}

// readSounds returns a .sound fixture's requests by mode, each as update, sample and pitch.
func readSounds(t *testing.T, file string) map[string][][3]int {
	want := map[string][][3]int{}
	for _, r := range readCSV(t, file) {
		u, _ := strconv.Atoi(r[1])
		w, _ := strconv.ParseUint(r[2], 16, 16)
		want[r[0]] = append(want[r[0]], [3]int{u, int(w & 255), int(w >> 8)})
	}
	return want
}

// compareSounds fails with a diverge line at the first request in got that differs from want.
func compareSounds(t *testing.T, name string, got, want [][3]int) {
	t.Helper()
	req := func(s [][3]int, i int) string {
		if i >= len(s) {
			return "none"
		}
		return fmt.Sprintf("%d:%02X%02X", s[i][0], s[i][2], s[i][1])
	}
	for i := range max(len(got), len(want)) {
		if i >= len(got) || i >= len(want) || got[i] != want[i] {
			var u int
			if i < len(want) {
				u = want[i][0]
			} else {
				u = got[i][0]
			}
			t.Error(diverge(name, u, -1, "sound", req(want, i), req(got, i)))
			return
		}
	}
}

// TestHazardSounds replays the traces with a .sound fixture in each sound mode their .sound fixture
// lists and compares every request, in order, with the game's own there (mode, update, pitch << 8 |
// sample). The fixture numbers a request by the update whose processing made it, the update a Step
// finishes: g.Update - 1 after it.
func TestHazardSounds(t *testing.T) {
	dir := os.Getenv("OPENLEMM_FIXTURES")
	if dir == "" {
		t.Skip("OPENLEMM_FIXTURES not set")
	}
	for _, c := range []struct {
		name string
		play int
	}{
		{"play16-traps", 16}, {"play8-bombers-water", 8}, {"play8-fire", 8}, {"play0-exit-midfall", 0},
		{"play1-fuse-fall-float", 1}, {"play17-fire-midfall", 17}, {"play17-fire-bombers", 17},
		{"play23-water-midfall", 23}, {"play17-crater-top", 17}, {"play16-crater-bottom", 16},
	} {
		want := readSounds(t, filepath.Join(dir, c.name+".sound"))
		for _, mode := range []string{"music", "effects"} {
			if want[mode] == nil {
				continue
			}
			t.Run(c.name+"/"+mode, func(t *testing.T) {
				var got [][3]int
				collect := func(g *Game) {
					// The first Step runs in music mode; it makes no mode-dependent request.
					g.Effects = mode == "effects"
					for _, s := range g.Sounds {
						got = append(got, [3]int{g.Update - 1, s.Sample, s.Pitch})
					}
				}
				g := replay(t, c.name+".csv", c.play, collect)
				g.Step()
				collect(g)
				compareSounds(t, c.name, got, want[mode])
				t.Logf("%d sounds", len(got))
			})
		}
	}
}
