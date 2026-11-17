package sim

import (
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/benemon/openlemm/internal/disc"
	"github.com/benemon/openlemm/internal/level"
)

func readCSV(t *testing.T, name string) [][]string {
	t.Helper()
	f, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows[1:]
}

// diverge is the line naming a fixture's first difference from the sim; lemming is -1 when the
// field is not a lemming's.
func diverge(name string, update, lemming int, field string, trace, sim any) string {
	return fmt.Sprintf("DIVERGE %s update=%d lemming=%d field=%s trace=%v sim=%v", name, update, lemming, field, trace, sim)
}

// readInput returns the rows of an input fixture, failing with an UNSUPPORTED line when its header
// is not the one the replay reads.
func readInput(t *testing.T, name, file, header string) [][]string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if h, _, _ := strings.Cut(string(b), "\n"); strings.TrimSpace(h) != header {
		t.Fatalf("UNSUPPORTED %s %s columns %s", name, filepath.Base(file), strings.TrimSpace(h))
	}
	return readCSV(t, file)
}

// replay steps the level at play index n through a trace observed in the running game (columns
// update, lemming, x, y, dir, frame, state, removed; map coordinates), applying the skill
// assignments in name+".inputs" (update, lemming, skill) and the real input in name+".input"
// if they exist, and fails at the first difference with a diverge line. It calls each after every
// Step, and returns the game at the trace's last update.
func replay(t *testing.T, name string, n int, each ...func(*Game)) *Game {
	adf, dir := os.Getenv("OPENLEMM_DISC"), os.Getenv("OPENLEMM_FIXTURES")
	if adf == "" || dir == "" {
		t.Skip("OPENLEMM_DISC or OPENLEMM_FIXTURES not set")
	}
	img, err := os.ReadFile(adf)
	if err != nil {
		t.Fatal(err)
	}
	d, err := disc.Read(img)
	if err != nil {
		t.Fatal(err)
	}
	l, err := level.LoadPlay(d, n)
	if err != nil {
		t.Fatal(err)
	}
	prog, err := d.Unpack("/!Lemmings/Lemmings/LemProg")
	if err != nil {
		t.Fatal(err)
	}
	tab, err := LoadTables(prog)
	if err != nil {
		t.Fatal(err)
	}
	rows := readCSV(t, filepath.Join(dir, name))
	stem := strings.TrimSuffix(filepath.Base(name), ".csv")
	type input struct {
		n int
		s Skill
	}
	inputs := map[int][]input{}
	if _, err := os.Stat(filepath.Join(dir, name+".inputs")); err == nil {
		for _, r := range readInput(t, stem, filepath.Join(dir, name+".inputs"), "update,lemming,skill") {
			s, ok := map[string]Skill{"climber": Climber, "floater": Floater, "bomber": Bomber, "builder": Builder, "basher": Basher, "miner": Miner, "digger": Digger}[r[2]]
			if !ok {
				t.Fatalf("UNSUPPORTED %s skill %s", stem, r[2])
			}
			u, err1 := strconv.Atoi(r[0])
			n, err2 := strconv.Atoi(r[1])
			if err1 != nil || err2 != nil {
				t.Fatalf("input %v: bad number", r)
			}
			inputs[u] = append(inputs[u], input{n, s})
		}
	}

	type pointer struct {
		in     Input
		chosen int // record index + 1, 0 for none
	}
	real := map[int]pointer{}
	if _, err := os.Stat(filepath.Join(dir, name+".input")); err == nil {
		for _, r := range readInput(t, stem, filepath.Join(dir, name+".input"), "update,x,y,buttons,scroll,chosen") {
			var v [6]int
			for k, s := range r {
				if v[k], err = strconv.Atoi(s); err != nil {
					t.Fatalf("input %v: %v", r, err)
				}
			}
			if v[3]&^1 != 0 {
				t.Fatalf("UNSUPPORTED %s buttons %d", stem, v[3])
			}
			real[v[0]] = pointer{realInput(v[1], v[2], v[4], v[3]&1 != 0), v[5]}
		}
	}

	g := New(l, tab)
	seen := 0 // lemmings in the trace at the current update
	for i, r := range rows {
		var v [8]int
		for k, s := range r {
			if v[k], err = strconv.Atoi(s); err != nil {
				t.Fatalf("row %d: %v", i+2, err)
			}
		}
		u, n := v[0], v[1]
		if u != g.Update {
			if seen != len(g.Lemmings) {
				t.Fatal(diverge(stem, g.Update, -1, "lemmings", seen, len(g.Lemmings)))
			}
			for g.Update < u {
				p, ok := real[g.Update+1]
				if ok {
					g.Input = p.in
				}
				g.Step()
				for _, f := range each {
					f(g)
				}
				if ok && g.chosen+1 != p.chosen {
					t.Fatal(diverge(stem, g.Update, -1, "chosen", p.chosen-1, g.chosen))
				}
				for _, in := range inputs[g.Update] {
					// The oracle forces assignments, lowering the count even from 0 to -1, past
					// the game's refusals. Only play23-water-midfall's digger, on steel, is one
					// the game would refuse: it is written as the oracle wrote it.
					g.Skills[in.s] = max(g.Skills[in.s], 1)
					if l := &g.Lemmings[in.n]; !g.Assign(in.n, in.s) {
						if in.s != Digger || l.own&15 != steel {
							t.Fatal(diverge(stem, g.Update, in.n, "assign", in.s, "refused"))
						}
						l.State, l.Frame = Digging, digFrom
						g.Skills[in.s]--
					}
				}
			}
			seen = 0
		}
		seen++
		if n >= len(g.Lemmings) {
			t.Fatal(diverge(stem, u, n, "lemmings", n+1, len(g.Lemmings)))
		}
		m := g.Lemmings[n]
		got := [6]int{m.X, m.Y, m.Dir, m.Frame, m.State, 0}
		if m.Removed {
			got[5] = 1
		}
		for k, name := range [6]string{"x", "y", "dir", "frame", "state", "removed"} {
			if got[k] != v[2+k] {
				t.Logf("lemming %+v", m)
				t.Fatal(diverge(stem, u, n, name, v[2+k], got[k]))
			}
		}
	}
	if seen != len(g.Lemmings) {
		t.Fatal(diverge(stem, g.Update, -1, "lemmings", seen, len(g.Lemmings)))
	}
	t.Logf("matched %d rows to update %d", len(rows), g.Update)
	return g
}

// realInput is the sim's Input for the game's stored pointer (x, y), the view's left edge scroll
// and the left button. The stored pointer is the centre's view column and level row (screen row −
// 16), so map px = x + scroll, py = y + 4; level rows below 176 are the playfield, and screen rows
// 200–223 are the panel buttons, picked by column.
func realInput(x, y, scroll int, left bool) Input {
	b := -1
	if sy := y + 16; sy >= 200 && sy <= 223 && x < 16*(Nuke+1) {
		b = x / 16
	}
	return Input{X: x + scroll, Y: y + 4, Playfield: y < 176, Button: b, Left: left}
}

func TestPlay0NoInput(t *testing.T) {
	replay(t, "play0-noinput.csv", 0)
}

// TestPlay0Digger gives lemming 0 the digger at update 1136; all 10 lemmings exit.
func TestPlay0Digger(t *testing.T) {
	g := replay(t, "play0-digger.csv", 0)
	if g.Saved != 10 || g.Skills[Digger] != 9 {
		t.Errorf("saved %d, diggers left %d; want 10, 9", g.Saved, g.Skills[Digger])
	}
}

// TestPlay1Floaters replays play index 1 (Fun 2, record 77). Lemmings 0-2 get floaters.
func TestPlay1Floaters(t *testing.T) {
	replay(t, "play1-floaters.csv", 1)
}

// TestPlay1FloatersMap compares the sim's map after the play1-floaters.csv trace with
// play1-floaters.map.
func TestPlay1FloatersMap(t *testing.T) {
	compareMap(t, replay(t, "play1-floaters.csv", 1), "play1-floaters.map")
}

// TestPlay3ClimbersMiner replays play index 3 (Fun 4, record 74). Lemmings 0 and 2 get climbers,
// lemming 1 the miner.
func TestPlay3ClimbersMiner(t *testing.T) {
	replay(t, "play3-climbers-miner.csv", 3)
}

// TestPlay3Map compares the sim's map after the play3-climbers-miner.csv trace with
// play3-climbers-miner.map.
func TestPlay3Map(t *testing.T) {
	compareMap(t, replay(t, "play3-climbers-miner.csv", 3), "play3-climbers-miner.map")
}

// TestPlay4Bashers replays play index 4 (record 75). Lemming 1 gets a basher at update 118,
// lemming 0 at update 168.
func TestPlay4Bashers(t *testing.T) {
	replay(t, "play4-bashers.csv", 4)
}

// TestPlay4Map compares the sim's map after the play4-bashers.csv trace with play4-bashers.map.
func TestPlay4Map(t *testing.T) {
	compareMap(t, replay(t, "play4-bashers.csv", 4), "play4-bashers.map")
}

// TestPlay6Builders replays play index 6 (record 79). Lemming 0 gets a builder at update 65,
// lemming 1 at update 302.
func TestPlay6Builders(t *testing.T) {
	replay(t, "play6-builders.csv", 6)
}

// TestPlay6Map compares the sim's map after the play6-builders.csv trace with play6-builders.map.
func TestPlay6Map(t *testing.T) {
	compareMap(t, replay(t, "play6-builders.csv", 6), "play6-builders.map")
}

// TestPlay5Bombers replays play index 5 (record 76). Lemming 0 gets a bomber at update 65,
// lemming 3 at update 224.
func TestPlay5Bombers(t *testing.T) {
	replay(t, "play5-bombers.csv", 5)
}

// TestPlay5Map compares the sim's map after the play5-bombers.csv trace with play5-bombers.map.
func TestPlay5Map(t *testing.T) {
	compareMap(t, replay(t, "play5-bombers.csv", 5), "play5-bombers.map")
}

// TestPlay2BuilderBomber replays play index 2 (record 78). Lemming 0 gets a builder at update 60,
// lemming 1 a bomber at update 113.
func TestPlay2BuilderBomber(t *testing.T) {
	replay(t, "play2-builder-bomber.csv", 2)
}

// TestPlay2Map compares the sim's map after the play2-builder-bomber.csv trace with
// play2-builder-bomber.map.
func TestPlay2Map(t *testing.T) {
	compareMap(t, replay(t, "play2-builder-bomber.csv", 2), "play2-builder-bomber.map")
}

// compareMap compares g's whole map, colour index per pixel over 1632 × 176, with the fixture
// name, the game's terrain map after the last traced update (816 bytes per row, 4bpp, low nibble
// left; row 0 is the top of the 4 hidden rows).
func compareMap(t *testing.T, g *Game, name string) {
	t.Helper()
	m, err := os.ReadFile(filepath.Join(os.Getenv("OPENLEMM_FIXTURES"), name))
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != mapW/2*mapH {
		t.Fatalf("map: %d bytes, want %d", len(m), mapW/2*mapH)
	}
	diff := 0
	for i, b := range m {
		for k, v := range [2]uint8{b & 15, b >> 4} {
			if p := 2*i + k; g.terrain[p] != v {
				if diff == 0 {
					t.Error(diverge(strings.TrimSuffix(filepath.Base(name), ".map"), g.Update, -1, fmt.Sprintf("map(%d,%d)", p%mapW, p/mapW), v, g.terrain[p]))
				}
				diff++
			}
		}
	}
	if diff > 0 {
		t.Errorf("map: %d pixels differ", diff)
	}
}

// TestPlay2BlockerClick replays play index 2 (record 78) with real clicks: the blocker button at
// update 62 and lemming 0 at update 65, which blocks from then on; the rest turn at its field.
func TestPlay2BlockerClick(t *testing.T) {
	g := replay(t, "play2-blocker-click.csv", 2)
	if g.Skills[Blocker] != 9 {
		t.Errorf("blockers left %d, want 9", g.Skills[Blocker])
	}
}

// TestPlay2BlockerClickMap compares the sim's map after the play2-blocker-click.csv trace with
// play2-blocker-click.map.
func TestPlay2BlockerClickMap(t *testing.T) {
	compareMap(t, replay(t, "play2-blocker-click.csv", 2), "play2-blocker-click.map")
}

// TestPlay4BashersClick replays play index 4 (record 75) with bashers given by real clicks.
func TestPlay4BashersClick(t *testing.T) {
	replay(t, "play4-bashers-click.csv", 4)
}

// TestPlay4BashersClickMap compares the sim's map after the play4-bashers-click.csv trace with
// play4-bashers-click.map.
func TestPlay4BashersClickMap(t *testing.T) {
	compareMap(t, replay(t, "play4-bashers-click.csv", 4), "play4-bashers-click.map")
}

// TestTrapTraces replays the traces with a .traps fixture (update, slot, flags, frame) and compares
// each trap's frame with it: after a Step, TrapFrame is the frame shown in the update that Step
// finished. In play16-traps lemming 2 gets a basher at update 94.
func TestTrapTraces(t *testing.T) {
	dir := os.Getenv("OPENLEMM_FIXTURES")
	if dir == "" {
		t.Skip("OPENLEMM_FIXTURES not set")
	}
	for _, c := range []struct {
		name string
		play int
	}{{"play16-traps", 16}, {"play23-water-midfall", 23}, {"play16-crater-bottom", 16}} {
		t.Run(c.name, func(t *testing.T) {
			want := map[[2]int]int{}
			for _, r := range readCSV(t, filepath.Join(dir, c.name+".traps")) {
				u, _ := strconv.Atoi(r[0])
				s, _ := strconv.Atoi(r[1])
				want[[2]int{u, s}], _ = strconv.Atoi(r[3])
			}
			check := func(g *Game) {
				for s := range g.trapLen {
					if f, ok := want[[2]int{g.Update - 1, s}]; ok && g.TrapFrame(s) != f {
						t.Fatalf("update %d trap %d: frame %d, trace %d", g.Update-1, s, g.TrapFrame(s), f)
					}
				}
			}
			g := replay(t, c.name+".csv", c.play, check)
			g.Step()
			check(g)
		})
	}
}

// TestCraterTraces replays the traces with a .crater or .dig fixture (update, lemming, x0, y0, map)
// and compares the map region it holds, 20 pixels wide from (x0, y0), at each listed update's row.
// In play12-dig-break the digger breaks into open space at update 172 with solid pixels left at
// x 435 and 436 of the row it falls out of.
func TestCraterTraces(t *testing.T) {
	dir := os.Getenv("OPENLEMM_FIXTURES")
	if dir == "" {
		t.Skip("OPENLEMM_FIXTURES not set")
	}
	for _, c := range []struct {
		name, ext string
		play      int
	}{{"play1-fuse-fall-float", ".crater", 1}, {"play17-fire-bombers", ".crater", 17}, {"play17-crater-top", ".crater", 17},
		{"play16-crater-bottom", ".crater", 16}, {"play12-dig-break", ".dig", 12}} {
		t.Run(c.name, func(t *testing.T) {
			type region struct {
				x0, y0 int
				m      []byte
			}
			want := map[int][]region{}
			for _, r := range readCSV(t, filepath.Join(dir, c.name+c.ext)) {
				u, _ := strconv.Atoi(r[0])
				x0, _ := strconv.Atoi(r[2])
				y0, _ := strconv.Atoi(r[3])
				m, err := hex.DecodeString(r[4])
				if err != nil {
					t.Fatal(err)
				}
				want[u] = append(want[u], region{x0, y0, m})
			}
			n := 0
			replay(t, c.name+".csv", c.play, func(g *Game) {
				for _, w := range want[g.Update] {
					n++
					for i, b := range w.m {
						for k, v := range [2]uint8{b & 15, b >> 4} {
							x, y := w.x0+2*(i%10)+k, w.y0+i/10
							if g.terrain[y*mapW+x] != v {
								t.Fatalf("update %d (%d, %d): colour %d, trace %d", g.Update, x, y, g.terrain[y*mapW+x], v)
							}
						}
					}
				}
			})
			if n != len(readCSV(t, filepath.Join(dir, c.name+c.ext))) {
				t.Errorf("compared %d regions", n)
			}
		})
	}
}

// TestPlay12DigBreakMap replays play12-dig-break, a digger given by real clicks, and compares the
// sim's map after it with play12-dig-break.map.
func TestPlay12DigBreakMap(t *testing.T) {
	compareMap(t, replay(t, "play12-dig-break.csv", 12), "play12-dig-break.map")
}

// TestPlay16TrapsMap compares the sim's map after the play16-traps.csv trace with play16-traps.map.
func TestPlay16TrapsMap(t *testing.T) {
	compareMap(t, replay(t, "play16-traps.csv", 16), "play16-traps.map")
}

// TestPlay8BombersWater replays play index 8 (Fun 9, an OddTable level). Lemming 0 gets a bomber
// at update 67, lemming 2 at update 75.
func TestPlay8BombersWater(t *testing.T) {
	replay(t, "play8-bombers-water.csv", 8)
}

// TestPlay8BombersWaterMap compares the sim's map after the play8-bombers-water.csv trace with
// play8-bombers-water.map.
func TestPlay8BombersWaterMap(t *testing.T) {
	compareMap(t, replay(t, "play8-bombers-water.csv", 8), "play8-bombers-water.map")
}

// TestPlay8Fire replays play index 8 with no input.
func TestPlay8Fire(t *testing.T) {
	replay(t, "play8-fire.csv", 8)
}

// TestPlay8FireMap compares the sim's map after the play8-fire.csv trace with play8-fire.map.
func TestPlay8FireMap(t *testing.T) {
	compareMap(t, replay(t, "play8-fire.csv", 8), "play8-fire.map")
}

// TestNewHazards replays the mid-air trigger and fuse traces and compares each final map.
func TestNewHazards(t *testing.T) {
	for _, c := range []struct {
		name string
		play int
	}{{"play0-exit-midfall", 0}, {"play1-fuse-fall-float", 1}, {"play17-fire-midfall", 17}, {"play17-fire-bombers", 17}, {"play23-water-midfall", 23},
		{"play17-crater-top", 17}, {"play16-crater-bottom", 16}} {
		t.Run(c.name, func(t *testing.T) {
			compareMap(t, replay(t, c.name+".csv", c.play), c.name+".map")
		})
	}
}
