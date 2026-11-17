package screen

// faces lists the title's blinking faces in turn: face image and top-left.
var faces = [8]struct{ img, x, y int }{
	{4, 88, 144}, {1, 128, 64}, {6, 216, 136}, {0, 16, 64},
	{5, 152, 144}, {3, 24, 144}, {2, 248, 56}, {7, 280, 136},
}

const (
	stripW   = 256 // the credits paper, screen x 32-287
	stripH   = 24  // screen rows 208-231
	stripTop = 208
	pause    = 100 // frames a complete credits message stands
)

// Title is the title menu's animation: the blinking faces and the credits scroller. The zero
// value is not ready; use NewTitle.
type Title struct {
	started bool // the first animated frame has run
	c, i    int  // face counter 0-15 and face number 0-7
	b       int  // roller counter 0-15
	sub     int  // scroller sub-step 0-3
	pos     int  // next byte of the credits text
	pause   int  // frames of pause left
	strip   [stripW * stripH]uint8
	column  [8 * stripH]uint8 // the paper column entering at the right
}

// NewTitle returns the animation as the title opens: counters and text at their start and the
// strip plain paper.
func (m *Menu) NewTitle() *Title {
	t := &Title{}
	paper := m.scroller[16*512:]
	for y := range stripH {
		for x := range stripW {
			t.strip[y*stripW+x] = paper[(8+y)*16+x%32/2] >> (4 * (x % 2)) & 15
		}
	}
	return t
}

// Step advances the animation by one display frame.
func (m *Menu) Step(t *Title) {
	t.started = true
	t.c = (t.c + 1) % 16
	if t.c == 0 {
		t.i = (t.i + 1) % 8
	}
	if t.pause > 0 {
		t.pause--
		return
	}
	t.b = (t.b + 1) % 16
	if t.sub == 0 {
		ch := m.credits[t.pos]
		if ch == 0 {
			t.pos = 0
			ch = m.credits[0]
		}
		t.pos++
		if ch == 0xFF {
			t.pause = pause
			return
		}
		paper := m.scroller[16*512:]
		for y := range stripH {
			for x := range 8 {
				t.column[y*8+x] = paper[(8+y)*16+x/2] >> (4 * (x % 2)) & 15
			}
		}
		g := m.font[(91+int(ch)-32)*64:][:64]
		for y := range 16 {
			for x := range 8 {
				if v := g[4*y+x/2] >> (4 * (x % 2)) & 15; v != 0 {
					t.column[(4+y)*8+x] = v
				}
			}
		}
	}
	for y := range stripH {
		row := t.strip[y*stripW:][:stripW]
		copy(row, row[2:])
		row[stripW-2], row[stripW-1] = t.column[y*8+2*t.sub], t.column[y*8+2*t.sub+1]
	}
	t.sub = (t.sub + 1) % 4
}

// TitleScreen returns the title menu: the picture, the signs for rating (0 Fun … 3 Mayhem) and
// mode, the current face, the scroller. Before the first animated frame the strip is bare paper
// and neither the rollers nor a blinking face are drawn.
func (m *Menu) TitleScreen(t *Title, rating int, effects bool) *Screen {
	s := newScreen(m.TitlePal)
	copy(s.Pix[16*screenW:], m.title)
	mode := 0
	if effects {
		mode = 1
	}
	image4(s, m.at(musicSigns+1024*mode), 64, 32, 0, 192, 152, true)
	image4(s, m.at(ratingSigns+1024*rating), 64, 32, 0, 256, 152, true)
	for y := range stripH {
		copy(s.Pix[(stripTop+y)*screenW+32:], t.strip[y*stripW:][:stripW])
	}
	if !t.started {
		return s
	}
	f := faces[t.i]
	image4(s, m.at(facesAddr+512*f.img+128*((t.c/4+1)%4)), 16, 16, 0, f.x, f.y, true)
	image4(s, m.scroller[512*(t.b/2):], 32, 32, 8, 0, stripTop, true)
	image4(s, m.scroller[512*(8+t.b/2):], 32, 32, 8, 288, stripTop, true)
	return s
}
