package main

import (
	"image"
	"math/bits"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/benemon/openlemm/internal/level"
	"github.com/benemon/openlemm/internal/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestToMap's oracle is the screen layout: screen rows 16-174 show level rows 0-158 of a 320-pixel
// window whose left edge is the scroll position in map x; map row = level row + 4.
func TestToMap(t *testing.T) {
	for _, c := range []struct{ sx, sy, scroll, px, py int }{
		{0, 16, 0, 0, 4},
		{319, 174, 0, 319, 162},
		{5, 100, 640, 645, 88},
		{319, 16, 1280, 1599, 4},
	} {
		if px, py := toMap(c.sx, c.sy, c.scroll); px != c.px || py != c.py {
			t.Errorf("toMap(%d, %d, %d) = %d, %d; want %d, %d", c.sx, c.sy, c.scroll, px, py, c.px, c.py)
		}
	}
}

// TestScrollStep's oracle is the scrolling rule: limits 0 and 1280; a 16-pixel step to the right
// is taken only when it lands below 1280, one to the left when it lands on 0 or above; a 4-pixel
// step is skipped only at the limit it moves towards.
func TestScrollStep(t *testing.T) {
	for _, c := range []struct{ scroll, d, want int }{
		{640, 4, 644},
		{640, -4, 636},
		{640, 16, 656},
		{16, -16, 0},
		{8, -16, 8},
		{1248, 16, 1264},
		{1264, 16, 1264},
		{1260, 16, 1276},
		{1272, 16, 1272},
		{1276, 4, 1280},
		{1280, 4, 1280},
		{4, -4, 0},
		{0, -4, 0},
	} {
		if got := scrollStep(c.scroll, c.d); got != c.want {
			t.Errorf("scrollStep(%d, %d) = %d, want %d", c.scroll, c.d, got, c.want)
		}
	}
}

// TestSpriteFor's oracle is the decoded LemGrafP (block offset, frame bytes, frames, left-facing
// set; the explosion's 32 × 32 first frame) and the lemming drawing rule (offsets from the feet;
// the behaviour priority choosing among set bits).
func TestSpriteFor(t *testing.T) {
	for _, c := range []struct {
		name  string
		state int
		want  sprite
	}{
		{"walking", 0, sprite{0x0000, 0x50, 9, 0x2D0, -8, -10, 16}},
		{"jumping", sim.Jumping, sprite{0x0000, 0x50, 9, 0x2D0, -8, -10, 16}},
		{"falling", sim.Falling, sprite{0x05A0, 0x50, 4, 0x140, -8, -10, 16}},
		{"digging", sim.Digging, sprite{0x0820, 0x70, 16, 0, -8, -12, 16}},
		{"climbing", sim.Climbing, sprite{0x0F20, 0x60, 8, 0x300, -8, -12, 16}},
		{"hoisting", sim.Hoisting, sprite{0x1520, 0x60, 8, 0x300, -8, -12, 16}},
		{"exploding", sim.Exploding, sprite{0x1B20, 0x200, 1, 0, -15, -25, 32}},
		{"drowning with exploding", sim.Drowning | sim.Exploding, sprite{0x1B20, 0x200, 1, 0, -15, -25, 32}},
		{"building", sim.Building, sprite{0x1D20, 0x68, 16, 0x680, -8, -13, 16}},
		{"splatting", sim.Splatting | sim.Falling, sprite{0x2A20, 0x50, 16, 0, -8, -10, 16}},
		{"blocking", sim.Blocking, sprite{0x2F20, 0x50, 16, 0, -8, -10, 16}},
		{"bashing", sim.Bashing, sprite{0x3420, 0x50, 32, 0xA00, -8, -10, 16}},
		{"floating", sim.Floating, sprite{0x4820, 0x80, 8, 0x400, -8, -16, 16}},
		{"mining", sim.Mining, sprite{0x5020, 0x68, 24, 0x9C0, -8, -13, 16}},
		{"drowning", sim.Drowning, sprite{0x63A0, 0x50, 16, 0, -8, -10, 16}},
		{"exiting", sim.Exiting, sprite{0x68A0, 0x68, 8, 0, -8, -13, 16}},
		{"burning", sim.Burning, sprite{0x6BE0, 0x70, 14, 0, -8, -14, 16}},
		{"shrugging", sim.Shrugging, sprite{0x7200, 0x50, 8, 0x280, -8, -10, 16}},
		{"oh no", sim.OhNo, sprite{0x7700, 0x50, 16, 0, -8, -10, 16}},
	} {
		if got := spriteFor(&sim.Lemming{State: c.state}); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// TestTypeName's oracle is the status line's naming rule: climber and floater ATHLETE, floater
// FLOATER, climber CLIMBER, fuse running BOMBER, walking WALKER, falling FALLER, exploding BOMBER,
// then digging, building, blocking, bashing, mining by name; else blank.
func TestTypeName(t *testing.T) {
	for _, c := range []struct {
		name   string
		skills []sim.Skill
		state  int
		want   string
	}{
		{"athlete", []sim.Skill{sim.Climber, sim.Floater}, sim.Digging, "ATHLETE"},
		{"floater", []sim.Skill{sim.Floater}, sim.Digging, "FLOATER"},
		{"climber", []sim.Skill{sim.Climber}, sim.Falling, "CLIMBER"},
		{"fused walker", []sim.Skill{sim.Bomber}, 0, "BOMBER"},
		{"walker", nil, 0, "WALKER"},
		{"faller", nil, sim.Falling, "FALLER"},
		{"exploding", nil, sim.Exploding, "BOMBER"},
		{"digger", nil, sim.Digging, "DIGGER"},
		{"builder", nil, sim.Building, "BUILDER"},
		{"blocker", nil, sim.Blocking, "BLOCKER"},
		{"basher", nil, sim.Bashing, "BASHER"},
		{"miner", nil, sim.Mining, "MINER"},
		{"shrugger", nil, sim.Shrugging, ""},
		{"jumper", nil, sim.Jumping, ""},
		{"oh no", nil, sim.OhNo, ""},
		{"oh no, building kept", nil, sim.OhNo | sim.Building, "BUILDER"},
	} {
		g := sim.New(&level.Level{Map: image.NewPaletted(image.Rect(0, 0, 1632, 176), nil)}, &sim.Tables{})
		g.Skills = [8]int{1, 1, 1, 1, 1, 1, 1, 1}
		g.Lemmings = []sim.Lemming{{X: 100, Y: 50, Dir: 1}}
		for _, s := range c.skills {
			if !g.Assign(0, s) {
				t.Fatalf("%s: skill %d refused", c.name, s)
			}
		}
		g.Lemmings[0].State = c.state
		if got := typeName(&g.Lemmings[0]); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// TestFuseDigit's oracle is the lemming drawing rule: while a fuse runs, the digit fuse ÷ 16 + 1 is
// drawn.
func TestFuseDigit(t *testing.T) {
	for _, c := range []struct{ fuse, want int }{{0, 0}, {1, 1}, {15, 1}, {16, 2}, {64, 5}, {79, 5}} {
		if got := fuseDigit(c.fuse); got != c.want {
			t.Errorf("fuseDigit(%d) = %d, want %d", c.fuse, got, c.want)
		}
	}
}

// TestObjectFrame's oracle is the object animation rule: a static object shows frame 0; a looping
// one n mod its frame count; a trapdoor frame 1 up to update 34, frames 2 … 9 in updates 35 … 42
// and frame 0 from update 43.
func TestObjectFrame(t *testing.T) {
	for _, c := range []struct {
		name            string
		loop, entrance  bool
		frames, n, want int
	}{
		{"static", false, false, 1, 77, 0},
		{"loop", true, false, 8, 1, 1},
		{"loop wraps", true, false, 8, 61, 5},
		{"trapdoor at 1", false, true, 10, 1, 1},
		{"trapdoor at 34", false, true, 10, 34, 1},
		{"trapdoor at 35", false, true, 10, 35, 2},
		{"trapdoor at 42", false, true, 10, 42, 9},
		{"trapdoor at 43", false, true, 10, 43, 0},
		{"trapdoor at 500", false, true, 10, 500, 0},
	} {
		if got := objectFrame(c.loop, c.entrance, c.frames, c.n); got != c.want {
			t.Errorf("%s: frame %d, want %d", c.name, got, c.want)
		}
	}
}

// TestParticle's oracle is the particle drawing rule: a particle of value c at an even x turns that
// pixel into old | (c mod 16) and the pixel to its right into c ÷ 16; at an odd x, that pixel into
// old | (c mod 16) and the pixel to its left into 0.
func TestParticle(t *testing.T) {
	row := []uint8{5, 6, 9, 9}
	particle(row, 0, 0x4A)
	particle(row, 3, 0x21)
	if want := []uint8{5 | 0xA, 4, 0, 9 | 1}; string(row) != string(want) {
		t.Errorf("row %v, want %v", row, want)
	}
}

// TestParticlesAfterNuke's oracle is the particle drawing rule: particles k of a frame use c = 79 −
// k, or c = 39 − k for 40 particles once the nuke has been triggered.
func TestParticlesAfterNuke(t *testing.T) {
	a := &art{prog: make([]byte, particleAddr+51*160)}
	for k := range 80 {
		a.prog[particleAddr+2*k+1] = byte(k) // dy: particle k at (x, y + 4 + k)
	}
	for _, nuked := range []bool{false, true} {
		view := image.NewPaletted(image.Rect(0, 0, mapW, 160), nil)
		a.drawParticles(view, &sim.Lemming{X: 10, Y: 50, Frame: 1, State: sim.Exploding}, nuked)
		n := 80
		if nuked {
			n = 40
		}
		for k := range 80 {
			got, want := view.Pix[(50+k)*mapW+10], uint8((n-1-k)&15)
			if k >= n {
				want = 0
			}
			if got != want {
				t.Errorf("nuked %v, particle %d: colour %d, want %d", nuked, k, got, want)
				break
			}
		}
	}
}

// TestClampPointer's oracle is the pointer range: in play the centre, 16 rows below the menu
// pointer's top-left, stays on screen columns 0-319 and rows 16-222.
func TestClampPointer(t *testing.T) {
	for _, c := range []struct{ x, y, wx, wy float64 }{
		{160, 128, 160, 128},
		{-5, 300, 0, 222},
		{100, 223, 100, 222},
		{100, 222, 100, 222},
		{400, 0, 319, 16},
		{0.5, 16.5, 0.5, 16.5},
	} {
		f := &flow{ph: playing, mx: c.x, my: c.y - 16}
		if f.clampPointer(); f.mx != c.wx || f.my+16 != c.wy {
			t.Errorf("centre (%v, %v) clamped to %v, %v; want %v, %v", c.x, c.y, f.mx, f.my+16, c.wx, c.wy)
		}
	}
}

// TestEdgeStep's oracle is the scrolling rule: 4 pixels at the left or right screen edge with the
// left button up on rows 16-200, 16 with the right held.
func TestEdgeStep(t *testing.T) {
	for _, c := range []struct {
		sx, sy      int
		left, right bool
		want        int
	}{
		{0, 100, false, false, -4},
		{319, 100, false, false, 4},
		{0, 100, false, true, -16},
		{319, 183, false, true, 16},
		{1, 100, false, false, 0},
		{318, 100, false, false, 0},
		{0, 100, true, false, 0},
		{319, 200, false, false, 4},
		{0, 201, false, false, 0},
	} {
		if got := edgeStep(c.sx, c.sy, c.left, c.right); got != c.want {
			t.Errorf("edgeStep(%d, %d, %v, %v) = %d, want %d", c.sx, c.sy, c.left, c.right, got, c.want)
		}
	}
}

// TestMinimapScroll's oracle is the minimap rule: a click on rows 200-222 from column 208 sets
// scroll = clamp(x - 218, 0, 80) × 16.
func TestMinimapScroll(t *testing.T) {
	for _, c := range []struct {
		sx, sy, want int
		ok           bool
	}{
		{208, 210, 0, true},
		{218, 210, 0, true},
		{219, 210, 16, true},
		{258, 200, 640, true},
		{298, 222, 1280, true},
		{319, 210, 1280, true},
		{207, 210, 0, false},
		{250, 199, 0, false},
	} {
		if got, ok := minimapScroll(c.sx, c.sy); got != c.want || ok != c.ok {
			t.Errorf("minimapScroll(%d, %d) = %d, %v; want %d, %v", c.sx, c.sy, got, ok, c.want, c.ok)
		}
	}
}

// TestWindowScale checks the window fits the display with an 8-pixel border round the menus:
// 336 × scale within 90% of its width and 232 × scale within 85% of its height, never below 1.
func TestWindowScale(t *testing.T) {
	for _, c := range []struct{ w, h, want int }{
		{1470, 956, 3},
		{1920, 1080, 3},
		{2560, 1440, 5},
		{1280, 800, 2},
		{3840, 1080, 3},
		{1024, 600, 2},
		{640, 480, 1},
		{200, 100, 1},
	} {
		if got := windowScale(c.w, c.h); got != c.want {
			t.Errorf("windowScale(%d, %d) = %d, want %d", c.w, c.h, got, c.want)
		}
	}
}

// TestFit checks the shown 320 × 208 rows are drawn at the largest whole-number scale that fits,
// at least 1, centred with equal borders: original pixels only at integer scale, the crop before the
// scaling.
func TestFit(t *testing.T) {
	for _, c := range []struct{ w, h, s, ox, oy int }{
		{320, 208, 1, 0, 0},
		{1920, 1248, 6, 0, 0},
		{2940, 1912, 9, 30, 20},
		{1920, 1080, 5, 160, 20},
		{2560, 1600, 7, 160, 72},
		{960, 1000, 3, 0, 188},
		{639, 415, 1, 159, 103},
		{300, 200, 1, -10, -4},
	} {
		if s, ox, oy := fit(c.w, c.h, shownH); s != c.s || ox != c.ox || oy != c.oy {
			t.Errorf("fit(%d, %d) = %d, %d, %d; want %d, %d, %d", c.w, c.h, s, ox, oy, c.s, c.ox, c.oy)
		}
	}
}

// TestReleaseKeys checks the release rule: Cmd-Backspace on macOS and Ctrl-Backspace
// elsewhere release the mouse and never reach the game; plain Backspace reaches the game, which
// deletes a letter with it on the access-code screen.
func TestReleaseKeys(t *testing.T) {
	for goos, want := range map[string]ebiten.Key{"darwin": ebiten.KeyMeta, "windows": ebiten.KeyControl, "linux": ebiten.KeyControl} {
		if k := releaseMod(goos); k != want {
			t.Errorf("%s: release modifier %v, want %v", goos, k, want)
		}
	}
	if release, toGame := backspaceAction(false); release || !toGame {
		t.Errorf("plain Backspace: release %v, to game %v; want false, true", release, toGame)
	}
	if release, toGame := backspaceAction(true); !release || toGame {
		t.Errorf("modified Backspace: release %v, to game %v; want true, false", release, toGame)
	}
}

// TestTextScreen checks the farewell's text placement on the 640 × 256 mode-12 screen: 8 × 8
// cells from column 0 of text row 4 (!Run's four line feeds), sysFont bit 0 leftmost, colour 1,
// wrapping after 80 columns, nothing drawn for codes outside the font, further lines below.
func TestTextScreen(t *testing.T) {
	line := "A" + strings.Repeat(" ", 78) + "\x01B!"
	pix := textScreen(line)
	cell := func(col, row int, c byte) {
		t.Helper()
		for y := range 8 {
			for x := range 8 {
				want := uint8(0)
				if sysFont[c-32][y]>>x&1 != 0 {
					want = farewellColor
				}
				if got := pix[(8*row+y)*wideW+8*col+x]; got != want {
					t.Fatalf("%q at column %d row %d: pixel (%d, %d) = %d, want %d", c, col, row, x, y, got, want)
				}
			}
		}
	}
	cell(0, 4, 'A')
	cell(80-1, 4, ' ') // the \x01 cell
	cell(0, 5, 'B')
	cell(1, 5, '!')
	n := 0
	for _, p := range pix {
		if p != 0 {
			n++
		}
	}
	lit := 0
	for _, c := range "AB!" {
		for _, b := range sysFont[c-32] {
			lit += bits.OnesCount8(b)
		}
	}
	if n != lit {
		t.Errorf("%d pixels set, want %d (only the three glyphs)", n, lit)
	}
	// 'A' row 0 is 0x0C: pixels 2 and 3 of the row, at screen row 32.
	if pix[32*wideW+2] != farewellColor || pix[32*wideW+1] != 0 {
		t.Error("'A' is not at the left edge of screen row 32 with bit 0 leftmost")
	}
	// Each further line starts a new text row after the rows the one before took, an empty line
	// taking one.
	pix = textScreen(line, "", "C")
	cell(0, 7, 'C')
}

// TestFitWide checks the farewell's 640-wide screen is drawn with pixels twice as tall as wide at
// whole-number scales: the largest even vertical scale (at least 2) at which 320 × rows fit, half
// that horizontally, centred.
func TestFitWide(t *testing.T) {
	for _, c := range []struct{ w, h, rows, hs, vs, ox, oy int }{
		{1920, 1296, 216, 3, 6, 0, 0},
		{2016, 1392, 216, 3, 6, 48, 48},
		{1600, 1100, 216, 2, 4, 160, 118},
		{320, 216, 216, 1, 2, -160, -108},
		{2940, 1912, 216, 4, 8, 190, 92},
	} {
		if hs, vs, ox, oy := fitWide(c.w, c.h, c.rows); hs != c.hs || vs != c.vs || ox != c.ox || oy != c.oy {
			t.Errorf("fitWide(%d, %d, %d) = %d, %d, %d, %d; want %d, %d, %d, %d",
				c.w, c.h, c.rows, hs, vs, ox, oy, c.hs, c.vs, c.ox, c.oy)
		}
	}
}

// TestUnknownDisc's oracle is the player's disc with one byte of MUSINFO, a file the assets do not
// read, flipped: Identify refuses it, so opening the image fails.
func TestUnknownDisc(t *testing.T) {
	p := filepath.Join(t.TempDir(), "altered.adf")
	if err := os.WriteFile(p, alteredImage(t), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := openDisc(p); err == nil || !strings.Contains(err.Error(), "unknown version") {
		t.Errorf("openDisc of an altered disc: err = %v, want unknown version", err)
	}
}

// TestVersionText's oracle is the build information Go stamps into the binary: the Homebrew
// formula's test matches the version line against the release's version.
func TestVersionText(t *testing.T) {
	info := &debug.BuildInfo{
		Main:     debug.Module{Path: "github.com/benemon/openlemm", Version: "v1.2.3"},
		Settings: []debug.BuildSetting{{Key: "vcs", Value: "git"}, {Key: "vcs.revision", Value: "abc123"}},
	}
	if got, want := versionText(info), "version=v1.2.3\nrevision=abc123\n"; got != want {
		t.Errorf("versionText = %q, want %q", got, want)
	}
	if got, want := versionText(nil), "version=(devel)\nrevision=\n"; got != want {
		t.Errorf("versionText(nil) = %q, want %q", got, want)
	}
}
