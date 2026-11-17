// Package screen composes the in-level skill panel: status line, button counts, highlight, minimap,
// view rectangle and lemming dots; and the splash screens, title menu, access-code screen and the
// screens before and after a level.
package screen

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"strconv"

	"github.com/benemon/openlemm/internal/disc"
	"github.com/benemon/openlemm/internal/level"
)

const (
	width  = 320
	height = 40 // screen rows 184-223; panel row r is screen row 184 + r

	progBase = 0x8000 // LemProg load address
	fontAddr = 0x25CC8
	fontLen  = 59 * 64
	digAddr  = 0x2543C
	digLen   = 22 * 32
)

// Panel holds the panel art, palette and fonts read from the disc.
type Panel struct {
	art    []byte // 320×40 colour indices, values 2 and 3 exchanged as on load
	pal    color.Palette
	font   []byte // status font: 59 glyphs of 16 rows × 4 bytes, codes 32-90
	digits []byte // skill digits: 22 glyphs of 8 rows × 4 bytes
}

// State is what one frame of the panel shows.
type State struct {
	Scroll   int     // view's left edge, map x
	Buttons  [10]int // minimum rate, current rate, then climber … digger counts; each 0-99
	Skill    int     // selected skill, 0 climber … 7 digger
	Type     string  // selected lemming's type name; "" when none or nameless
	Count    int     // lemmings under the pointer
	InPlay   int
	Saved    int // percentage
	Minutes  int
	Seconds  int
	Lemmings []image.Point // released lemmings in play: map x, feet row
}

// Load reads the panel's art and palette from d and its fonts from prog, the unpacked LemProg.
func Load(d *disc.Disc, prog []byte) (*Panel, error) {
	art, err := d.Unpack("/!Lemmings/Lemmings/LemGrafix/Arcpan1p")
	if err != nil {
		return nil, err
	}
	if len(art) != width*height/2 {
		return nil, fmt.Errorf("Arcpan1p: %d bytes, want %d", len(art), width*height/2)
	}
	if fontAddr-progBase+fontLen > len(prog) || digAddr-progBase+digLen > len(prog) {
		return nil, fmt.Errorf("LemProg: %d bytes, too short for the fonts", len(prog))
	}
	dat, err := d.Unpack("/!Lemmings/Lemmings/LemLevels/LEVELDAT")
	if err != nil {
		return nil, err
	}
	if len(dat) < 112 {
		return nil, fmt.Errorf("LEVELDAT: %d bytes, too short", len(dat))
	}

	p := &Panel{
		art:    make([]byte, width*height),
		pal:    make(color.Palette, 16),
		font:   prog[fontAddr-progBase:][:fontLen],
		digits: prog[digAddr-progBase:][:digLen],
	}
	swap := func(v byte) byte {
		switch v {
		case 2:
			return 3
		case 3:
			return 2
		}
		return v
	}
	for i, b := range art {
		p.art[2*i] = swap(b & 15)
		p.art[2*i+1] = swap(b >> 4)
	}
	// Words 40-55 are the same in all five LEVELDAT blocks.
	for i := range p.pal {
		c := binary.BigEndian.Uint16(dat[80+2*int(swap(byte(i))):])
		p.pal[i] = color.RGBA{uint8(c>>8&15) * 17, uint8(c>>4&15) * 17, uint8(c&15) * 17, 255}
	}
	return p, nil
}

// Palette returns the panel's palette.
func (p *Panel) Palette() color.Palette { return p.pal }

// glyph draws status-font glyph g at cell c of the status line.
func (p *Panel) glyph(img *image.Paletted, c, g int) {
	for y := range 16 {
		for x := range 8 {
			b := p.font[g*64+y*4+x/2]
			img.Pix[y*img.Stride+8*c+x] = b >> (4 * (x % 2)) & 15
		}
	}
}

// text draws s from cell c, padded with the empty glyph to n cells.
func (p *Panel) text(img *image.Paletted, c, n int, s string) error {
	if len(s) > n {
		return fmt.Errorf("status text %q longer than %d cells", s, n)
	}
	for i := range n {
		g := 0
		if i < len(s) {
			if s[i] < 32 || s[i] > 90 {
				return fmt.Errorf("status text %q: %q not in the font", s, s[i])
			}
			g = int(s[i]) - 32
		}
		p.glyph(img, c+i, g)
	}
	return nil
}

// digit draws skill-digit glyph g's columns x0 … x0+5 at panel x, rows 17-24.
func (p *Panel) digit(img *image.Paletted, x, g, x0 int) {
	for y := range 8 {
		for i := range 6 {
			b := p.digits[g*32+y*4+(x0+i)/2]
			img.Pix[(17+y)*img.Stride+x+i] = b >> (4 * ((x0 + i) % 2)) & 15
		}
	}
}

func outline(img *image.Paletted, r image.Rectangle, c uint8) {
	for x := r.Min.X; x < r.Max.X; x++ {
		img.SetColorIndex(x, r.Min.Y, c)
		img.SetColorIndex(x, r.Max.Y-1, c)
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		img.SetColorIndex(r.Min.X, y, c)
		img.SetColorIndex(r.Max.X-1, y, c)
	}
}

// Draw composes the panel, screen rows 184-223, as 320×40 colour indices in the panel palette.
// The minimap is built from l.Map, the terrain as loaded.
func (p *Panel) Draw(l *level.Level, s State) (*image.Paletted, error) {
	img := image.NewPaletted(image.Rect(0, 0, width, height), p.pal)
	copy(img.Pix, p.art)

	for _, f := range []struct {
		c int
		s string
	}{{0, "WALKER"}, {14, "OUT"}, {23, "IN"}, {28, "%"}, {31, "TIME"}, {37, "-"}} {
		if err := p.text(img, f.c, len(f.s), f.s); err != nil {
			return nil, err
		}
	}
	if err := p.text(img, 9, 3, strconv.Itoa(s.Count)); err != nil {
		return nil, err
	}
	var err error
	if s.Type == "" {
		err = p.text(img, 0, 12, "")
	} else {
		err = p.text(img, 0, 8, s.Type)
	}
	if err != nil {
		return nil, err
	}
	if err := p.text(img, 18, 3, strconv.Itoa(s.InPlay)); err != nil {
		return nil, err
	}
	// The percentage shows tens and units only, so 100% reads "00".
	if err := p.text(img, 26, 2, fmt.Sprintf("%02d", s.Saved%100)); err != nil {
		return nil, err
	}
	if err := p.text(img, 36, 1, strconv.Itoa(s.Minutes%10)); err != nil {
		return nil, err
	}
	if err := p.text(img, 38, 2, fmt.Sprintf("%02d", s.Seconds)); err != nil {
		return nil, err
	}

	for i, n := range s.Buttons {
		if n < 0 || n > 99 {
			return nil, fmt.Errorf("button %d: %d not in 0-99", i, n)
		}
		tens, units := 10, 21
		if n > 0 {
			tens, units = n/10, 11+n%10
		}
		p.digit(img, 16*i+2, tens, 2)
		p.digit(img, 16*i+8, units, 0)
	}

	// Minimap pixel (i, j): any solid pixel (colour 8-15) at map x 16i+2 … 16i+5, map row
	// 8j+7 (level row 8j+3).
	// Objects are left out because some of their pixels use colours 8-15.
	t := l.Map
	for j := range 20 {
		for i := range 96 {
			var v uint8
			for x := 16*i + 2; x <= 16*i+5; x++ {
				if t.ColorIndexAt(x, 8*j+7)&8 != 0 {
					v = 1
				}
			}
			img.Pix[(18+j)*img.Stride+208+i] = v
		}
	}

	if s.Skill < 0 || s.Skill > 7 {
		return nil, fmt.Errorf("skill %d not in 0-7", s.Skill)
	}
	outline(img, image.Rect(16*(s.Skill+2), 16, 16*(s.Skill+2)+16, 40), 3)
	vx := 206 + s.Scroll>>4
	outline(img, image.Rect(vx, 16, vx+25, 40), 2)
	for _, m := range s.Lemmings {
		img.SetColorIndex(208+m.X>>4, 17+m.Y>>3, 3)
	}
	return img, nil
}
