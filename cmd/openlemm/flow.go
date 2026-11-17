package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"strings"

	"github.com/benemon/openlemm/internal/disc"
	"github.com/benemon/openlemm/internal/level"
	"github.com/benemon/openlemm/internal/screen"
	"github.com/benemon/openlemm/internal/sim"
)

// input is the mouse and keyboard as read at one display frame.
type input struct {
	dx, dy              float64 // pointer motion since the last frame, in screen pixels
	left, right, middle bool
	letters             uint32 // bit i: the key for letter A + i held
	back, del           bool   // Backspace, Delete held
	ret, enter          bool   // Return, keypad Enter held
	space               bool
	f1, f2, f5          bool
	escape              bool // Escape pressed since the last frame
}

func (in input) anyButton() bool { return in.left || in.right || in.middle }

// codeKeys reports whether any key the access-code screen reads is held.
func (in input) codeKeys() bool {
	return in.letters != 0 || in.back || in.del || in.ret || in.enter || in.left
}

// lastLevel is the last level the game goes to past Mayhem 30 (level 119), by the cheat skip and by
// passing levels after it (levels 120-159 replay the last Mayhem level, 160-179 are the two-player
// levels played alone). Level 180 and beyond read past the level-order table.
const lastLevel = level.Levels - 1

type phase int

const (
	splashing phase = iota
	waiting         // black, after the splashes
	titleMenu
	codeEntry
	preLevel
	playing
	results
	congrats
	notice   // before the splashes
	farewell // after RETURN TO DESKTOP: the line !Run echoes once the game has exited
)

// farewellHold is how many frames the farewell stays up unless a button skips it: 3 s.
const farewellHold = 150

// The farewell is drawn as RISC OS prints it after the game: mode 12, 640 × 256, 80 columns of
// 8 × 8 characters in farewellPal's colour 1, on text row 4: !Run's two "Echo <13> <10>" lines
// leave four line feeds below the top-left, where the game is taken to leave the cursor.
const (
	wideW         = 2 * screenW
	farewellRow   = 4
	farewellColor = 1
)

// farewellPal is black and the text colour seen on the original's screen after it has exited (a
// capture of the running game, about (177, 72, 38)), taken to the nearest 12-bit colour &A42:
// RISC OS state after the game, not the game's data.
var farewellPal = color.Palette{color.Black, color.RGBA{0xaa, 0x44, 0x22, 255}}

// textScreen draws lines in sysFont, each from column 0 of a new text row starting at row
// farewellRow, wrapping at 80 columns as the VDU does; codes outside 32-126 take a cell and draw
// nothing.
func textScreen(lines ...string) []uint8 {
	const cols = wideW / 8
	pix := make([]uint8, wideW*screenH)
	top := farewellRow
	for _, line := range lines {
		for i, c := range []byte(line) {
			col, row := i%cols, top+i/cols
			if c < 32 || c > 126 || row >= screenH/8 {
				continue
			}
			for y, bits := range sysFont[c-32] {
				for x := range 8 {
					if bits>>x&1 != 0 {
						pix[(8*row+y)*wideW+8*col+x] = farewellColor
					}
				}
			}
		}
		top += max(len(line)+cols-1, cols) / cols
	}
	return pix
}

// farewellLine returns the text of the last Echo command in an Obey script, or "" if it has none.
func farewellLine(obey []byte) string {
	line := ""
	for _, l := range strings.Split(string(obey), "\n") {
		if cmd, text, ok := strings.Cut(strings.TrimSpace(l), " "); ok && strings.EqualFold(cmd, "Echo") {
			line = strings.TrimSpace(text)
		}
	}
	return line
}

// noticeHold is how many frames the notice stays up unless a button skips it: 5 s.
const noticeHold = 250

// noticeText is the start-up notice; its wording is fixed.
var noticeText = []string{
	"OpenLemm is a tribute to Acorn Archimedes Lemmings.",
	"It is not maintained, supported, or endorsed by the original publishers.",
}

// session is what the game keeps from screen to screen.
type session struct {
	level      int // 0-119 in play order
	effects    bool
	volume     int
	quiet      bool
	show       bool
	cheat      bool
	nukeRecord int
}

// assets is what the screens read from the disc.
type assets struct {
	disc   *disc.Disc
	menu   *screen.Menu
	art    *art
	tables *sim.Tables
}

// flow is the game from the splash screens on, one display frame (50 Hz) per update.
type flow struct {
	*assets
	sess  session
	sound *mixer // nil without audio
	pan   bool
	quit  bool
	bye   string // the farewell line, once RETURN TO DESKTOP has quit
	only  bool   // the level was chosen by record and is outside the play order: the game ends with it

	ph     phase
	mx, my float64 // the mouse: x 0-319; the menu pointer's top-left is (mx - 16, my)

	// Fades: shown components are max(0, c - dark).
	dark    int
	frame   int   // display frames since the program started; fade steps fall on even ones
	begin   int   // the frame the fade began
	leaving bool  // fading out
	to      phase // the phase after the fade-out
	bright  int   // the brightest component of the current screen's palettes

	splash  int  // splash shown, 0-2 (Krisalis, Psygnosis, DMA Design)
	hold    int  // frames of the splash's hold
	skip    bool // a button ended a hold: no more splashes
	waitN   int  // frames of black after the splashes
	title   *screen.Title
	armed   bool // the left button was seen up since the last click
	border  int  // frames the red border has left
	field   [10]byte
	place   int
	finish  int  // frames left before a finished field is matched
	empty   bool // the field was finished empty: back to the title once the keys are up
	gate    bool // a code key acted: wait for all of them to be up
	res     screen.CodeResult
	allUp   bool // every mouse button was seen up since the screen opened
	spaceUp bool // Space was seen up since the screen opened (pre-level, cheat mode)
	held    bool // Space is held in play, cheat mode
	lvl     *level.Level
	still   *screen.Screen // the screen being shown, when it does not change
	play    *play
	result  screen.Results
}

func newFlow(a *assets, sound *mixer) *flow {
	f := &flow{assets: a, sound: sound, mx: screenW / 2, my: screenH / 2,
		sess: session{volume: 255}, splash: -1}
	f.enter(notice)
	return f
}

// enter opens phase ph, black, with its fade-in starting.
func (f *flow) enter(ph phase) {
	f.ph, f.leaving, f.dark, f.begin, f.allUp, f.still = ph, false, 15, f.frame, false, nil
	switch ph {
	case notice:
		f.hold = 0
		f.still = f.menu.Notice(screen.Layout(noticeText, viewTop, menuH))
		f.bright = brightest(f.still.Upper)
	case farewell:
		// Printed by the OS after the game has gone: no fade.
		f.hold, f.dark = 0, 0
		f.still = &screen.Screen{Pix: textScreen(f.bye), Upper: farewellPal, Lower: farewellPal,
			Split: screenH}
	case splashing:
		f.splash++
		f.hold = 0
		f.bright = brightest(f.menu.Splash(f.splash).Upper)
	case titleMenu:
		f.title = f.menu.NewTitle()
		f.armed, f.border = false, 0
		f.bright = brightest(f.menu.TitlePal)
	case codeEntry:
		f.field, f.place, f.gate, f.res = [10]byte([]byte("----------")), 0, false, screen.Typing
		f.finish, f.empty = 0, false
		f.bright = brightest(f.menu.TitlePal)
	case preLevel:
		f.spaceUp = !f.sess.cheat
		f.still = f.menu.PreLevel(f.lvl, f.sess.level)
		f.bright = brightest(f.still.Upper, f.still.Lower)
	case playing:
		n := f.sess.level
		if f.only {
			n = -1
		}
		f.beginPlay(n)
	case results:
		f.still = f.menu.ResultsScreen(f.result)
		f.bright = brightest(f.menu.TitlePal)
	case congrats:
		f.still = f.menu.Congratulations()
		f.bright = brightest(f.menu.TitlePal)
	}
}

// leave starts the fade-out to phase ph.
func (f *flow) leave(ph phase) {
	f.leaving, f.to, f.begin = true, ph, f.frame
}

// startLevel loads the current level for the pre-level screen and the level after it.
func (f *flow) startLevel() error {
	l, err := level.LoadPlay(f.disc, f.sess.level)
	if err != nil {
		return err
	}
	f.lvl = l
	f.leave(preLevel)
	return nil
}

// clampPointer keeps the mouse where the current screen allows it.
func (f *flow) clampPointer() {
	lo, hi := 0.0, 174.0
	switch f.ph {
	case preLevel:
		lo = 80
	case playing:
		// The pointer centre is (mx, my + 16), on screen rows 16-222.
		hi = 206
	}
	f.mx = min(max(f.mx, 0), screenW-1)
	f.my = min(max(f.my, lo), hi)
}

// update runs one display frame.
func (f *flow) update(in input) error {
	f.frame++
	f.mx, f.my = f.mx+in.dx, f.my+in.dy
	f.clampPointer()
	if !in.anyButton() {
		f.allUp = true
	}
	if !in.left {
		f.armed = true
	}
	if !f.leaving || f.ending() {
		if err := f.work(in); err != nil {
			return err
		}
	}
	f.fade()
	return nil
}

// The results screen's fade-in begins resultsFade frames after the level is left, so its first
// step falls 9 or 10 frames after; its pointer is on screen from resultsPointer frames after.
const resultsFade, resultsPointer = 8, 12

// fade advances the fade-in or fade-out by one frame. Steps fall on alternate frames counted
// from program start, not from the fade's start, so the first comes one or two frames after the
// fade begins; that they are the even frames is a choice.
func (f *flow) fade() {
	if f.frame <= f.begin || f.frame%2 != 0 {
		return
	}
	switch {
	case f.leaving && f.dark >= f.bright:
		// The first step that finds everything black ends the fade-out; a level ends when its
		// ending sequence is over.
		if !f.ending() {
			f.enter(f.to)
		}
	case f.leaving:
		f.dark++
	case f.dark > 0:
		f.dark--
	}
}

// ending reports whether the level's ending sequence is running, with its fade.
func (f *flow) ending() bool { return f.ph == playing && f.leaving && f.to == results }

// work is the current screen's own work for one frame.
func (f *flow) work(in input) error {
	switch f.ph {
	case farewell:
		if f.hold++; f.hold >= farewellHold || f.allUp && in.anyButton() {
			f.quit = true
		}
	case notice:
		// Held and skipped as a splash is, but a skip ends only the notice.
		if f.hold++; f.hold >= noticeHold || in.anyButton() {
			f.leave(splashing)
		}
	case splashing:
		f.hold++
		if in.anyButton() {
			f.skip = true
		}
		switch {
		case !f.skip && f.hold < 240:
		case f.skip || f.splash == 2:
			f.leave(waiting)
		default:
			f.leave(splashing)
		}
	case waiting:
		// The loader counts frames over 2 seconds to choose the timing mode; at 50 Hz it stays in
		// the first.
		f.waitN++
		if f.waitN == 100 {
			f.enter(titleMenu)
		}
	case titleMenu:
		return f.titleWork(in)
	case codeEntry:
		f.codeWork(in)
	case preLevel:
		if !f.spaceUp {
			f.spaceUp = !in.space
			break
		}
		if f.allUp && in.anyButton() {
			f.leave(playing)
		}
	case playing:
		return f.playWork(in)
	case results:
		switch {
		case f.result.Passed() && f.allUp && in.anyButton():
			switch {
			case f.result.Next == 120:
				f.leave(congrats)
			case f.result.Next > lastLevel:
				// Past lastLevel the game is not followed: back to the title, level kept.
				f.leave(titleMenu)
			default:
				return f.startLevel()
			}
		case f.result.Passed():
		case in.left:
			return f.startLevel()
		case in.right:
			f.leave(titleMenu)
		}
	case congrats:
		if f.allUp && in.anyButton() {
			f.sess.level = 0
			f.leave(titleMenu)
		}
	}
	return nil
}

// titleWork animates the title and answers a left click on its boards.
func (f *flow) titleWork(in input) error {
	f.menu.Step(f.title)
	if f.border > 0 {
		f.border--
	}
	if !in.left || !f.armed {
		return nil
	}
	f.armed = false
	x, y := int(f.mx)-16, int(f.my)
	if y < 148 || y > 174 {
		return nil
	}
	rating := f.sess.level / 30
	switch {
	case x >= 6 && x <= 61:
		return f.startLevel()
	case x >= 70 && x <= 125:
		switch {
		case f.sess.show:
		case f.border > 0:
			if f.bye = farewellLine(f.disc.Files["/!Lemmings/!Run"]); f.bye == "" {
				f.quit = true
			} else {
				f.enter(farewell)
			}
		default:
			f.border = 60
		}
	case x >= 134 && x <= 185:
		f.leave(codeEntry)
	case x >= 194 && x <= 247:
		f.sess.effects = !f.sess.effects
	case x >= 292 && x <= 303 && y <= 168 && rating < 3:
		f.sess.level = 30 * (rating + 1)
	case x >= 292 && x <= 303 && y >= 169 && rating > 0:
		f.sess.level = 30 * (rating - 1)
	}
	return nil
}

// codeWork reads the access-code keys, or after the result waits for a mouse button.
func (f *flow) codeWork(in input) {
	switch {
	case f.res != screen.Typing:
		if f.allUp && in.anyButton() {
			f.leave(titleMenu)
		}
		return
	case f.empty:
		if !in.codeKeys() {
			f.leave(titleMenu)
		}
		return
	case f.finish > 0:
		if f.finish--; f.finish == 0 {
			f.match()
		}
		return
	}
	if f.gate {
		f.gate = in.codeKeys()
		return
	}
	if !in.codeKeys() {
		return
	}
	f.gate = true
	switch {
	case in.letters != 0:
		i := 0
		for in.letters&(1<<i) == 0 {
			i++
		}
		f.field[f.place] = byte('A' + i)
		f.place = min(f.place+1, 9)
	case in.back || in.del:
		if f.place == 9 && f.field[9] != '-' {
			f.field[9] = '-'
		} else {
			f.place = max(f.place-1, 0)
			f.field[f.place] = '-'
		}
	default:
		f.finish = 2
	}
}

// match ends typing two frames after a finishing key: an empty field returns to the title once
// the code keys are up; otherwise the cursor line is blanked and the field matched.
func (f *flow) match() {
	if f.field == [10]byte([]byte("----------")) {
		f.empty = true
		return
	}
	f.allUp = false
	switch r := f.menu.Match(f.field); {
	case r == 0:
		f.res, f.sess.level = screen.Incorrect, 0
	case r < 120:
		f.res, f.sess.level = screen.Found, r
	case r == 120:
		f.res, f.sess.show = screen.ShowMode, true
	default:
		f.res, f.sess.cheat = screen.CheatMode, true
	}
}

// beginPlay starts the level loaded for the pre-level screen, with the session's settings.
func (f *flow) beginPlay(n int) {
	p := newPlay(f.lvl, f.tables, n)
	p.sim.Effects, p.sim.Volume = f.sess.effects, f.sess.volume
	p.sim.Quiet, p.sim.NukeRecord = f.sess.quiet, f.sess.nukeRecord
	f.play, f.held = p, false
	f.bright = brightest(append(color.Palette{color.Black}, f.lvl.Map.Palette[1:]...), f.art.panel.Palette())
	if f.sound != nil && n >= 0 {
		t, err := levelTune(f.disc, n)
		if err != nil {
			log.Printf("no music: %v", err)
		}
		f.sound.play(t)
	}
}

// endPlay keeps what the session carries from the level and stops its music.
func (f *flow) endPlay() {
	s := f.play.sim
	f.sess.volume, f.sess.quiet, f.sess.nukeRecord = s.Volume, s.Quiet, s.NukeRecord
	if f.sound != nil {
		f.sound.play(nil)
	}
}

// playWork runs the level for one frame and, once it is over, opens the results screen.
func (f *flow) playWork(in input) error {
	p := f.play
	if f.sess.cheat && p.n >= 0 && p.n < lastLevel {
		// Space waits for its release, then skips to the next level.
		if in.space {
			f.held = true
			return nil
		}
		if f.held {
			f.endPlay()
			f.sess.level = p.n + 1
			return f.startLevel()
		}
	}
	p.update(in, int(f.mx), int(f.my)+16, f.sound, f.pan)
	s := p.sim
	if s.Ending() && !f.leaving {
		f.leave(results)
	}
	if !s.Over {
		return nil
	}
	f.levelOver()
	return nil
}

// levelOver leaves the level for its results screen once its ending sequence is over.
func (f *flow) levelOver() {
	p, s := f.play, f.play.sim
	f.endPlay()
	if p.n < 0 {
		f.quit = true
		return
	}
	n := max(f.lvl.Lemmings, 1)
	f.result = screen.Results{TimeUp: s.TimeUp, Needed: f.lvl.ToSave * 100 / n, Rescued: s.Saved * 100 / n,
		Next: p.n + 1}
	if f.result.Passed() && f.result.Next <= max(lastLevel, 120) {
		f.sess.level = f.result.Next // passing moves to the next level at once
	}
	f.enter(results)
	f.begin += resultsFade
}

// view is one frame as shown: the screen with its palettes faded, the rows shown and the colour
// around them.
type view struct {
	scr    *screen.Screen
	w      int // screen width: 320, or 640 with pixels twice as tall as wide
	top    int // first screen row shown
	rows   int
	border color.Color
}

func (f *flow) view() (view, error) {
	v := view{w: screenW, top: viewTop, rows: menuH, border: color.Black}
	var s *screen.Screen
	pointer, busy := true, false
	switch f.ph {
	case splashing:
		s, pointer = f.menu.Splash(f.splash), false
	case notice:
		pointer = false
	case farewell:
		v.w, pointer = wideW, false
	case waiting:
		black := color.Palette{color.Black}
		s, pointer = &screen.Screen{Pix: make([]uint8, screenW*screenH), Upper: black, Lower: black, Split: screenH}, false
	case titleMenu:
		s = f.menu.TitleScreen(f.title, f.sess.level/30, f.sess.effects)
		if f.border > 0 {
			v.border = color.RGBA{0xFF, 0, 0, 0xFF}
		}
	case codeEntry:
		s, pointer = f.menu.AccessCode(f.field, f.place, f.res, f.sess.level), false
	case playing:
		var err error
		if s, err = f.play.screen(f.art, image.Pt(int(f.mx), int(f.my)+16)); err != nil {
			return v, err
		}
		v.rows, pointer = shownH, false
	case results:
		pointer = !(f.result.Passed() && f.result.Next == 120) && f.frame-f.begin >= resultsPointer-resultsFade
	case congrats:
		pointer = false
	}
	if f.leaving && !f.ending() {
		pointer, busy = f.fadePointer()
	}
	if s == nil {
		c := *f.still
		c.Pix = append([]uint8(nil), c.Pix...)
		s = &c
	}
	if pointer {
		f.menu.Pointer(s, int(f.mx)-16, int(f.my), busy)
	}
	s.Upper, s.Lower = shade(s.Upper, f.dark), shade(s.Lower, f.dark)
	if f.ph == splashing {
		v.border = s.Upper[0]
	}
	v.scr = s
	return v, nil
}

// fadePointer returns whether the menu pointer shows during the current fade-out, and whether as
// the busy image. A cheat skip's fade-out keeps the in-play pointer, which the level's own screen
// draws.
func (f *flow) fadePointer() (show, busy bool) {
	switch {
	case f.to == preLevel && (f.ph == titleMenu || f.ph == results):
		return true, true
	case f.ph == results && f.to == titleMenu:
		return true, false
	case f.ph == preLevel:
		// The busy image shows for the click's frame only.
		return f.frame == f.begin, true
	}
	return false, false
}

// shade returns p with each 4-bit colour component lowered by dark, not below 0.
func shade(p color.Palette, dark int) color.Palette {
	q := make(color.Palette, len(p))
	for i, c := range p {
		r, g, b, _ := c.RGBA()
		d := func(v uint32) uint8 { return uint8(max(int(v>>12)-dark, 0) * 17) }
		q[i] = color.RGBA{d(r), d(g), d(b), 0xFF}
	}
	return q
}

// brightest returns the largest 4-bit colour component in ps.
func brightest(ps ...color.Palette) int {
	m := 0
	for _, p := range ps {
		for _, c := range p {
			r, g, b, _ := c.RGBA()
			m = max(m, int(r>>12), int(g>>12), int(b>>12))
		}
	}
	return m
}

// levelTune loads the tune of the level at play-order index n, as MUSINFO names it.
func levelTune(d *disc.Disc, n int) (*tracker, error) {
	mod, start, err := level.Music(d, n)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/!Lemmings/Lemmings/LemLevels/LEMMUS%d", mod)
	data, ok := d.Files[path]
	if !ok {
		return nil, fmt.Errorf("%s: not on disc", path)
	}
	return newTracker(data, start)
}
