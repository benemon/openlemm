package main

import (
	"image/color"
	"slices"
	"testing"

	"github.com/benemon/openlemm/internal/screen"
)

func testFlow(t *testing.T) *flow {
	t.Helper()
	a, err := loadAssets(testDisc(t))
	if err != nil {
		t.Fatal(err)
	}
	return newFlow(a, nil)
}

// run steps f n frames with the same input, failing on an error.
func run(t *testing.T, f *flow, in input, n int) {
	t.Helper()
	for range n {
		if err := f.update(in); err != nil {
			t.Fatal(err)
		}
	}
}

// until steps f with input in until ph is open and returns the frames taken, at most limit.
func until(t *testing.T, f *flow, in input, ph phase, limit int) int {
	t.Helper()
	for n := 1; n <= limit; n++ {
		if err := f.update(in); err != nil {
			t.Fatal(err)
		}
		if f.ph == ph {
			return n
		}
	}
	t.Fatalf("phase %d not reached in %d frames (at %d)", ph, limit, f.ph)
	return 0
}

// openTitle opens the title menu, faded in, with no button held.
func openTitle(t *testing.T, f *flow) {
	f.enter(titleMenu)
	run(t, f, input{}, 40)
}

// clickAt moves the menu pointer's fingertip to (x, y) and clicks the left button: one frame
// held, one released.
func clickAt(t *testing.T, f *flow, x, y int) {
	f.mx, f.my = float64(x+16), float64(y)
	run(t, f, input{left: true}, 1)
	run(t, f, input{}, 1)
}

// TestShade's oracle is the fade rule: after fade-in step s a component whose final value is c
// shows max(0, c − 15 + s), i.e. darkness 15 − s.
func TestShade(t *testing.T) {
	p := color.Palette{color.RGBA{0xFF, 0x88, 0x11, 0xFF}}
	for _, c := range []struct {
		dark    int
		r, g, b uint8
	}{{0, 15, 8, 1}, {1, 14, 7, 0}, {8, 7, 0, 0}, {15, 0, 0, 0}} {
		got := shade(p, c.dark)[0].(color.RGBA)
		if got != (color.RGBA{c.r * 17, c.g * 17, c.b * 17, 0xFF}) {
			t.Errorf("dark %d: %v", c.dark, got)
		}
	}
	if m := brightest(p, color.Palette{color.Black}); m != 15 {
		t.Errorf("brightest %d", m)
	}
}

// TestFades's oracle is the fade rule: a fade-in takes a step every second frame and every colour
// arrives at step 15; a fade-out from a palette whose brightest component is m takes m + 1 steps,
// one every second frame, and the next screen then starts black.
func TestFades(t *testing.T) {
	f := testFlow(t)
	f.enter(titleMenu)
	if f.dark != 15 {
		t.Fatalf("title opens at darkness %d", f.dark)
	}
	run(t, f, input{}, 29)
	if f.dark != 1 {
		t.Errorf("after 29 frames darkness %d, want 1", f.dark)
	}
	run(t, f, input{}, 1)
	if f.dark != 0 {
		t.Errorf("after 30 frames darkness %d, want 0", f.dark)
	}
	m := brightest(f.menu.TitlePal)
	clickAt(t, f, 150, 160) // NEW LEVEL
	// The click's frame starts the fade-out; clickAt also ran the frame after it.
	if n := until(t, f, input{}, codeEntry, 100) + 2; n != 2*(m+1) {
		t.Errorf("fade-out took %d frames, want %d", n, 2*(m+1))
	}
	if f.dark != 15 {
		t.Errorf("access-code screen opens at darkness %d", f.dark)
	}
}

// TestSplashes's oracle is the start-up sequence: Krisalis, Psygnosis, DMA Design in turn, each
// held 240 frames from when it starts to fade in, then faded out; black for 2 seconds (100 frames
// at 50 Hz); then the title, level 0 (Fun 1), music mode. A button held during a hold ends it at
// once and skips the splashes not yet shown.
func TestSplashes(t *testing.T) {
	f := testFlow(t)
	until(t, f, input{}, splashing, noticeHold+100)
	for k := range 3 {
		if f.ph != splashing || f.splash != k {
			t.Fatalf("splash %d not showing (phase %d, splash %d)", k, f.ph, f.splash)
		}
		v, err := f.view()
		if err != nil {
			t.Fatal(err)
		}
		if want := f.menu.Splash(k).Upper[0]; v.border != shade(color.Palette{want}, f.dark)[0] {
			t.Errorf("splash %d: border %v, want its colour 0 %v", k, v.border, want)
		}
		m := f.bright
		// The hold's last frame starts the fade-out, whose last step opens the next screen. Each
		// splash opens on an even frame, so its hold ends on one and the first fade step comes
		// two frames later.
		next := splashing
		if k == 2 {
			next = waiting
		}
		n := 0
		for f.ph == splashing && f.splash == k {
			run(t, f, input{}, 1)
			n++
		}
		if f.ph != next || n != 240+2*(m+1) {
			t.Errorf("splash %d: %d frames, then phase %d; want %d, then %d", k, n, f.ph, 240+2*(m+1), next)
		}
	}
	if n := until(t, f, input{}, titleMenu, 1000); n != 100 {
		t.Errorf("black for %d frames, want 100", n)
	}
	if f.sess.level != 0 || f.sess.effects || f.sess.volume != 255 {
		t.Errorf("title opens with %+v", f.sess)
	}

	f = testFlow(t)
	until(t, f, input{}, splashing, noticeHold+100)
	run(t, f, input{}, 50)
	run(t, f, input{right: true}, 1)
	if !f.leaving {
		t.Fatal("a held button does not end the hold")
	}
	until(t, f, input{}, waiting, 100)
}

// TestNotice's oracle is the start-up notice as worded, shown
// before the first splash, held 250 frames with the splashes' fade in and out on black, a button
// ending the hold as it ends a splash's; the splashes after it are as without it.
func TestNotice(t *testing.T) {
	f := testFlow(t)
	if f.ph != notice || f.dark != 15 {
		t.Fatalf("program opens in phase %d at darkness %d, want the notice at 15", f.ph, f.dark)
	}
	v, err := f.view()
	if err != nil {
		t.Fatal(err)
	}
	want := f.menu.Notice(screen.Layout(noticeText, viewTop, menuH))
	if !slices.Equal(v.scr.Pix, want.Pix) || v.border != color.Black {
		t.Error("notice screen is not the laid-out wording on a black border")
	}
	m := f.bright
	n := 0
	for f.ph == notice {
		run(t, f, input{}, 1)
		n++
	}
	if f.ph != splashing || f.splash != 0 || n != 250+2*(m+1) || f.frame%2 != 0 {
		t.Errorf("notice: %d frames, then phase %d splash %d at frame %d; want %d, then splash 0 on an even frame",
			n, f.ph, f.splash, f.frame, 250+2*(m+1)) // 5 s at 50 Hz
	}

	f = testFlow(t)
	run(t, f, input{}, 10)
	run(t, f, input{left: true}, 1)
	if !f.leaving {
		t.Fatal("a button does not end the notice's hold")
	}
	until(t, f, input{}, splashing, 100)
	if f.splash != 0 || f.skip {
		t.Fatalf("after a skipped notice: splash %d, skip %v; want splash 0 shown", f.splash, f.skip)
	}
	n, m = 0, f.bright
	for f.splash == 0 {
		run(t, f, input{}, 1)
		n++
	}
	if want := 240 + 2*(m+1); n != want {
		t.Errorf("splash 0 after a skipped notice lasts %d frames, want %d", n, want)
	}
}

// TestTitleBoards's oracle is the title menu's click rule: left button presses only, a button held
// as the title opens must be released first; the boards' columns on rows 148-174.
func TestTitleBoards(t *testing.T) {
	f := testFlow(t)
	f.enter(titleMenu)
	f.mx, f.my = 220+16, 160
	run(t, f, input{left: true}, 40)
	if f.sess.effects {
		t.Error("a button held as the title opens toggled the music sign")
	}
	run(t, f, input{}, 1)
	run(t, f, input{left: true}, 1)
	if !f.sess.effects {
		t.Error("music sign click did not select sound effects")
	}
	run(t, f, input{left: true}, 5)
	if !f.sess.effects {
		t.Error("holding the button toggled the sign again")
	}
	run(t, f, input{}, 1)
	f.mx, f.my = 220+16, 147
	run(t, f, input{left: true}, 1)
	run(t, f, input{right: true, middle: true}, 1)
	if !f.sess.effects {
		t.Error("click above the boards or another button toggled the sign")
	}

	for _, c := range []struct{ y, want int }{
		{148, 30}, {168, 60}, {148, 90}, {148, 90}, {168, 90}, {169, 60}, {174, 30}, {174, 0}, {174, 0},
	} {
		clickAt(t, f, 298, c.y)
		if f.sess.level != c.want {
			t.Errorf("arrow click on row %d: level %d, want %d", c.y, f.sess.level, c.want)
		}
	}
	clickAt(t, f, 291, 148)
	if f.sess.level != 0 {
		t.Errorf("click beside the arrows: level %d", f.sess.level)
	}

	// clickAt runs the click's frame and the one after it.
	clickAt(t, f, 100, 160)
	run(t, f, input{}, 57)
	clickAt(t, f, 100, 160)
	if f.ph != farewell {
		t.Error("second RETURN TO DESKTOP click 59 frames after the first did not quit")
	}
	openTitle(t, f)
	clickAt(t, f, 100, 160)
	if v, _ := f.view(); v.border != (color.RGBA{0xFF, 0, 0, 0xFF}) {
		t.Errorf("border %v after one click, want red", v.border)
	}
	run(t, f, input{}, 58)
	if v, _ := f.view(); v.border != (color.RGBA{0xFF, 0, 0, 0xFF}) {
		t.Errorf("border %v 59 frames after the click, want red", v.border)
	}
	run(t, f, input{}, 1)
	if v, _ := f.view(); v.border != color.Black {
		t.Errorf("border %v 60 frames after the click, want black", v.border)
	}
	clickAt(t, f, 100, 160)
	if f.quit || f.ph != titleMenu {
		t.Error("second click 60 frames after the first quit")
	}
	f.border = 0
	f.sess.show = true
	clickAt(t, f, 100, 160)
	clickAt(t, f, 100, 160)
	if f.quit || f.ph != titleMenu || f.border != 0 {
		t.Error("RETURN TO DESKTOP answered in show mode")
	}

	f.sess.level = 37
	clickAt(t, f, 30, 160)
	frozen := *f.title
	run(t, f, input{}, 3)
	if !f.leaving || *f.title != frozen {
		t.Fatal("START did not stop the title and fade it out")
	}
	until(t, f, input{}, preLevel, 100)
	if f.lvl == nil || f.sess.level != 37 {
		t.Errorf("START opened the pre-level screen of level %d", f.sess.level)
	}
}

// typeKeys presses each key of s (letters, '<' Backspace, '.' Return) for one frame, then
// releases all for one frame.
func typeKeys(t *testing.T, f *flow, s string) {
	for _, c := range s {
		var in input
		switch {
		case c >= 'A' && c <= 'Z':
			in.letters = 1 << (c - 'A')
		case c == '<':
			in.back = true
		case c == '.':
			in.ret = true
		}
		run(t, f, in, 1)
		run(t, f, input{}, 1)
	}
}

// TestCodeEntry's oracle is the access-code entry rule: letters fill the field and move the place
// on, the tenth place is replaced by further letters; Backspace clears the tenth letter in place,
// otherwise moves back and clears; each key acts once until all are released; an empty field
// returns at once, unchanged; a level's code selects it, the show and cheat codes set their modes,
// no match selects level 0.
func TestCodeEntry(t *testing.T) {
	f := testFlow(t)
	f.sess.level = 50
	f.enter(codeEntry)
	run(t, f, input{letters: 1}, 3)
	if string(f.field[:]) != "A---------" {
		t.Errorf("held key: field %q", f.field)
	}
	run(t, f, input{letters: 1, back: true}, 1)
	run(t, f, input{}, 1)
	typeKeys(t, f, "BCDEFGHIJKL")
	if string(f.field[:]) != "ABCDEFGHIL" || f.place != 9 {
		t.Errorf("field %q at place %d, want ABCDEFGHIL at 9", f.field, f.place)
	}
	typeKeys(t, f, "<")
	if string(f.field[:]) != "ABCDEFGHI-" || f.place != 9 {
		t.Errorf("Backspace at the tenth letter: %q at %d", f.field, f.place)
	}
	typeKeys(t, f, "<<")
	if string(f.field[:]) != "ABCDEFG---" || f.place != 7 {
		t.Errorf("Backspace twice more: %q at %d", f.field, f.place)
	}
	typeKeys(t, f, "<<<<<<<<<")
	if string(f.field[:]) != "----------" || f.place != 0 {
		t.Errorf("Backspace to the start: %q at %d", f.field, f.place)
	}
	typeKeys(t, f, ".") // the Return frame, then one more
	run(t, f, input{}, 1)
	if f.leaving {
		t.Error("empty field left before the second frame after finishing")
	}
	run(t, f, input{}, 1)
	if !f.leaving || f.to != titleMenu || f.sess.level != 50 || f.res != screen.Typing {
		t.Errorf("empty field: leaving %v to %d, level %d, result %d", f.leaving, f.to, f.sess.level, f.res)
	}

	for _, c := range []struct {
		row   int
		res   screen.CodeResult
		level int
	}{{37, screen.Found, 37}, {119, screen.Found, 119}, {0, screen.Incorrect, 0}, {120, screen.ShowMode, 50}, {121, screen.CheatMode, 50}} {
		f.sess.level = 50
		f.enter(codeEntry)
		code := "ABCDEFGHIJ"
		if c.row > 0 {
			code = f.menu.Code(c.row)
		}
		if f.menu.Match([10]byte([]byte(code))) != c.row {
			t.Fatalf("test code for row %d matches another row", c.row)
		}
		typeKeys(t, f, code)
		run(t, f, input{left: true}, 2)
		if f.res != screen.Typing {
			t.Errorf("row %d: matched within two frames of finishing", c.row)
		}
		run(t, f, input{left: true}, 1)
		if f.res != c.res || f.sess.level != c.level {
			t.Errorf("row %d: result %d, level %d; want %d, %d", c.row, f.res, f.sess.level, c.res, c.level)
		}
		run(t, f, input{left: true}, 5)
		if f.leaving {
			t.Errorf("row %d: the finishing click left the screen", c.row)
		}
		run(t, f, input{}, 1)
		run(t, f, input{middle: true}, 1)
		if !f.leaving || f.to != titleMenu {
			t.Errorf("row %d: a mouse button did not return to the title", c.row)
		}
	}
	if !f.sess.show || !f.sess.cheat {
		t.Errorf("show %v, cheat %v after their codes", f.sess.show, f.sess.cheat)
	}
}

// TestPreLevelAndPlay's oracle is the pre-level screen and progression rule: a mouse button pressed
// after all were up starts the level; the level starts with the session's volume setting, mode,
// speaker and nuke record; the pointer is limited to rows 80-174 and play starts with the centre 16
// rows below it.
func TestPreLevelAndPlay(t *testing.T) {
	f := testFlow(t)
	f.sess = session{level: 3, effects: true, volume: 99, quiet: true, nukeRecord: 7}
	if err := f.startLevel(); err != nil {
		t.Fatal(err)
	}
	until(t, f, input{left: true}, preLevel, 100)
	f.my = 0
	run(t, f, input{left: true}, 40)
	if f.ph != preLevel || f.leaving || f.my != 80 {
		t.Errorf("held button started the level, or pointer at row %v", f.my)
	}
	run(t, f, input{}, 1)
	run(t, f, input{right: true}, 1)
	until(t, f, input{}, playing, 100)
	s := f.play.sim
	if !s.Effects || s.Volume != 99 || !s.Quiet || s.NukeRecord != 7 || f.play.n != 3 {
		t.Errorf("level starts with effects %v, volume %d, quiet %v, nuke record %d, level %d",
			s.Effects, s.Volume, s.Quiet, s.NukeRecord, f.play.n)
	}

	f.sess.cheat = true
	if err := f.startLevel(); err != nil {
		t.Fatal(err)
	}
	until(t, f, input{space: true}, preLevel, 100)
	run(t, f, input{space: true}, 2)
	run(t, f, input{space: true, left: true}, 2)
	if f.ph != preLevel || f.leaving {
		t.Error("cheat mode: the pre-level screen reacted while Space was held")
	}
	run(t, f, input{}, 1)
	run(t, f, input{left: true}, 1)
	if !f.leaving {
		t.Error("cheat mode: no start after Space was released")
	}
}

// TestLevelEnd's oracle is the level-ending and results screen rule: Escape ends the level as
// time-up; it fades out while the ending runs and the results screen follows, "Your time is up!",
// needed and rescued percentages; the session keeps the volume setting. Failed, the left button
// held from the level retries at once; the right button returns to the title with the level kept.
func TestLevelEnd(t *testing.T) {
	f := testFlow(t)
	f.sess.level = 5
	if err := f.startLevel(); err != nil {
		t.Fatal(err)
	}
	until(t, f, input{}, preLevel, 100)
	run(t, f, input{}, 1)
	run(t, f, input{left: true}, 1)
	until(t, f, input{}, playing, 100)
	run(t, f, input{f2: true}, 3*10) // the setting is already 255: stays
	run(t, f, input{f1: true}, 3*10)
	run(t, f, input{escape: true}, 1)
	n := until(t, f, input{}, results, 200)
	if n < 3*16-3 || n > 3*16+3 {
		t.Errorf("results after %d frames, want about 16 updates", n)
	}
	if !f.result.TimeUp || f.result.Rescued != 0 || f.result.Needed != f.lvl.ToSave*100/f.lvl.Lemmings || f.result.Passed() {
		t.Errorf("results %+v", f.result)
	}
	if f.sess.volume != 255-30 {
		t.Errorf("session volume %d, want %d", f.sess.volume, 255-30)
	}
	run(t, f, input{left: true}, 1)
	if !f.leaving || f.to != preLevel || f.sess.level != 5 {
		t.Errorf("left held: leaving %v to %d, level %d", f.leaving, f.to, f.sess.level)
	}

	f.enter(results)
	run(t, f, input{right: true}, 1)
	until(t, f, input{}, titleMenu, 100)
	if f.sess.level != 5 {
		t.Errorf("right button: level %d", f.sess.level)
	}
}

// TestPassed's oracle is the results screen and progression rule: passed, a mouse button pressed
// after all were up opens the next level's pre-level screen, crossing into the next rating; after
// the last level the congratulations screen follows, then the title with level 0. On a pass results
// screen the game already holds the next level; the results are opened here as the
// game holds them.
func TestPassed(t *testing.T) {
	f := testFlow(t)
	f.sess.level = 30
	f.result = screen.Results{Needed: 50, Rescued: 50, Next: 30}
	f.enter(results)
	run(t, f, input{left: true}, 20)
	if f.ph != results || f.leaving {
		t.Error("a button held from the level dismissed the passed results")
	}
	run(t, f, input{}, 1)
	run(t, f, input{left: true}, 1)
	until(t, f, input{}, preLevel, 100)
	if f.sess.level != 30 {
		t.Errorf("level %d after passing 29", f.sess.level)
	}

	f.sess.level = 120
	f.result = screen.Results{Needed: 50, Rescued: 60, Next: 120}
	f.enter(results)
	run(t, f, input{}, 1)
	run(t, f, input{right: true}, 1)
	until(t, f, input{}, congrats, 100)
	run(t, f, input{left: true}, 5)
	run(t, f, input{}, 1)
	run(t, f, input{left: true}, 1)
	until(t, f, input{}, titleMenu, 100)
	if f.sess.level != 0 {
		t.Errorf("title after the last level at level %d", f.sess.level)
	}

	// Passing lastLevel: the game past it is not followed; the title, level kept.
	f.sess.level = lastLevel
	f.result = screen.Results{Needed: 50, Rescued: 60, Next: lastLevel + 1}
	f.enter(results)
	run(t, f, input{}, 1)
	run(t, f, input{left: true}, 1)
	until(t, f, input{}, titleMenu, 100)
	if f.sess.level != lastLevel {
		t.Errorf("title after passing level %d at level %d", lastLevel, f.sess.level)
	}
}

// TestCheatSkip's oracle is the cheat-mode skip rule: in cheat mode Space in play waits for its
// release, then goes to the next level's pre-level screen with no results; past the last Mayhem
// level as far as lastLevel, not beyond.
func TestCheatSkip(t *testing.T) {
	f := testFlow(t)
	f.sess.cheat = true
	f.sess.level = 10
	if err := f.startLevel(); err != nil {
		t.Fatal(err)
	}
	until(t, f, input{}, preLevel, 100)
	run(t, f, input{}, 1)
	run(t, f, input{left: true}, 1)
	until(t, f, input{}, playing, 100)
	run(t, f, input{}, 30)
	u := f.play.sim.Update
	run(t, f, input{space: true}, 30)
	if f.play.sim.Update != u || f.leaving {
		t.Errorf("level ran or left while Space was held")
	}
	until(t, f, input{}, preLevel, 100)
	if f.sess.level != 11 {
		t.Errorf("skipped to level %d", f.sess.level)
	}

	for _, c := range []struct{ from, to int }{{119, 120}, {lastLevel, lastLevel}} {
		f.sess.level = c.from
		if err := f.startLevel(); err != nil {
			t.Fatal(err)
		}
		for f.leaving {
			run(t, f, input{}, 1)
		}
		run(t, f, input{}, 1)
		run(t, f, input{left: true}, 1)
		until(t, f, input{}, playing, 100)
		run(t, f, input{space: true}, 3)
		run(t, f, input{}, 1)
		for f.leaving {
			run(t, f, input{}, 1)
		}
		if f.sess.level != c.to {
			t.Errorf("Space at level %d: level %d, want %d", c.from, f.sess.level, c.to)
		}
	}
}

// TestFadePhase's oracle is the fade rule: steps fall on alternate frames counted from program
// start, never re-aligned to the fade, so the first comes one or two frames after a fade begins.
func TestFadePhase(t *testing.T) {
	f := testFlow(t)
	for _, odd := range []bool{false, true} {
		openTitle(t, f)
		if odd == (f.frame%2 == 1) {
			run(t, f, input{}, 1)
		}
		// The click lands on the next frame: odd or even.
		f.mx, f.my = 150+16, 160
		dark := f.dark
		run(t, f, input{left: true}, 1)
		start := f.frame
		if f.dark != dark {
			t.Errorf("fade begun on frame %d stepped on that frame", start)
		}
		first := 0
		for first == 0 {
			run(t, f, input{}, 1)
			if f.dark != dark {
				first = f.frame - start
			}
		}
		if want := map[bool]int{true: 1, false: 2}[start%2 == 1]; first != want || f.frame%2 != 0 {
			t.Errorf("fade begun on frame %d: first step %d frames later (frame %d), want %d", start, first, f.frame, want)
		}
		until(t, f, input{}, codeEntry, 100)
		run(t, f, input{}, 1)
		run(t, f, input{}, 1)
	}
}

// TestCodeFinish's oracle is the access-code screen's finish rule: a left button still held from
// the NEW LEVEL click finishes the field; with it empty the title reopens as soon as the button is
// released. The cursor line stays for two frames after finishing.
func TestCodeFinish(t *testing.T) {
	f := testFlow(t)
	openTitle(t, f)
	f.mx, f.my = 150+16, 160
	until(t, f, input{left: true}, codeEntry, 100)
	run(t, f, input{left: true}, 60)
	if f.leaving || f.ph != codeEntry || !f.empty {
		t.Fatalf("held left button: leaving %v, phase %d, finished empty %v", f.leaving, f.ph, f.empty)
	}
	run(t, f, input{}, 1)
	if !f.leaving || f.to != titleMenu {
		t.Error("release did not reopen the title")
	}
}

// TestFadePointer's oracle is the fade-out pointer rule stated on fadePointer.
func TestFadePointer(t *testing.T) {
	f := testFlow(t)
	for _, c := range []struct {
		from, to   phase
		show, busy bool
		name       string
	}{
		{titleMenu, preLevel, true, true, "START"},
		{titleMenu, codeEntry, false, false, "NEW LEVEL"},
		{codeEntry, titleMenu, false, false, "out of the code screen"},
		{results, preLevel, true, true, "results to pre-level"},
		{results, titleMenu, true, false, "results' right button"},
		{results, congrats, false, false, "into congratulations"},
		{congrats, titleMenu, false, false, "out of congratulations"},
	} {
		f.ph, f.to, f.leaving = c.from, c.to, true
		if show, busy := f.fadePointer(); show != c.show || show && busy != c.busy {
			t.Errorf("%s: pointer %v busy %v, want %v %v", c.name, show, busy, c.show, c.busy)
		}
	}
	f.ph, f.to, f.begin = preLevel, playing, f.frame
	if show, busy := f.fadePointer(); !show || !busy {
		t.Error("pre-level click frame: no busy pointer")
	}
	f.frame++
	if show, _ := f.fadePointer(); show {
		t.Error("pre-level fade-out: pointer after the click frame")
	}
}

// TestReturnToDesktop's oracle is the title menu's quit rule: from program start, past the notice
// and splashes, a click on RETURN TO DESKTOP only turns the border red; a second click within 59
// frames quits the game. Then, as the disc's !Run echoes its last Echo line, the farewell shows
// that line at once (no fade) on RISC OS's 640-wide screen for 150 frames, or until a fresh click,
// and only then sets quit, which Update turns into ebiten.Termination.
func TestReturnToDesktop(t *testing.T) {
	f := testFlow(t)
	run(t, f, input{left: true}, 1) // ends the notice
	until(t, f, input{}, splashing, 100)
	run(t, f, input{left: true}, 1) // skips the splashes
	until(t, f, input{}, titleMenu, 300)
	run(t, f, input{}, 40)
	clickAt(t, f, 97, 160)
	if v, _ := f.view(); f.quit || v.border != (color.RGBA{0xFF, 0, 0, 0xFF}) {
		t.Fatalf("one click: quit %v, border %v; want no quit, red border", f.quit, v.border)
	}
	run(t, f, input{}, 10)
	f.mx, f.my = 97+16, 160
	run(t, f, input{left: true}, 1)
	bye := farewellLine(f.disc.Files["/!Lemmings/!Run"])
	if f.ph != farewell || f.quit || f.bye != bye || bye == "" {
		t.Fatalf("second click: phase %d, quit %v, line %q; want the farewell %q", f.ph, f.quit, f.bye, bye)
	}
	v, err := f.view()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(v.scr.Pix, textScreen(bye)) || !slices.Equal(v.scr.Upper, shade(farewellPal, 0)) ||
		v.w != wideW || v.border != color.Black {
		t.Error("farewell is not the line on the 640-wide screen in the captured colour, unfaded, on black")
	}
	run(t, f, input{left: true}, 20)
	if f.quit {
		t.Error("the quitting click, still held, skipped the farewell")
	}
	run(t, f, input{}, farewellHold-21)
	if f.quit {
		t.Errorf("quit before %d frames", farewellHold)
	}
	run(t, f, input{}, 1)
	if !f.quit {
		t.Errorf("no quit after %d frames", farewellHold)
	}

	f.quit = false
	f.enter(farewell)
	run(t, f, input{}, 5)
	run(t, f, input{right: true}, 1)
	if !f.quit {
		t.Error("a click did not skip the farewell")
	}
}

// TestFarewellLine's oracle is RISC OS Obey syntax: one command per line, command names in any
// case; the farewell is the text of the last Echo command.
func TestFarewellLine(t *testing.T) {
	for _, c := range []struct{ obey, want string }{
		{"Set A$B 1\necho <13>\nEcho  So long, and thanks  \nUnset A$B\n\n   ", "So long, and thanks"},
		{"ECHO first\r\n  echo second\r\nRun X\r\n", "second"},
		{"Echoes not a command\nEcho kept\nEchoes still not\n", "kept"},
		{"Set A 1\nRun B\n", ""},
		{"", ""},
	} {
		if got := farewellLine([]byte(c.obey)); got != c.want {
			t.Errorf("farewellLine(%q) = %q, want %q", c.obey, got, c.want)
		}
	}
}
