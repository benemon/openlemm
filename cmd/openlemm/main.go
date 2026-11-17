// Command openlemm plays the game from the player's disc image.
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"math"
	"math/rand/v2"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/benemon/openlemm/internal/disc"
	"github.com/benemon/openlemm/internal/level"
	"github.com/benemon/openlemm/internal/screen"
	"github.com/benemon/openlemm/internal/sim"
)

const (
	screenW, screenH = 320, 256
	shownH           = 208 // in play the window shows screen rows 16-223
	menuH            = 216 // elsewhere rows 16-231: the title's scroller reaches row 231
	viewTop          = 16  // screen row of level row 0
	borderW          = 8   // screen pixels of border colour around the menus in a new window
	viewRows         = 160 // level rows 0-159 on screen rows 16-175
	panelTop         = 184
	mapW             = 1632
	maxScroll        = 1280

	crossAddr    = 0x22BDC // unpacked offsets in LemProg
	bracketsAddr = 0x22ADC
	maskOff      = 0x80
	hotspot      = 8 // pointer image pixel at the pointer position
	particleAddr = 0x3AEC
	fuseAddr     = 0x0F20
	fuseColour   = 3
)

// edgeStep is the scroll step for the pointer at screen (sx, sy): 4 pixels, 16 with the right
// button held, at the left or right screen edge with the left button up, on rows 16-200.
func edgeStep(sx, sy int, left, right bool) int {
	if left || sy > 200 {
		return 0
	}
	d := 4
	if right {
		d = 16
	}
	switch sx {
	case 0:
		return -d
	case screenW - 1:
		return d
	}
	return 0
}

// minimapScroll is the scroll a click at screen (sx, sy) gives, and whether it is on the minimap's
// strip: the button rows 200-222 from column 208, wider and taller than the drawn minimap.
func minimapScroll(sx, sy int) (int, bool) {
	if sx < 208 || sy < 200 {
		return 0, false
	}
	return min(max(sx-218, 0), 80) * 16, true
}

// windowScale is the largest integer scale at which the menus' rows and their border fit a w × h
// display with room for the window frame, menu bar and dock.
func windowScale(w, h int) int {
	return max(min(w*9/10/(screenW+2*borderW), h*85/100/(menuH+2*borderW)), 1)
}

// releaseMod is the key held with Backspace to release the mouse: Cmd on macOS, Ctrl elsewhere
// (Windows takes Ctrl-Escape for the Start menu).
func releaseMod(goos string) ebiten.Key {
	if goos == "darwin" {
		return ebiten.KeyMeta
	}
	return ebiten.KeyControl
}

// backspaceAction says what Backspace does: with the release modifier it releases the mouse and
// the game never sees it; plain, it reaches the game, which reads it on the access-code screen.
func backspaceAction(mod bool) (release, toGame bool) {
	return mod, !mod
}

// fit is the largest integer scale, at least 1, at which 320 × rows fit a w × h area, and the
// offsets that centre them there.
func fit(w, h, rows int) (s, ox, oy int) {
	s = max(min(w/screenW, h/rows), 1)
	return s, (w - screenW*s) / 2, (h - rows*s) / 2
}

// fitWide is fit for a 640-pixel-wide screen whose pixels are twice as tall as wide: the largest
// even vertical scale vs, at least 2, at which it fits, horizontal scale vs/2, and the offsets.
func fitWide(w, h, rows int) (hs, vs, ox, oy int) {
	s, _, _ := fit(w, h, rows)
	vs = max(s-s%2, 2)
	hs = vs / 2
	return hs, vs, (w - wideW*hs) / 2, (h - rows*vs) / 2
}

// toMap converts a screen position to map coordinates for a view whose left edge is map x scroll.
func toMap(sx, sy, scroll int) (px, py int) {
	return scroll + sx, sy - viewTop + 4
}

// scrollStep moves scroll by d, refusing a fast step that would pass a limit.
func scrollStep(scroll, d int) int {
	n := scroll + d
	if (n < 0 || n > maxScroll) && (d == 16 || d == -16) {
		return scroll
	}
	return min(max(n, 0), maxScroll)
}

// sprite is a LemGrafP block: offset, bytes per frame, frames, offset of the left-facing set
// (0 for none), drawing offsets from the feet and width in pixels.
type sprite struct{ off, size, frames, left, ox, oy, w int }

// spriteFor returns the block drawn for l, choosing by behaviour priority except that exploding
// is drawn before drowning. The explosion's block is its first frame only; it has no sprite after
// that.
func spriteFor(l *sim.Lemming) sprite {
	switch {
	case l.State&sim.OhNo != 0:
		return sprite{0x7700, 0x50, 16, 0, -8, -10, 16}
	case l.State&sim.Exploding != 0:
		// Also a drowning lemming whose fuse ran out: it drowns on but is drawn as the explosion.
		return sprite{0x1B20, 0x200, 1, 0, -15, -25, 32}
	case l.State&sim.Drowning != 0:
		return sprite{0x63A0, 0x50, 16, 0, -8, -10, 16}
	case l.State&sim.Splatting != 0:
		return sprite{0x2A20, 0x50, 16, 0, -8, -10, 16}
	case l.State&sim.Falling != 0:
		return sprite{0x05A0, 0x50, 4, 0x140, -8, -10, 16}
	case l.State&sim.Exiting != 0:
		return sprite{0x68A0, 0x68, 8, 0, -8, -13, 16}
	case l.State&sim.Burning != 0:
		return sprite{0x6BE0, 0x70, 14, 0, -8, -14, 16}
	case l.State&sim.Digging != 0:
		return sprite{0x0820, 0x70, 16, 0, -8, -12, 16}
	case l.State&sim.Climbing != 0:
		return sprite{0x0F20, 0x60, 8, 0x300, -8, -12, 16}
	case l.State&sim.Hoisting != 0:
		return sprite{0x1520, 0x60, 8, 0x300, -8, -12, 16}
	case l.State&sim.Building != 0:
		return sprite{0x1D20, 0x68, 16, 0x680, -8, -13, 16}
	case l.State&sim.Blocking != 0:
		return sprite{0x2F20, 0x50, 16, 0, -8, -10, 16}
	case l.State&sim.Bashing != 0:
		return sprite{0x3420, 0x50, 32, 0xA00, -8, -10, 16}
	case l.State&sim.Floating != 0:
		return sprite{0x4820, 0x80, 8, 0x400, -8, -16, 16}
	case l.State&sim.Mining != 0:
		return sprite{0x5020, 0x68, 24, 0x9C0, -8, -13, 16}
	case l.State&sim.Shrugging != 0:
		return sprite{0x7200, 0x50, 8, 0x280, -8, -10, 16}
	}
	return sprite{0x0000, 0x50, 9, 0x2D0, -8, -10, 16} // walking, and jumping as frame 8
}

// typeName is the status line's name for l, first match winning.
func typeName(l *sim.Lemming) string {
	switch {
	case l.Climber() && l.Floater():
		return "ATHLETE"
	case l.Floater():
		return "FLOATER"
	case l.Climber():
		return "CLIMBER"
	case l.Fuse() > 0:
		return "BOMBER"
	case l.State == 0:
		return "WALKER"
	case l.State&sim.Falling != 0:
		return "FALLER"
	case l.State&sim.Exploding != 0:
		return "BOMBER"
	case l.State&sim.Digging != 0:
		return "DIGGER"
	case l.State&sim.Building != 0:
		return "BUILDER"
	case l.State&sim.Blocking != 0:
		return "BLOCKER"
	case l.State&sim.Bashing != 0:
		return "BASHER"
	case l.State&sim.Mining != 0:
		return "MINER"
	}
	return ""
}

// fuseDigit is the digit 1-5 drawn over a lemming whose fuse has n updates left, or 0 for none.
func fuseDigit(n int) int {
	if n == 0 {
		return 0
	}
	return n/16 + 1
}

// play is a level in play.
type play struct {
	n      int // play-order index, or -1 for a record outside the play order
	lvl    *level.Level
	sim    *sim.Game
	frame  int // 50 Hz frames since level start
	scroll int
	escape bool // Escape pressed since the last update
}

func newPlay(l *level.Level, tab *sim.Tables, n int) *play {
	return &play{n: n, lvl: l, sim: sim.New(l, tab), scroll: min(max(l.StartX+16, 0), maxScroll)}
}

// update runs one display frame with the pointer centre at screen (sx, sy); the game updates
// every third frame.
func (p *play) update(in input, sx, sy int, sound *mixer, pan bool) {
	if in.escape {
		p.escape = true
	}
	p.frame++
	if p.frame%3 != 0 {
		return
	}
	left, right := in.left, in.right
	if d := edgeStep(sx, sy, left, right); d != 0 {
		p.scroll = scrollStep(p.scroll, d)
	}
	if s, ok := minimapScroll(sx, sy); ok && left {
		p.scroll = s
	}

	px, py := toMap(sx, sy, p.scroll)
	button := -1
	if sy >= 200 && sy < 224 && sx >= 0 && sx < 16*(sim.Nuke+1) {
		button = sx / 16
	}
	p.sim.Input = sim.Input{X: px, Y: py, Playfield: sy-viewTop < 176,
		Button: button, Left: left, Right: right,
		VolDown: in.f1, VolUp: in.f2, Escape: p.escape, Speaker: in.f5}
	p.escape = false
	p.sim.Step()
	if sound != nil {
		for _, s := range p.sim.Sounds {
			x := s.X
			if !pan {
				x = -1
			}
			sound.request(s.Sample, s.Pitch, min(p.sim.Volume+20, 255), x, p.scroll)
		}
		sound.set(p.sim.MusicOn(), p.sim.MusicVolume(), p.sim.Quiet)
	}
}

// objectFrame is the frame shown in update n, counting the first update as 1, of a looping
// object, a trapdoor (entrance) or a static object with the given number of frames.
func objectFrame(loop, entrance bool, frames, n int) int {
	switch {
	case loop:
		return n % frames
	case entrance && n > 34:
		// Opening from update 35, when the start counter signals; frame 0 once fully open.
		if f := n - 33; f < frames {
			return f
		}
		return 0
	case entrance:
		return 1 // closed
	}
	return 0
}

func (p *play) panelState() screen.State {
	s := screen.State{
		Scroll: p.scroll,
		Skill:  int(p.sim.Selected()),
		InPlay: 1,
		Saved:  p.sim.Saved * 100 / max(p.lvl.Lemmings, 1),
	}
	s.Buttons[0], s.Buttons[1] = p.sim.Rate()
	copy(s.Buttons[2:], p.sim.Skills[:])
	chosen, count := p.sim.Chosen()
	s.Count = count
	if chosen >= 0 {
		s.Type = typeName(&p.sim.Lemmings[chosen])
	}
	if len(p.sim.Lemmings) > 0 {
		s.InPlay = 0
	}
	for _, l := range p.sim.Lemmings {
		if !l.Removed {
			s.InPlay++
			s.Lemmings = append(s.Lemmings, image.Pt(l.X, l.Y))
		}
	}
	s.Minutes, s.Seconds = p.sim.Clock()
	return s
}

// screen composes the level's screen with the pointer centre at screen position ptr.
func (p *play) screen(a *art, ptr image.Point) (*screen.Screen, error) {
	sc := &scene{
		lvl:      p.lvl,
		terrain:  p.sim.Terrain(),
		frames:   make([]int, len(p.lvl.Objects)),
		lemmings: p.sim.Lemmings,
		panel:    p.panelState(),
		scroll:   p.scroll,
		nuked:    p.sim.Nuked(),
		pointer:  ptr,
	}
	for i := range p.lvl.Objects {
		o := &p.lvl.Objects[i]
		sc.frames[i] = objectFrame(o.Loop, o.Entrance, o.Frames(), p.sim.Start())
		if o.Effect == 4 && o.Slot < 16 { // a trap
			sc.frames[i] = p.sim.TrapFrame(o.Slot)
		}
	}
	_, count := p.sim.Chosen()
	sc.brackets = count > 0
	pix, lp, pp, err := a.compose(sc)
	if err != nil {
		return nil, err
	}
	return &screen.Screen{Pix: pix, Upper: lp, Lower: pp, Split: panelTop}, nil
}

// game runs the flow in Ebitengine: it reads the mouse and keys, and shows the flow's frames at
// a whole-number scale.
type game struct {
	f    *flow
	rows int // screen rows shown in the last frame drawn

	cx, cy   float64 // the OS cursor last frame, while captured
	captured bool    // cx, cy are valid
	swallow  bool    // the click that recaptured the cursor is still held
	lw, lh   int     // the outside size in device pixels, from the last LayoutF

	rgba []byte
	out  *ebiten.Image
	wide *ebiten.Image // the farewell's 640 × 256 screen, made when first shown
	dock []image.Image // the window icon, until the first Update shows it in the macOS Dock
}

func (g *game) Update() error {
	if g.dock != nil {
		setDockIcon(g.dock)
		g.dock = nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) || inpututil.IsKeyJustPressed(ebiten.KeyF) &&
		(ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)) {
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	}
	release, toGame := backspaceAction(ebiten.IsKeyPressed(releaseMod(runtime.GOOS)))
	if release && inpututil.IsKeyJustPressed(ebiten.KeyBackspace) &&
		ebiten.CursorMode() == ebiten.CursorModeCaptured {
		ebiten.SetCursorMode(ebiten.CursorModeVisible)
	}
	in := input{
		back:   toGame && ebiten.IsKeyPressed(ebiten.KeyBackspace),
		escape: inpututil.IsKeyJustPressed(ebiten.KeyEscape),
		del:    ebiten.IsKeyPressed(ebiten.KeyDelete),
		ret:    ebiten.IsKeyPressed(ebiten.KeyEnter),
		enter:  ebiten.IsKeyPressed(ebiten.KeyNumpadEnter),
		space:  ebiten.IsKeyPressed(ebiten.KeySpace),
		f1:     ebiten.IsKeyPressed(ebiten.KeyF1),
		f2:     ebiten.IsKeyPressed(ebiten.KeyF2),
		f5:     ebiten.IsKeyPressed(ebiten.KeyF5),
	}
	for i := range 26 {
		if ebiten.IsKeyPressed(ebiten.KeyA + ebiten.Key(i)) {
			in.letters |= 1 << i
		}
	}
	in.dx, in.dy = g.track()
	if g.captured && !g.swallow {
		in.left = ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
		in.right = ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight)
		in.middle = ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle)
	}
	if err := g.f.update(in); err != nil {
		return err
	}
	if g.f.quit {
		return ebiten.Termination
	}
	return nil
}

// track returns the captured cursor's motion in screen pixels. Losing focus releases the
// cursor; a click in the window captures it again, and that click is not passed to the game.
func (g *game) track() (dx, dy float64) {
	switch {
	case !ebiten.IsFocused():
		if ebiten.CursorMode() == ebiten.CursorModeCaptured {
			ebiten.SetCursorMode(ebiten.CursorModeVisible)
		}
		g.captured = false
		return 0, 0
	case ebiten.CursorMode() != ebiten.CursorModeCaptured:
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) ||
			inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
			ebiten.SetCursorMode(ebiten.CursorModeCaptured)
			g.swallow = true
		}
		g.captured = false
		return 0, 0
	}
	if g.swallow && !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) &&
		!ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
		g.swallow = false
	}
	// Captured, the cursor position is unbounded and in device pixels; only its motion counts.
	x, y := ebiten.CursorPositionF()
	if g.captured {
		s, _, _ := fit(g.lw, g.lh, g.rows)
		dx, dy = (x-g.cx)/float64(s), (y-g.cy)/float64(s)
	}
	g.cx, g.cy, g.captured = x, y, true
	return dx, dy
}

func (g *game) Draw(dst *ebiten.Image) {
	v, err := g.f.view()
	if err != nil {
		log.Fatal(err)
	}
	s := v.scr
	for i, c := range s.Pix {
		p := s.Upper
		if i >= s.Split*v.w {
			p = s.Lower
		}
		r, gr, b, _ := p[c].RGBA()
		g.rgba[4*i], g.rgba[4*i+1], g.rgba[4*i+2], g.rgba[4*i+3] = byte(r>>8), byte(gr>>8), byte(b>>8), 0xff
	}
	out := g.out
	bd := dst.Bounds()
	var hs, vs, ox, oy int
	if v.w == wideW {
		if g.wide == nil {
			g.wide = ebiten.NewImage(wideW, screenH)
		}
		out = g.wide
		hs, vs, ox, oy = fitWide(bd.Dx(), bd.Dy(), v.rows)
	} else {
		hs, ox, oy = fit(bd.Dx(), bd.Dy(), v.rows)
		vs = hs
	}
	out.WritePixels(g.rgba[:4*len(s.Pix)])
	g.rows = v.rows
	dst.Fill(v.border)
	op := &ebiten.DrawImageOptions{} // nearest filtering
	op.GeoM.Scale(float64(hs), float64(vs))
	op.GeoM.Translate(float64(ox), float64(oy))
	dst.DrawImage(out.SubImage(image.Rect(0, v.top, v.w, v.top+v.rows)).(*ebiten.Image), op)
}

func (g *game) Layout(int, int) (int, int) { panic("unused: LayoutF is called instead") }

// LayoutF makes the screen the window's size in device pixels, so the final copy to the display
// is 1:1 and only Draw's integer scale enlarges the game.
func (g *game) LayoutF(w, h float64) (float64, float64) {
	d := ebiten.Monitor().DeviceScaleFactor()
	lw, lh := int(math.Ceil(w*d)), int(math.Ceil(h*d))
	if lw != g.lw || lh != g.lh {
		// The cursor position's units change with the size; take a fresh baseline.
		g.lw, g.lh, g.captured = lw, lh, false
	}
	return w * d, h * d
}

func loadAssets(d *disc.Disc) (*assets, error) {
	prog, err := d.Unpack("/!Lemmings/Lemmings/LemProg")
	if err != nil {
		return nil, err
	}
	tab, err := sim.LoadTables(prog)
	if err != nil {
		return nil, err
	}
	a, err := loadArt(d, prog)
	if err != nil {
		return nil, err
	}
	m, err := screen.LoadMenu(d, prog)
	if err != nil {
		return nil, err
	}
	return &assets{disc: d, menu: m, art: a, tables: tab}, nil
}

// versionText is the -version output: the module version and revision Go stamps from the checkout.
func versionText(info *debug.BuildInfo) string {
	version, revision := "(devel)", ""
	if info != nil {
		version = info.Main.Version
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				revision = s.Value
			}
		}
	}
	return fmt.Sprintf("version=%s\nrevision=%s\n", version, revision)
}

func main() {
	adf := flag.String("disc", "", "path to the player's disc image (.adf or .jfd)")
	rec := flag.Int("record", -1, "developer shortcut: play level record 0-99 at once, skipping the menus")
	pan := flag.Bool("pan", true, "pan sound effects by the lemming's place in the view (the original centres them)")
	version := flag.Bool("version", false, "print the version and revision, then exit")
	flag.Parse()
	if *version {
		info, _ := debug.ReadBuildInfo()
		fmt.Print(versionText(info))
		return
	}

	scale := 1
	if mon := ebiten.Monitor(); mon != nil {
		scale = windowScale(mon.Size())
	}
	ebiten.SetWindowSize((screenW+2*borderW)*scale, (menuH+2*borderW)*scale)
	ebiten.SetWindowSizeLimits(screenW, menuH, -1, -1)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("OpenLemm")
	// The palette is plain RGB; Ebitengine otherwise tags a macOS window Display P3, which
	// oversaturates every colour.
	opts := &ebiten.RunGameOptions{ColorSpace: ebiten.ColorSpaceSRGB}

	var d *disc.Disc
	var err error
	if *adf != "" {
		if d, err = openDisc(*adf); err != nil {
			log.Fatal(err)
		}
	} else {
		dirs := discDirs(runtime.GOOS)
		var skipped []error
		if d, skipped = findDisc(dirs); d == nil {
			text := noDiscText(dirs, skipped)
			fmt.Fprintln(os.Stderr, strings.Join(text, "\n"))
			for _, err := range skipped {
				fmt.Fprintln(os.Stderr, err)
			}
			if err := ebiten.RunGameWithOptions(&noDisc{pix: textScreen(text...)}, opts); err != nil {
				log.Print(err)
			}
			os.Exit(1)
		}
	}
	a, err := loadAssets(d)
	if err != nil {
		log.Fatal(err)
	}
	// Record 0 is in tileset 0. The icon frames use colours 1-6, the same in every tileset,
	// except the digger's 7 and the burner's 8, which are tileset 0's.
	lvl, err := level.Load(d, 0)
	if err != nil {
		log.Fatal(err)
	}
	icon := icons(a.art.iconFrame(iconStates[rand.IntN(len(iconStates))], lvl.Map.Palette))
	ebiten.SetWindowIcon(icon)
	m, err := loadSound(d, a.art.prog)
	if err != nil {
		log.Fatal(err)
	}
	f := newFlow(a, m)
	f.pan = *pan
	if *rec >= 0 {
		if f.lvl, err = level.Load(d, *rec); err != nil {
			log.Fatal(err)
		}
		n, err := level.PlayIndex(d, *rec)
		if err != nil {
			log.Printf("%v: the game ends with the level", err)
			n = -1
		}
		f.sess.level, f.only = max(n, 0), n < 0
		f.enter(playing)
	}

	ebiten.SetCursorMode(ebiten.CursorModeCaptured)
	// One 50 Hz display frame per Update.
	ebiten.SetTPS(50)
	p, err := audio.NewContext(sampleRate).NewPlayer(m)
	if err != nil {
		log.Fatal(err)
	}
	p.SetBufferSize(40 * time.Millisecond)
	p.Play()
	g := &game{f: f, rows: menuH, rgba: make([]byte, 4*wideW*screenH), out: ebiten.NewImage(screenW, screenH), dock: icon}
	if err := ebiten.RunGameWithOptions(g, opts); err != nil {
		log.Fatal(err)
	}
	if f.ph == farewell {
		fmt.Println(f.bye)
	}
}

// loadSound reads the effects from SampdatP and the pitch table from LemProg.
func loadSound(d *disc.Disc, prog []byte) (*mixer, error) {
	b, err := d.Unpack("/!Lemmings/Lemmings/SampdatP")
	if err != nil {
		return nil, err
	}
	s, err := readSamples(b)
	if err != nil {
		return nil, err
	}
	periods, err := readPeriods(prog)
	if err != nil {
		return nil, err
	}
	return &mixer{samples: &s, periods: periods}, nil
}
