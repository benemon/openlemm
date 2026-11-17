package screen

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/benemon/openlemm/internal/disc"
	"github.com/benemon/openlemm/internal/level"
)

// Addresses of the menu and between-level data in LemProg as loaded.
const (
	titleEnd     = 0x12C18 // the title picture's packed stream ends here
	scrollerEnd  = 0x157F8
	fontEnd      = 0x16948
	facesAddr    = 0x12C18
	ratingSigns  = 0x13C18
	musicSigns   = 0x14C18
	creditsAddr  = 0x16948
	tileAddr     = 0x17D60
	busyAddr     = 0x28800
	handAddr     = 0x28A00
	titlePalAddr = 0x198D4
	codesAddr    = 0x18DE0
	ratingsAddr  = 0x18797
	verdictAddr  = 0x18955

	congratsText  = 0x18560
	pressText     = 0x186A1
	tryAgainText  = 0x186C3
	preLevelText1 = 0x18703
	preLevelText2 = 0x18744
	timeUpText    = 0x18880
	allOutText    = 0x18894
	percentText   = 0x188B3
	accessText    = 0x18C64
	promptText    = 0x18CD2
	fieldText     = 0x18CEB
	blankText     = 0x18D07
	foundText     = 0x18D15
	incorrectText = 0x18D3E
	cheatText     = 0x18D50
	showText      = 0x18D77
)

// screenW and screenH are the screen's size in pixels.
const (
	screenW = 320
	screenH = 256
)

// Screen is one displayed frame: colour indices, shown in Upper above row Split and in Lower from
// it.
type Screen struct {
	Pix          []uint8
	Upper, Lower color.Palette
	Split        int
}

func newScreen(p color.Palette) *Screen {
	return &Screen{Pix: make([]uint8, screenW*screenH), Upper: p, Lower: p, Split: screenH}
}

// record is one text record: a line of text at a column and text row.
type record struct {
	col, row int
	text     []byte
}

// Menu holds what the title menu, the access-code screen and the screens around a level draw,
// read from the disc.
type Menu struct {
	prog     []byte
	font     []byte // large font: 3 colour sets × 91 glyphs of 8×16, codes 32-122
	title    []byte // the title picture, 320×200 colour indices
	TitlePal color.Palette
	scroller []byte // 17 images of 32×32: left roller 0-7, right roller 8-15, paper 16
	credits  []byte // 32-character messages each ended by &FF; a zero byte ends the text
	splash   [3][]byte
	splashP  [3]color.Palette
	texts    map[int][]record
}

// pal reads 16 Amiga-order 0RGB big-endian words.
func pal(b []byte) (color.Palette, error) {
	p := make(color.Palette, 16)
	for i := range p {
		c := binary.BigEndian.Uint16(b[2*i:])
		if c > 0xFFF {
			return nil, fmt.Errorf("colour %d is %#x, not 0RGB", i, c)
		}
		p[i] = color.RGBA{uint8(c>>8) * 17, uint8(c>>4&15) * 17, uint8(c&15) * 17, 255}
	}
	return p, nil
}

// nibbles unpacks 4bpp pixels, low nibble first.
func nibbles(b []byte) []byte {
	pix := make([]byte, 2*len(b))
	for i, v := range b {
		pix[2*i], pix[2*i+1] = v&15, v>>4
	}
	return pix
}

// LoadMenu reads the menus' art, texts and palettes from LemProg, the splash images and their
// palettes in LemBoot.
func LoadMenu(d *disc.Disc, prog []byte) (*Menu, error) {
	if len(prog) < handAddr+512-progBase {
		return nil, fmt.Errorf("LemProg: %d bytes, too short for the menus", len(prog))
	}
	m := &Menu{prog: prog, texts: map[int][]record{}}
	for _, s := range []struct {
		dst  *[]byte
		end  int
		size int
	}{{&m.title, titleEnd, 32000}, {&m.scroller, scrollerEnd, 8704}, {&m.font, fontEnd, 17472}} {
		b, err := disc.Unpack(prog[:s.end-progBase])
		if err != nil {
			return nil, fmt.Errorf("LemProg stream ending &%X: %w", s.end, err)
		}
		if len(b) != s.size {
			return nil, fmt.Errorf("LemProg stream ending &%X: %d bytes, want %d", s.end, len(b), s.size)
		}
		*s.dst = b
	}
	m.title = nibbles(m.title)
	var err error
	if m.TitlePal, err = pal(m.at(titlePalAddr)); err != nil {
		return nil, fmt.Errorf("title palette: %w", err)
	}

	c := m.at(creditsAddr)
	n := 0
	for c[n] != 0 {
		if c[n+32] != 0xFF {
			return nil, fmt.Errorf("credits message at &%X: no &FF after 32 characters", creditsAddr+n)
		}
		for _, ch := range c[n : n+32] {
			if ch < 32 || ch > 122 {
				return nil, fmt.Errorf("credits message at &%X: character %#x not in the font", creditsAddr+n, ch)
			}
		}
		n += 33
	}
	m.credits = c[:n+1]

	for _, a := range []int{congratsText, pressText, tryAgainText, preLevelText1, preLevelText2,
		timeUpText, allOutText, percentText, accessText, promptText, fieldText, blankText,
		foundText, incorrectText, cheatText, showText} {
		if m.texts[a], err = m.records(a); err != nil {
			return nil, err
		}
	}
	for k := range 9 {
		a := verdictAddr + 87*k
		if m.texts[a], err = m.records(a); err != nil {
			return nil, err
		}
	}
	for r := range 122 {
		for _, b := range m.code(r) {
			if r > 0 && b > 25 {
				return nil, fmt.Errorf("access code %d: byte %d is not a letter", r, b)
			}
		}
	}

	boot, ok := d.Files["/!Lemmings/LemBoot"]
	if !ok {
		return nil, fmt.Errorf("/!Lemmings/LemBoot: not on disc")
	}
	for k, s := range []struct {
		name string
		pal  int
	}{{"Krisalis", 0xC41}, {"Psygno", 0xCD7}, {"DmaLogo", 0xD65}} {
		b, err := d.Unpack("/!Lemmings/Lemmings/" + s.name)
		if err != nil {
			return nil, err
		}
		if len(b) != 32000 {
			return nil, fmt.Errorf("%s: %d bytes, want 32000", s.name, len(b))
		}
		m.splash[k] = nibbles(b)
		if len(boot) < s.pal+32 {
			return nil, fmt.Errorf("LemBoot: too short for the %s palette", s.name)
		}
		if m.splashP[k], err = pal(boot[s.pal:]); err != nil {
			return nil, fmt.Errorf("%s palette: %w", s.name, err)
		}
	}
	return m, nil
}

// at returns LemProg from address a.
func (m *Menu) at(a int) []byte { return m.prog[a-progBase:] }

// records parses the text records at address a: column, row, text, a zero byte; a column of
// &FF ends the list.
func (m *Menu) records(a int) ([]record, error) {
	b := m.at(a)
	var rs []record
	o := 0
	for o < len(b) && b[o] != 0xFF {
		r := record{col: int(b[o]), row: int(b[o+1])}
		o += 2
		for o < len(b) && b[o] != 0 {
			if c := b[o]; c > 122 {
				return nil, fmt.Errorf("text at &%X: character %#x not in the font", a, c)
			}
			r.text = append(r.text, b[o])
			o++
		}
		o++
		rs = append(rs, r)
	}
	if o >= len(b) {
		return nil, fmt.Errorf("text at &%X: no end", a)
	}
	return rs, nil
}

// text returns a copy of the records at address a.
func (m *Menu) text(a int) []record {
	rs := make([]record, len(m.texts[a]))
	for i, r := range m.texts[a] {
		rs[i] = record{r.col, r.row, append([]byte(nil), r.text...)}
	}
	return rs
}

// number writes n into the field of w places at place i of t: decimal, no leading zeros, from
// the field's first place, ':' in the places left over.
func number(t []byte, i, w, n int) {
	s := strconv.Itoa(n)
	for k := range w {
		t[i+k] = ':'
		if k < len(s) {
			t[i+k] = s[k]
		}
	}
}

// draw draws records, each character in an 8×16 cell from x = 8 × column, y = 16 + 16 × row + dy,
// with colour set (row + 1) mod 3. A ':' takes no cell; codes 1-31 take a cell and draw nothing;
// a zero ends the text.
// Transparent text leaves colour 0 alone; opaque text writes the whole cell.
func (m *Menu) draw(s *Screen, rs []record, opaque bool, dy int) {
	for _, r := range rs {
		set := (r.row + 1) % 3
		x := 8 * r.col
		for _, c := range r.text {
			if c == 0 {
				break // only bytes read past a table (access codes after row 121) hold a zero
			}
			if c == ':' {
				continue
			}
			// Codes above 122 are past the font; they appear only in access codes after row 121,
			// and what the game draws for them is unknown: here they take a cell and draw nothing.
			if c >= 32 && c <= 122 {
				m.glyph(s, x, 16+16*r.row+dy, set, int(c)-32, opaque)
			}
			x += 8
		}
	}
}

// glyph draws glyph g of colour set set with its top-left at (x, y).
func (m *Menu) glyph(s *Screen, x, y, set, g int, opaque bool) {
	b := m.font[(set*91+g)*64:][:64]
	for yy := range 16 {
		for xx := range 8 {
			v := b[4*yy+xx/2] >> (4 * (xx % 2)) & 15
			if X, Y := x+xx, y+yy; (v != 0 || opaque) && X >= 0 && X < screenW && Y >= 0 && Y < screenH {
				s.Pix[Y*screenW+X] = v
			}
		}
	}
}

// tile fills screen rows y0 to y1-1 with the 64×64 background tile; screen row y shows tile row
// (y - 16) mod 64.
func (m *Menu) tile(s *Screen, y0, y1 int) {
	t := m.at(tileAddr)
	for y := y0; y < y1; y++ {
		row := t[32*((y-16)&63):]
		for x := range screenW {
			s.Pix[y*screenW+x] = row[(x&63)/2] >> (4 * (x % 2)) & 15
		}
	}
}

// image4 draws a w×h row-major 4bpp image b with its top-left at (x, y), rows r0 onwards only,
// colour 0 included when opaque.
func image4(s *Screen, b []byte, w, h, r0, x, y int, opaque bool) {
	for yy := r0; yy < h; yy++ {
		for xx := range w {
			v := b[yy*w/2+xx/2] >> (4 * (xx % 2)) & 15
			if X, Y := x+xx, y+yy-r0; (v != 0 || opaque) && X >= 0 && X < screenW && Y >= 0 && Y < screenH {
				s.Pix[Y*screenW+X] = v
			}
		}
	}
}

// Pointer draws the menu pointer, the busy image or the pointing hand, with its top-left at
// (x, y); colour 0 is transparent and screen rows 209 and below are left alone.
func (m *Menu) Pointer(s *Screen, x, y int, busy bool) {
	a := handAddr
	if busy {
		a = busyAddr
	}
	b := m.at(a)
	for yy := range 32 {
		for xx := range 32 {
			v := b[16*yy+xx/2] >> (4 * (xx % 2)) & 15
			if X, Y := x+xx, y+yy; v != 0 && X >= 0 && X < screenW && Y >= 0 && Y < 209 {
				s.Pix[Y*screenW+X] = v
			}
		}
	}
}

// Splash returns splash k (0 Krisalis, 1 Psygnosis, 2 DMA Design) at screen rows 16-215, the rest
// of the screen colour 0 of its palette.
func (m *Menu) Splash(k int) *Screen {
	s := newScreen(m.splashP[k])
	copy(s.Pix[16*screenW:], m.splash[k])
	return s
}

// code returns row r of the access-code table.
func (m *Menu) code(r int) []byte { return m.at(codesAddr + 10*r)[:10] }

// Code returns the access code of level n (1-119) as letters.
func (m *Menu) Code(n int) string {
	b := make([]byte, 10)
	for i, v := range m.code(n) {
		b[i] = 'A' + v
	}
	return string(b)
}

// Match returns the first row of the access-code table, 1-121, that field (letters A-Z, '-' for
// an empty place) equals, or 0 for none.
func (m *Menu) Match(field [10]byte) int {
	for r := 1; r < 122; r++ {
		if string(field[:]) == m.Code(r) {
			return r
		}
	}
	return 0
}

// ratingName returns the 8-character name of rating r (0 Fun … 3 Mayhem).
func (m *Menu) ratingName(r int) []byte { return m.at(ratingsAddr + 8*r)[:8] }

// CodeResult is what the access-code screen shows below the field.
type CodeResult int

// Access-code screen results.
const (
	Typing    CodeResult = iota // the field is being typed
	Found                       // a level's code
	ShowMode                    // the code for shows
	CheatMode                   // the cheat code
	Incorrect                   // no match
)

// AccessCode returns the access-code screen: the field holding field (letters A-Z, '-' for an
// empty place) with, while typing, the cursor under place p; otherwise the result line for res,
// level being the level a Found code selects (0-119), and the prompt to press a mouse button.
func (m *Menu) AccessCode(field [10]byte, p int, res CodeResult, level int) *Screen {
	s := newScreen(m.TitlePal)
	m.tile(s, 16, 224)
	m.draw(s, m.text(promptText), false, 0)
	cur := m.text(blankText)
	if res == Typing {
		// The cursor record holds '^' at its first place; the game shows a '-' at p.
		cur[0].text[p] = '-'
	}
	m.draw(s, cur, true, -6)
	f := m.text(fieldText)
	copy(f[0].text, field[:])
	m.draw(s, f, true, 0)
	switch res {
	case Typing:
		return s
	case Found:
		t := m.text(foundText)
		number(t[0].text, 15, 2, level%30+1)
		copy(t[1].text[9:], m.ratingName(level/30))
		m.draw(s, t, false, 0)
	case ShowMode:
		m.draw(s, m.text(showText), false, 0)
	case CheatMode:
		m.draw(s, m.text(cheatText), false, 0)
	case Incorrect:
		m.draw(s, m.text(incorrectText), false, 0)
	}
	m.draw(s, m.text(pressText), false, 0)
	return s
}

// PreLevel returns the screen before level l, which is level n (0-119) in play order: the
// preview at rows 32-71 in the level's palette, the tile and text below in the title palette.
func (m *Menu) PreLevel(l *level.Level, n int) *Screen {
	lp := append(color.Palette{color.Black}, l.Map.Palette[1:]...)
	s := &Screen{Pix: make([]uint8, screenW*screenH), Upper: lp, Lower: m.TitlePal, Split: 76}

	pic := image.NewPaletted(image.Rect(0, 0, l.Map.Rect.Dx(), 160), lp)
	copy(pic.Pix, l.Map.Pix[4*l.Map.Stride:])
	for i := range l.Objects {
		l.Objects[i].Draw(pic, l.Objects[i].Preview)
	}
	for j := range 40 {
		for i := range screenW {
			s.Pix[(32+j)*screenW+i] = pic.Pix[4*j*pic.Stride+5*i]
		}
	}

	m.tile(s, 80, 224)
	t1, t2 := m.text(preLevelText1), m.text(preLevelText2)
	number(t1[0].text, 6, 2, n%30+1)
	copy(t1[1].text, l.Title)
	number(t1[2].text, 13, 2, l.ReleaseRate)
	number(t2[0].text, 19, 3, l.Lemmings)
	number(t2[1].text, 0, 3, l.ToSave*100/max(l.Lemmings, 1))
	t2[2].text[8] = byte('0' + l.Minutes%10)
	if l.Minutes == 1 {
		t2[2].text = t2[2].text[:len(t2[2].text)-1]
	}
	copy(t2[3].text[7:], m.ratingName(n/30))
	m.draw(s, t1, false, 0)
	m.draw(s, t2, false, 0)
	m.draw(s, m.text(pressText), false, 0)
	return s
}

// Results is what the screen after a level shows.
type Results struct {
	TimeUp          bool // the level ended by time-up or Escape
	Needed, Rescued int  // percentages
	// Next is the level that follows in play order, 1-119, or 120 after the last level.
	Next int
}

// Passed reports whether the level was passed.
func (r Results) Passed() bool { return r.Rescued >= r.Needed }

// Verdict returns the verdict message number, 0-8, for rescued r and needed q percentages.
func Verdict(r, q int) int {
	switch {
	case r == 100:
		return 8
	case r == 0:
		return 0
	case r < q/2:
		return 1
	case r < q-5:
		return 2
	case r < q-1:
		return 3
	case r == q-1:
		return 4
	case r == q:
		return 5
	case r < q+20:
		return 6
	}
	return 7
}

// ResultsScreen returns the screen after a level.
func (m *Menu) ResultsScreen(r Results) *Screen {
	s := newScreen(m.TitlePal)
	m.tile(s, 16, 224)
	if r.TimeUp {
		m.draw(s, m.text(timeUpText), false, 0)
	} else {
		m.draw(s, m.text(allOutText), false, 0)
	}
	p := m.text(percentText)
	number(p[0].text, 12, 3, r.Rescued)
	number(p[1].text, 12, 3, r.Needed)
	m.draw(s, p, false, 0)
	switch {
	case r.Passed() && r.Next == 120:
		m.draw(s, m.text(pressText), false, 0)
		return s
	case r.Passed():
		a := m.text(accessText)
		number(a[0].text, 27, 2, r.Next%30+1)
		copy(a[1].text[3:], m.Code(r.Next))
		m.draw(s, a, false, 0)
		m.draw(s, m.text(pressText), false, 0)
	default:
		m.draw(s, m.text(tryAgainText), false, 0)
	}
	m.draw(s, m.text(verdictAddr+87*Verdict(r.Rescued, r.Needed)), false, 0)
	return s
}

// Congratulations returns the screen after the last level.
func (m *Menu) Congratulations() *Screen {
	s := newScreen(m.TitlePal)
	m.tile(s, 16, 224)
	m.draw(s, m.text(congratsText), false, 0)
	m.draw(s, m.text(pressText), false, 0)
	return s
}

// Line is one line of text placed with its top-left at pixel (X, Y).
type Line struct {
	X, Y int
	Text string
}

// Layout wraps each paragraph at word boundaries to 40 columns of the large font, centres each
// line, leaves a blank line between paragraphs and centres the whole on screen rows top to
// top+rows-1.
func Layout(paras []string, top, rows int) []Line {
	var ls []Line
	for i, p := range paras {
		if i > 0 {
			ls = append(ls, Line{})
		}
		line := ""
		for _, w := range strings.Fields(p) {
			if line != "" && len(line)+1+len(w) > screenW/8 {
				ls = append(ls, Line{Text: line})
				line = ""
			}
			if line != "" {
				line += " "
			}
			line += w
		}
		ls = append(ls, Line{Text: line})
	}
	y := top + (rows-16*len(ls))/2
	for i := range ls {
		ls[i].X, ls[i].Y = (screenW-8*len(ls[i].Text))/2, y+16*i
	}
	return ls
}

// Notice draws lines in colour set 0 of the large font on black, in the title palette.
func (m *Menu) Notice(ls []Line) *Screen {
	s := newScreen(m.TitlePal)
	for _, l := range ls {
		for i, c := range []byte(l.Text) {
			m.glyph(s, l.X+8*i, l.Y, 0, int(c)-32, false)
		}
	}
	return s
}
