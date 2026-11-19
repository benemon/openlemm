// Package sim runs the game's level simulation one update at a time.
package sim

import (
	"encoding/binary"
	"fmt"
	"image"

	"github.com/benemon/openlemm/internal/level"
)

// Behaviour bits of Lemming.State, as the game stores them; no bit set means walking.
const (
	Exiting   = 0x2
	Burning   = 0x4
	Drowning  = 0x8
	Floating  = 0x20
	Blocking  = 0x80
	Exploding = 0x400
	Shrugging = 0x1
	Mining    = 0x10
	Bashing   = 0x40
	Splatting = 0x100
	Building  = 0x200
	Hoisting  = 0x800
	Climbing  = 0x1000
	Jumping   = 0x2000
	Digging   = 0x4000
	Falling   = 0x8000
	OhNo      = 0x10000
)

// Skill indexes Game.Skills: climber, floater, bomber, blocker, builder, basher, miner, digger.
type Skill int

// Skills, in panel order.
const (
	Climber Skill = 0
	Floater Skill = 1
	Bomber  Skill = 2
	Blocker Skill = 3
	Builder Skill = 4
	Basher  Skill = 5
	Miner   Skill = 6
	Digger  Skill = 7
)

const (
	mapW        = 1632
	mapH        = 176 // map rows 0-175, as level.Map
	gridW       = 408
	gridH       = 44
	steel       = 9
	trap        = 4
	water       = 5
	fire        = 6
	oneWayLeft  = 7
	oneWayRight = 8
	exit        = 1
	fieldLeft   = 2
	fieldRight  = 3
	fieldCentre = 10
	offRow      = 172 // a lemming reaching this row leaves the level
	topRow      = 8
	digFrom     = 17 // a new digger's frame until its first update

	framesPerUpdate = 3 // display frames, each one clock tick
)

// Lemming is one released lemming. X and Y are map coordinates; Y is the row it stands on.
type Lemming struct {
	X, Y    int
	Dir     int // +1 right, -1 left, 0 once splatting
	Frame   int
	State   int
	Removed bool

	fall      uint8 // fall-distance count, a single byte in the game
	own       uint8 // trigger cell under the feet at the last stored position
	ahead     uint8 // trigger cell 2 blocks ahead and 2 up at the last stored position
	climber   bool
	floater   bool
	floatStep uint8    // index into the floater table
	bricks    int      // bricks a builder has left
	fuse      int      // bomb fuse updates left; 0 when none is running
	field     bool     // a blocker field is in the grid for this lemming
	saved     [9]uint8 // the cells under the field, row by row
}

// Climber reports whether l has the climber ability.
func (l *Lemming) Climber() bool { return l.climber }

// Fuse returns the updates left on l's bomb fuse, 0 when none is running.
func (l *Lemming) Fuse() int { return l.fuse }

// Floater reports whether l has the floater ability.
func (l *Lemming) Floater() bool { return l.floater }

// Game is a level in play.
type Game struct {
	// Update counts updates from level start; the first lemming is released in update 50.
	Update   int
	Lemmings []Lemming // in release order
	Skills   [8]int    // skills left
	Saved    int
	// Finished is the level-finished flag: no lemming in play and all released.
	Finished bool
	// TimeUp is the time-up flag, set by the clock or at once by the Escape key.
	TimeUp bool
	// Over reports that the ending sequence has run and the level is left.
	Over bool
	// NukeRecord is the start counter recorded by the last nuke-button click. The game sets it to
	// 0 when the program starts and never resets it between levels, so the caller carries it
	// from one level's Game to the next.
	NukeRecord int
	// Input is the pointer, buttons and keys for the next Step.
	Input Input
	// Effects selects sound-effects mode, with no music; false is music mode, the default.
	Effects bool
	// Volume is the music volume setting, 0-255; the game keeps it across levels, so the caller
	// carries it from one level's Game to the next.
	Volume int
	// Quiet is the speaker turned off with F5; the caller carries it from level to level.
	Quiet bool
	// Sounds holds the sound requests made during the last Step, in order.
	Sounds []Sound

	terrain   []uint8 // mapW × mapH colour indices
	grid      [gridH][gridW]uint8
	slots     [4]image.Point // release positions, used in rotation
	total     int
	countdown int
	interval  int
	inPlay    int
	tables    *Tables
	moving    int        // the lemming advance is moving
	crater    *Lemming   // the lemming making this update's crater
	trapLen   [16]int    // animation frames of the trap in each object slot
	trapBusy  [16]int    // updates until each trap is idle again
	trapSound [16][2]int // sample and pitch of the trap in each object slot

	clock  [3]int // minutes, seconds, ticks left
	start  int    // the start counter: unpaused updates, 1 in the first
	shown  int    // the start counter in the update last drawn
	paused bool
	rate   int // the current release rate; the level's is the minimum
	min    int
	nuke   bool
	nukeAt int  // the next lemming the nuke looks at; -1 before the first
	ending bool // the ending sequence has started
	fade   int  // ending updates run, from the update after the flag was first seen
	music  int  // the music's volume during the ending
	f5     bool // F5 was down when last looked at

	read          Input // the input as the last pointer step read it
	selected      Skill
	latched       bool // the left button has been held since the last click acted on
	chosen, other int  // the lemmings chosen under the pointer, and the best not only working
	count         int  // lemmings under the pointer
}

// Input is the pointer, mouse buttons and keys as the game reads them once per update.
type Input struct {
	X, Y        int  // pointer centre in map pixels
	Playfield   bool // the pointer's level row is below 176
	Button      int  // the panel button under the pointer, or -1: RateDown … Nuke
	Left, Right bool // buttons held
	VolDown     bool // F1 held
	VolUp       bool // F2 held
	Escape      bool // Escape pressed since the last update
	Speaker     bool // F5 held
}

// Sound is a sound request made in update Update: a SampdatP sample and a pitch index, and the map
// x of the lemming it is for, or -1 when it is for none. The original records that x but never uses
// it; X is for presentation only.
type Sound struct{ Update, Sample, Pitch, X int }

// fadeUpdates is the updates the screen fade takes: 16 steps, one every 2 display frames.
const fadeUpdates = 11

// Panel buttons, left to right; the eight skill buttons lie between RateUp and Pause.
const (
	RateDown = 0
	RateUp   = 1
	Pause    = 10
	Nuke     = 11
)

type floatEntry struct {
	frame, move int
}

// Tables holds the game's movement tables, read from LemProg.
type Tables struct {
	float  [64]floatEntry
	bash   [2][4][10]uint16 // basher masks: facing right, left; sub-frame 2-5; rows, bit 15 leftmost
	mine   [2][2][13]uint16 // miner masks: facing right, left; frame 1, 2; rows, bit 15 leftmost
	crater [22]uint16
}

// LoadTables reads the floater movement table at &23A40 and the basher, miner and crater masks at
// &A22C, &A2CC and &A924 in prog, the unpacked LemProg.
func LoadTables(prog []byte) (*Tables, error) {
	const off = 0x23A40 - 0x8000
	if len(prog) < off+64*4 {
		return nil, fmt.Errorf("LemProg: %d bytes, too short for the floater table", len(prog))
	}
	t := &Tables{}
	for i := range t.float {
		e := prog[off+4*i:]
		t.float[i] = floatEntry{int(binary.LittleEndian.Uint16(e)), int(int16(binary.LittleEndian.Uint16(e[2:])))}
	}
	for side := range t.bash {
		for m := range t.bash[side] {
			for r := range t.bash[side][m] {
				t.bash[side][m][r] = binary.LittleEndian.Uint16(prog[0xA22C-0x8000+0x50*side+20*m+2*r:])
			}
		}
	}
	for r := range t.crater {
		t.crater[r] = binary.LittleEndian.Uint16(prog[0xA924-0x8000+2*r:])
	}
	for side := range t.mine {
		for m := range t.mine[side] {
			for r := range t.mine[side][m] {
				t.mine[side][m][r] = binary.LittleEndian.Uint16(prog[0xA2CC-0x8000+0x34*side+26*m+2*r:])
			}
		}
	}
	return t, nil
}

// New starts level l with the movement tables tab.
func New(l *level.Level, tab *Tables) *Game {
	g := &Game{
		tables:    tab,
		Skills:    l.Skills,
		terrain:   make([]uint8, mapW*mapH),
		total:     l.Lemmings,
		countdown: 50,
		interval:  103 - l.ReleaseRate,
		inPlay:    1,
		chosen:    -1,
		other:     -1,
		clock:     [3]int{l.Minutes, 0, 49},
		rate:      l.ReleaseRate,
		min:       l.ReleaseRate,
		Volume:    255,
	}
	copy(g.terrain, l.Map.Pix)

	for _, s := range l.Steel {
		c, r := s.Min.X/4, s.Min.Y/4
		// Steel is filled one block row lower than stored.
		g.fill(image.Rect(c, r+1, c+s.Dx()/4, r+1+s.Dy()/4), steel)
	}
	for _, o := range l.Objects {
		if o.Slot < 16 {
			g.fill(o.Trigger.Add(image.Pt(o.X>>2, o.Y>>2)), uint8(o.Effect+16*o.Slot))
			if o.Effect == trap {
				g.trapLen[o.Slot] = o.Frames()
				g.trapSound[o.Slot] = [2]int{o.Sample, o.Pitch}
			}
		}
	}

	var doors []image.Point
	for _, o := range l.Objects {
		if o.Entrance {
			doors = append(doors, image.Pt(o.X+24, o.Y+14))
		}
	}
	// With 1-4 trapdoors the four slots are AAAA, ABAB, ABCB, ABCD.
	switch len(doors) {
	case 0:
		g.total = 0
	case 1:
		g.slots = [4]image.Point{doors[0], doors[0], doors[0], doors[0]}
	case 2:
		g.slots = [4]image.Point{doors[0], doors[1], doors[0], doors[1]}
	case 3:
		g.slots = [4]image.Point{doors[0], doors[1], doors[2], doors[1]}
	default:
		g.slots = [4]image.Point(doors[:4])
	}
	return g
}

// TrapFrame returns the animation frame of the trap in object slot s: 0 while idle, counting up
// from the update it caught a lemming until it is idle again.
func (g *Game) TrapFrame(s int) int {
	if g.trapBusy[s] == 0 {
		return 0
	}
	return g.trapLen[s] - g.trapBusy[s]
}

// Terrain returns the terrain map as it is now: 1632 × 176 colour indices, row-major, map rows.
// Callers must not modify it.
func (g *Game) Terrain() []uint8 { return g.terrain }

func (g *Game) fill(r image.Rectangle, v uint8) {
	r = r.Intersect(image.Rect(0, 0, gridW, gridH))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			g.grid[y][x] = v
		}
	}
}

func (g *Game) cell(x, y int) uint8 {
	c, r := x>>2, y>>2
	if c < 0 || c >= gridW || r < 0 || r >= gridH {
		return 0
	}
	return g.grid[r][c]
}

// Assign gives lemming i a skill in the current update, before lemmings move, as a forced
// assignment does: whatever lemming the pointer would choose. It reports whether the skill was
// accepted.
func (g *Game) Assign(i int, s Skill) bool {
	ok, _ := g.assign(i, s)
	return ok
}

// assign reports whether lemming i accepted skill s and, if not, whether the refusal came from the
// behaviour test, the one a click retries.
func (g *Game) assign(i int, s Skill) (ok, retry bool) {
	if i < 0 || i >= len(g.Lemmings) || s < Climber || s > Digger || g.Skills[s] == 0 {
		return false, false
	}
	l := &g.Lemmings[i]
	if l.Removed {
		return false, false
	}
	switch s {
	case Climber:
		if l.climber || l.State&(Exploding|Splatting|Blocking) != 0 {
			return false, false
		}
		l.climber = true
	case Floater:
		if l.floater || l.State&(Exploding|Splatting|Blocking|Burning) != 0 {
			return false, false
		}
		l.floater = true
	case Digger:
		if l.State&^(Building|Bashing|Mining|Shrugging) != 0 {
			return false, true
		}
		if l.own&15 == steel {
			g.sound(l.X, 0x1A, 0x0C)
			return false, false
		}
		l.State = Digging
		l.Frame = digFrom
	case Bomber:
		if l.fuse > 0 || l.State&(OhNo|Exploding|Splatting|Burning) != 0 {
			return false, false
		}
		l.fuse = 79
	case Blocker:
		if l.State&^(Digging|Building|Bashing|Mining|Shrugging) != 0 {
			return false, false
		}
		c, r := (l.X-4)>>2, (l.Y-6)>>2
		for k := range 9 {
			switch g.cell(4*(c+k%3), 4*(r+k/3)) & 15 {
			case exit, fieldLeft, fieldRight, fieldCentre:
				return false, false
			}
		}
		for k := range 9 {
			l.saved[k] = g.cell(4*(c+k%3), 4*(r+k/3))
		}
		for k, v := range [3]uint8{fieldLeft, fieldCentre, fieldRight} {
			g.fill(image.Rect(c+k, r, c+k+1, r+3), v)
		}
		l.field = true
		l.Dir = 1
		l.State = Blocking
		l.Frame = 0
	case Builder:
		if l.State&^(Digging|Bashing|Mining|Shrugging) != 0 {
			return false, true
		}
		if l.Y <= 8 {
			return false, false
		}
		l.State = Building
		l.Frame = 0
		l.bricks = 12
	case Basher:
		if l.State&^(Building|Digging|Mining|Shrugging) != 0 {
			return false, true
		}
		if l.ahead&15 == steel || l.ahead&15 == oneWayLeft && l.Dir > 0 || l.ahead&15 == oneWayRight && l.Dir < 0 {
			g.sound(l.X, 0x1A, 0x0C)
			return false, false
		}
		l.State = Bashing
		l.Frame = 0
	case Miner:
		if l.State&^(Building|Bashing|Digging|Shrugging) != 0 {
			return false, true
		}
		if l.own&15 == steel || l.ahead&15 == steel ||
			l.ahead&15 == oneWayLeft && l.Dir > 0 || l.ahead&15 == oneWayRight && l.Dir < 0 {
			g.sound(l.X, 0x1A, 0x0C)
			return false, false
		}
		l.State = Mining
		l.Frame = 0
		l.Y++
	}
	g.Skills[s]--
	g.sound(l.X, 0x17, 0x13)
	return true, false
}

// sound requests sample at pitch for the lemming at map x, or -1 for none.
func (g *Game) sound(x, sample, pitch int) {
	g.Sounds = append(g.Sounds, Sound{g.Update, sample, pitch, x})
}

const working = Digging | Building | Blocking | Bashing | Mining | Shrugging

// pointer acts on the left button, read once per update: a fresh press selects the skill on its
// button or, over the playfield, assigns the selected skill to the lemming chosen at the end of
// the last update, retrying once on a behaviour refusal with the best lemming not only working.
func (g *Game) pointer() {
	in := g.Input
	g.read = in
	b := in.Button
	switch {
	case !in.Left:
		g.latched = false
	case (b == RateDown || b == RateUp) && !g.paused:
		// Held, the rate buttons act every update and never latch.
		if r := g.rate + 2*b - 1; r >= g.min && r <= 99 {
			g.rate = r
			g.interval = (99-r)/2 + 4
			g.sound(-1, 0x17, r/3+1)
		}
	case g.latched:
	case b == Pause:
		g.paused = !g.paused
		g.latched = true
	case g.paused:
	case b == Nuke:
		if !g.nuke {
			g.latched = true
			if d := g.start - g.NukeRecord; d >= 0 && d <= 5 {
				g.nuke = true
				g.nukeAt = -1
				g.sound(-1, 0x18, 0x0E)
			}
			g.NukeRecord = g.start
		}
	case b >= 2 && b <= 9:
		g.selected = Skill(b - 2)
		g.latched = true
		g.sound(-1, 0x14, 0x0E+b-2)
	case in.Playfield:
		g.latched = true
		if ok, retry := g.assign(g.chosen, g.selected); !ok && retry {
			g.assign(g.other, g.selected)
		}
	}
}

// nukeStep gives at most one more lemming, in release order, a fuse.
func (g *Game) nukeStep() {
	if g.nukeAt < 0 {
		g.nukeAt = 0
		return
	}
	for ; g.nukeAt < len(g.Lemmings); g.nukeAt++ {
		l := &g.Lemmings[g.nukeAt]
		if l.Removed {
			continue
		}
		if l.fuse == 0 && l.State&(Exploding|Splatting) == 0 {
			l.fuse = 79
		}
		g.nukeAt++
		return
	}
}

// advance runs the object animation and moves every lemming.
func (g *Game) advance() {
	for i, n := range g.trapBusy {
		if n > 0 {
			g.trapBusy[i]--
		}
	}
	for i := range g.Lemmings {
		g.moving = i
		if l := &g.Lemmings[i]; !l.Removed && !g.burnFuse(l) {
			if g.move(l) {
				g.trigger(l)
			}
		}
	}
}

// Nuked reports whether the nuke has been triggered.
func (g *Game) Nuked() bool { return g.nuke }

// Ending reports whether the ending sequence has started: an update's end has seen time-up or
// level-finished.
func (g *Game) Ending() bool { return g.ending }

// Rate returns the level's minimum release rate and the current rate.
func (g *Game) Rate() (min, current int) { return g.min, g.rate }

// Start returns the start counter in the update last drawn: unpaused updates since level start,
// 1 in the first.
func (g *Game) Start() int { return g.shown }

// choose picks the lemming under the pointer as the last pointer step read it, for the next
// update's click: the latest released that is only working, unless the right button is held;
// otherwise the latest of the rest.
func (g *Game) choose() {
	g.chosen, g.other, g.count = g.choice(g.read)
}

// choice returns the lemming choose would pick with the pointer at in, the best not only working,
// and how many lemmings are under it.
func (g *Game) choice(in Input) (chosen, other, count int) {
	busy, other := -1, -1
	for i, l := range g.Lemmings {
		if l.Removed || in.X < l.X-5 || in.X > l.X+4 || in.Y < l.Y-11 || in.Y > l.Y {
			continue
		}
		count++
		if l.State != 0 && l.State&^working == 0 {
			busy = i
		} else {
			other = i
		}
	}
	if busy >= 0 && !in.Right {
		return busy, other, count
	}
	return other, other, count
}

// tick runs the level clock for one display frame.
func (g *Game) tick() {
	c := &g.clock
	switch {
	case g.TimeUp:
	case *c == [3]int{}:
		g.TimeUp = true
	case c[2] > 0:
		c[2]--
	case c[1] > 0:
		c[1], c[2] = c[1]-1, 49
	default:
		c[0], c[1], c[2] = c[0]-1, 59, 49
	}
}

// Clock returns the minutes and seconds left on the level clock.
func (g *Game) Clock() (minutes, seconds int) { return g.clock[0], g.clock[1] }

// Chosen returns the lemming the pointer chose at the end of the last update, or -1, and how many
// lemmings were under it.
func (g *Game) Chosen() (i, count int) { return g.chosen, g.count }

// Selected returns the selected skill.
func (g *Game) Selected() Skill { return g.selected }

// Step finishes the current update by moving every lemming, then starts the next update with its
// release and its pointer step, which reads Input. Between Steps the game is where the oracle
// observes it: released and input acted on, not yet moved.
func (g *Game) Step() {
	g.finish()
	g.begin()
}

// finish moves every lemming and ends the update with its choice of lemming under the pointer.
func (g *Game) finish() {
	g.Sounds = g.Sounds[:0]
	if !g.paused {
		for range framesPerUpdate {
			g.tick()
		}
		g.advance()
		if g.nuke {
			g.nukeStep()
		}
	}
	if l := g.crater; l != nil {
		g.crater = nil
		if c := l.own & 15; c != steel && c != exit && c != water {
			// Moved down at the top; cut off below map row 167 at the bottom.
			y := max(l.Y-14, 0)
			g.erase(max(l.X-8, 0), y, g.tables.crater[:min(len(g.tables.crater), 168-y)])
		}
	}
	// The finished test uses the in-play count of the previous update.
	g.Finished = g.inPlay == 0 && (g.nuke || len(g.Lemmings) == g.total)
	if len(g.Lemmings) > 0 {
		g.inPlay = 0
		for _, l := range g.Lemmings {
			if !l.Removed {
				g.inPlay++
			}
		}
	}

	g.choose()
	switch {
	case g.ending:
		g.music = max(g.music-16, 0)
		g.fade++
		g.Over = g.music == 0 && g.fade >= fadeUpdates
	case g.TimeUp || g.Finished:
		g.ending = true
		g.music = g.Volume
	}
}

// begin starts the next update with its release and its pointer step, which reads Input.
func (g *Game) begin() {
	g.shown = g.start
	g.Update++
	g.keys()
	if !g.paused {
		g.start++
		switch g.start {
		case 15:
			g.sound(-1, 0x16, 0x0E)
		case 35:
			g.sound(-1, 0x15, 0x0C)
		}
		g.release()
	}
	g.pointer()
}

// keys scans F1, F2, Escape and F5 in that order; F1 or F2 ends the scan.
func (g *Game) keys() {
	in := g.Input
	switch {
	case in.VolDown:
		g.Volume = max(g.Volume-3, 0)
	case in.VolUp:
		g.Volume = min(g.Volume+3, 255)
	default:
		if in.Escape {
			g.TimeUp = true
		}
		if in.Speaker && !g.f5 {
			g.Quiet = !g.Quiet
		}
		g.f5 = in.Speaker
		return
	}
	g.music = g.Volume
}

// MusicOn reports whether the level's music is playing: in music mode, from start counter 55.
func (g *Game) MusicOn() bool { return !g.Effects && g.start >= 55 }

// MusicVolume returns the music's volume, 0-255: the setting, falling during the ending.
func (g *Game) MusicVolume() int {
	if g.ending {
		return g.music
	}
	return g.Volume
}

func (g *Game) release() {
	if len(g.Lemmings) < g.total && !g.nuke {
		g.countdown--
		if g.countdown == 0 {
			p := g.slots[len(g.Lemmings)%4]
			g.Lemmings = append(g.Lemmings, Lemming{X: p.X, Y: p.Y, Dir: 1})
			g.countdown = g.interval
		}
	}
}

// colour is map pixel (x, y); outside the map it is 0.
func (g *Game) colour(x, y int) uint8 {
	if x < 0 || x >= mapW || y < 0 || y >= mapH {
		return 0
	}
	return g.terrain[y*mapW+x]
}

func (g *Game) solid(x, y int) bool {
	return g.colour(x, y)&8 != 0
}

// move runs l's behaviour for one update and reports whether its position was stored.
func (g *Game) move(l *Lemming) bool {
	var stored bool
	x, y := l.X, l.Y
	switch {
	case l.State&OhNo != 0:
		l.Frame++
		if l.Frame == 16 {
			l.State = l.State&^OhNo | Exploding
			l.Frame = 0
			break
		}
		if l.Frame == 5 && !g.nuke {
			g.sound(l.X, 0x18, 0x0E)
			g.stray(l.X)
		}
		stored = g.drop(l)
	case l.State&Drowning != 0:
		l.Frame = (l.Frame + 1) & 15
		if l.Frame == 0 {
			l.Removed = true
			if g.Effects {
				g.sound(l.X, 0x01, 0x12)
			}
		} else if !g.solid(l.X+8*l.Dir, l.Y) {
			l.X += l.Dir
			stored = true
		}
	case l.State&Splatting != 0:
		l.Frame = (l.Frame + 1) & 15
		l.Removed = l.Frame == 0
	case l.State&Exploding != 0:
		l.Frame++
		if l.Frame == 1 {
			g.crater = l
			g.sound(l.X, 0x1D, 0x10)
			g.unblock(l)
			l.State &^= Blocking
		}
		l.Removed = l.Frame == 52
	case l.State&Falling != 0:
		g.fall(l)
		stored = true
	case l.State&Jumping != 0:
		g.jump(l)
		stored = true
	case l.State&Exiting != 0:
		l.Frame = (l.Frame + 1) & 7
		if l.Frame == 0 {
			l.Removed = true
			g.Saved++
		}
	case l.State&Burning != 0:
		l.Frame++
		l.Removed = l.Frame == 14
		if l.Removed && !g.Effects {
			g.sound(l.X, 0x1E, 0x1A)
		}
	case l.State&Digging != 0:
		stored = g.dig(l)
	case l.State&Climbing != 0:
		g.climb(l)
		stored = true
	case l.State&Hoisting != 0:
		l.Frame = (l.Frame + 1) & 7
		if l.Frame == 0 {
			l.State &^= Hoisting
		} else if l.Frame <= 4 {
			l.Y -= 2
		}
		stored = l.Frame <= 4
	case l.State&Building != 0:
		stored = g.build(l)
	case l.State&Blocking != 0:
		l.Frame = (l.Frame + 1) & 15
		if !g.solid(l.X, l.Y) {
			l.State = 0
			l.Frame = 0
			g.unblock(l)
		}
	case l.State&Bashing != 0:
		stored = g.bash(l)
	case l.State&Floating != 0:
		g.float(l)
		stored = true
	case l.State&Mining != 0:
		stored = g.mine(l)
	case l.State&Shrugging != 0:
		l.Frame = (l.Frame + 1) & 7
		if l.Frame == 0 {
			l.State = 0
			stored = true
		}
	default:
		g.walk(l)
		stored = true
	}
	if l.Y >= offRow {
		// The move off the bottom is never stored.
		l.Removed = true
		l.X, l.Y = x, y
		g.sound(l.X, 0x1E, 0x1A)
	}
	return stored && !l.Removed
}

// trigger caches the cells under and ahead of l's stored position and acts on the one under it.
func (g *Game) trigger(l *Lemming) {
	l.own = g.cell(l.X, l.Y)
	l.ahead = g.cell(l.X+8*l.Dir, l.Y-8)
	switch l.own & 15 {
	case exit:
		if l.State&(Exiting|Falling) == 0 {
			l.State = Exiting
			l.Frame = 0
			g.sound(l.X, 0x1B, 0x0C)
			g.stray(l.X)
		}
	case fieldLeft:
		if l.Dir > 0 {
			l.Dir = -l.Dir
		}
	case fieldRight:
		if l.Dir < 0 {
			l.Dir = -l.Dir
		}
	case fieldCentre:
		l.Dir = -l.Dir
		l.X += 4 * l.Dir
		if g.solid(l.X, l.Y-8) || g.solid(l.X, l.Y-2) {
			l.Dir = -l.Dir
			l.X += 4 * l.Dir
		}
	case trap:
		if s := l.own >> 4; g.trapBusy[s] == 0 {
			l.Removed = true
			g.trapBusy[s] = g.trapLen[s]
			if g.Effects {
				g.sound(l.X, g.trapSound[s][0], g.trapSound[s][1])
			} else {
				g.sound(l.X, 0x1E, 0x1A)
			}
		}
	case water:
		if l.State&Drowning == 0 {
			l.State = Drowning
			l.Frame = 0
			if g.Effects {
				g.sound(l.X, 0x0A, 0x10)
			} else {
				g.sound(l.X, 0x1E, 0x1A)
			}
		}
	case fire:
		if l.State&Burning == 0 {
			l.State = Burning
			l.Frame = 0
			if g.Effects {
				g.sound(l.X, 0x08, 0x11)
			}
		}
	}
}

// burnFuse runs l's bomb fuse and reports whether it ran out this update, which takes the place
// of the lemming's behaviour.
func (g *Game) burnFuse(l *Lemming) bool {
	if l.fuse == 0 {
		return false
	}
	l.fuse--
	if l.fuse > 0 {
		return false
	}
	switch {
	case l.State&(Falling|Floating|Burning|Drowning) != 0:
		// Drowning outranks exploding, so a drowning lemming drowns on with no crater.
		l.State |= Exploding
		l.Frame = 0
	default:
		l.State |= OhNo
		l.Frame = 0
	}
	return true
}

// unblock puts back the cells saved under l's blocker field, at the field of its current
// position: a blocker that has moved since leaves its old field in place (an original-game bug).
func (g *Game) unblock(l *Lemming) {
	if !l.field {
		return
	}
	l.field = false
	c, r := (l.X-4)>>2, (l.Y-6)>>2
	for k, v := range l.saved {
		g.fill(image.Rect(c+k%3, r+k/3, c+k%3+1, r+k/3+1), v)
	}
}

// stray reproduces the original game's stray write of x, as 4 little-endian bytes, into the
// record two places after the lemming being moved: over its cached ahead cell and the saved top
// row of its blocker field.
func (g *Game) stray(x int) {
	if i := g.moving + 2; i < len(g.Lemmings) {
		l := &g.Lemmings[i]
		l.ahead = uint8(x)
		l.saved[0], l.saved[1], l.saved[2] = uint8(x>>8), 0, 0
	}
}

// drop moves an "Oh no!" lemming down onto ground within 2 rows, or 3 rows when there is none, and
// reports whether it moved.
func (g *Game) drop(l *Lemming) bool {
	for dy := range 3 {
		if g.solid(l.X, l.Y+dy) {
			l.Y += dy
			return dy > 0
		}
	}
	l.Y += 3
	return true
}

func (g *Game) fall(l *Lemming) {
	l.Frame = (l.Frame + 1) & 3
	for dy := range 3 {
		if g.solid(l.X, l.Y+dy) {
			l.Y += dy
			l.Frame = 0
			if l.fall > 60 && !l.floater {
				l.State = l.State&^Exploding | Splatting
				l.Dir = 0
				l.fuse = 0
				g.sound(l.X, 0x19, 0x13)
			} else {
				l.State &^= Falling
			}
			return
		}
	}
	l.Y += 3
	l.fall += 3
	if l.fall > 16 && l.floater {
		l.State = Floating
		l.Frame = 0
		l.floatStep = 0
	}
}

// float runs one step of the floater table.
func (g *Game) float(l *Lemming) {
	st := g.tables.float[l.floatStep&63]
	l.floatStep++
	l.Frame = st.frame
	if st.move <= 0 {
		l.Y += st.move
		return
	}
	for dy := range st.move {
		if g.solid(l.X, l.Y+dy) {
			l.Y += dy
			l.State &^= Floating
			l.Frame = 0
			return
		}
	}
	l.Y += st.move
}

func (g *Game) jump(l *Lemming) {
	switch {
	case !g.solid(l.X, l.Y-1):
		l.State &^= Jumping
		l.Frame = 0
	case !g.solid(l.X, l.Y-2):
		l.State &^= Jumping
		l.Frame = 0
		l.Y--
	default:
		l.Y -= 2
	}
	if l.Y < topRow {
		l.Y = topRow
		l.Dir = -l.Dir
		l.State &^= Jumping
	}
}

func (g *Game) clearRow(x, y int) {
	if y < 0 || y >= mapH {
		return
	}
	for i := max(x-4, 0); i <= min(x+4, mapW-1); i++ {
		g.terrain[y*mapW+i] = 0
	}
}

// dig reports whether the digger ran a dig step this update, which stores its position.
func (g *Game) dig(l *Lemming) bool {
	if l.Frame == digFrom {
		g.clearRow(l.X, l.Y-2)
		g.clearRow(l.X, l.Y-1)
		l.Frame = 0
		return g.digStep(l)
	}
	l.Frame = (l.Frame + 1) & 15
	if l.Frame&7 != 0 {
		return false
	}
	return g.digStep(l)
}

func (g *Game) digStep(l *Lemming) bool {
	// Open space means colour 0 exactly; colours 1-7 are not solid but are not open either.
	open := true
	for x := l.X - 2; x <= l.X+2; x++ {
		open = open && g.colour(x, l.Y) == 0
	}
	if open {
		l.State = Falling
		l.Frame = 0
		l.fall = 0
	}
	g.clearRow(l.X, l.Y)
	l.Y++
	if l.State&Digging != 0 && g.cell(l.X, l.Y)&15 == steel {
		l.State &^= Digging
		l.Frame = 0
		g.sound(l.X, 0x1A, 0x0C)
	}
	return true
}

func (g *Game) walk(l *Lemming) {
	l.Frame = (l.Frame + 1) & 7
	l.X += l.Dir
	if l.X < 16 || l.X > 1607 {
		// The turn ends the update: no ground is tested, and the out-of-range x is stored.
		l.Dir = -l.Dir
		return
	}
	if g.solid(l.X, l.Y) {
		rise := 0
		for rise < 7 && g.solid(l.X, l.Y-rise-1) {
			rise++
		}
		switch {
		case rise <= 2:
			l.Y -= rise
		case rise <= 6:
			l.Y -= 2
			l.State |= Jumping
			l.Frame = 8
		case l.climber:
			l.State |= Climbing
			l.Frame = 0
		default:
			l.Dir = -l.Dir
		}
	} else {
		down := 1
		for down <= 3 && !g.solid(l.X, l.Y+down) {
			down++
		}
		if down <= 3 {
			l.Y += down
		} else {
			l.Y += 4
			l.State |= Falling
			l.Frame = 0
			l.fall = 3
		}
	}
	if l.Y < topRow {
		l.Y = topRow
		l.Dir = -l.Dir
	}
}

func (g *Game) climb(l *Lemming) {
	l.Frame = (l.Frame + 1) & 7
	if l.Frame < 4 {
		if !g.solid(l.X, l.Y-7-l.Frame) {
			l.State = l.State&^Climbing | Hoisting
			l.Y += 2 - l.Frame
			l.Frame = 0
		}
		return
	}
	l.Y--
	head := g.solid(l.X-l.Dir, l.Y-8)
	if l.Y > 12 && !head {
		return
	}
	if head {
		l.X -= 2 * l.Dir
	}
	l.Dir = -l.Dir
	l.State = l.State&^Climbing | Falling
	l.Frame = 0
	l.fall = 0
}

// build runs one update of the builder's cycle and reports whether its position was stored.
func (g *Game) build(l *Lemming) bool {
	l.Frame = (l.Frame + 1) & 15
	switch l.Frame {
	case 9:
		x := l.X
		if l.Dir < 0 {
			x -= 4
		}
		if y := l.Y - 1; y >= 0 && y < mapH {
			for i := max(x, 0); i < min(x+6, mapW); i++ {
				g.terrain[y*mapW+i] |= 8
			}
		}
		return false
	case 10:
		if l.bricks <= 3 {
			g.sound(l.X, 0x1C, 0x06)
			g.stray(l.X)
		}
	case 0:
		g.buildStep(l)
	}
	return true
}

func (g *Game) buildStep(l *Lemming) {
	turn := func() {
		l.Dir = -l.Dir
		l.State = 0
	}
	edge := func() bool { return l.X <= 16 || l.X >= 1608 }
	l.X += l.Dir
	l.Y--
	switch {
	case edge(), g.solid(l.X, l.Y-1):
		turn()
	case g.solid(l.X+l.Dir, l.Y-1):
		l.X += l.Dir
		turn()
	default:
		l.X += l.Dir
		l.bricks--
		switch {
		case l.bricks == 0:
			l.State = Shrugging
		case g.solid(l.X+2*l.Dir, l.Y-9), edge():
			turn()
		case l.Y < 9:
			l.State = 0
		}
	}
	if l.Y < 8 {
		l.Y = 8
		l.Dir = -l.Dir
	}
}

// turnToWalk ends a basher's or miner's work: it turns round and walks.
func turnToWalk(l *Lemming) {
	l.State = 0
	l.Dir = -l.Dir
	l.Frame = 0
}

// bash runs one update of the basher's cycle and reports whether its position was stored.
func (g *Game) bash(l *Lemming) bool {
	l.Frame = (l.Frame + 1) & 31
	side := 0
	if l.Dir < 0 {
		side = 1
	}
	sub := l.Frame & 15
	switch {
	case sub >= 2 && sub <= 5:
		g.erase(l.X+l.Dir-8, l.Y-10, g.tables.bash[side][sub-2][:])
		if l.Frame == 5 && !g.solid(l.X+8*l.Dir, l.Y-6) && !g.solid(l.X+9*l.Dir, l.Y-6) &&
			!g.solid(l.X+10*l.Dir, l.Y-6) {
			l.State = 0
			l.Frame = 0
		}
	case sub >= 11:
		l.X += l.Dir
		if l.X < 16 || l.X >= 1608 {
			turnToWalk(l)
			return true
		}
		down := 0
		for down <= 2 && !g.solid(l.X, l.Y+down) {
			down++
		}
		l.Y += down
		if down > 2 {
			l.State = Falling
			l.Frame = 0
			l.fall = 0
			return true
		}
		c := g.cell(l.X+8*l.Dir, l.Y-8) & 15
		if c == steel || c == oneWayLeft && l.Dir >= 0 || c == oneWayRight && l.Dir < 0 {
			g.sound(l.X, 0x1A, 0x0C)
			turnToWalk(l)
		}
		return true
	}
	return false
}

// mine runs one update of the miner's cycle and reports whether its position was stored.
func (g *Game) mine(l *Lemming) bool {
	l.Frame = (l.Frame + 1) % 24
	side := 0
	if l.Dir < 0 {
		side = 1
	}
	switch l.Frame {
	case 0:
		l.Y++
		return true
	case 1:
		g.erase(l.X-8, l.Y-13, g.tables.mine[side][0][:])
	case 2:
		g.erase(l.X+l.Dir-8, l.Y-12, g.tables.mine[side][1][:])
	case 3, 15:
		for range 2 {
			l.X += l.Dir
			if l.X < 16 || l.X >= 1608 {
				g.sound(l.X, 0x1A, 0x0C)
				turnToWalk(l)
				return true
			}
		}
		if l.Frame == 3 {
			l.Y++
		}
		g.mineStep(l)
		return true
	}
	return false
}

// mineStep ends mining after a forward move when the ground gives out or the way is blocked.
func (g *Game) mineStep(l *Lemming) {
	if !g.solid(l.X, l.Y) {
		l.State = Falling
		l.Frame = 0
		l.fall = 0
		return
	}
	// The game never tests for a rightward one-way cell, so a left-facing miner mines through one.
	for _, c := range [2]uint8{g.cell(l.X, l.Y), g.cell(l.X+8*l.Dir, l.Y-8)} {
		if c&15 == steel || c&15 == oneWayLeft && l.Dir >= 0 {
			g.sound(l.X, 0x1A, 0x0C)
			turnToWalk(l)
			return
		}
	}
}

// erase clears the map pixels under the 0 bits of a 16-pixel-wide mask with top-left (x, y).
func (g *Game) erase(x, y int, mask []uint16) {
	for r, bits := range mask {
		for c := range 16 {
			px, py := x+c, y+r
			if bits&(0x8000>>c) == 0 && px >= 0 && px < mapW && py >= 0 && py < mapH {
				g.terrain[py*mapW+px] = 0
			}
		}
	}
}
