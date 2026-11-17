// Package level loads the game's level records and draws their terrain and objects.
package level

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"slices"

	"github.com/benemon/openlemm/internal/disc"
)

const (
	width  = 1632 // the game's map: the 1600-pixel level with a 16-pixel margin either side
	height = 160  // level rows in Image
	// The game's terrain map is mapHeight rows; terrain is kept only in map rows 0 to mapKept-1.
	mapHeight = 176
	mapKept   = 163

	levelDir   = "/!Lemmings/Lemmings/LemLevels/"
	grafixDir  = "/!Lemmings/Lemmings/LemGrafix/"
	recordSize = 2048
	blockSize  = 0x590

	// specialX is the map x of a special backdrop: level x 304.
	specialX = 320
)

var objFiles = [5]string{"ArcObj1P", "ArcObj2P", "Arcobj3P", "ArcObj4P", "Arcobj5P"}

// Level is one level record with its terrain and objects drawn.
type Level struct {
	ReleaseRate int
	Lemmings    int
	ToSave      int
	Minutes     int
	Skills      [8]int
	StartX      int
	tileset     int
	special     int
	Objects     []Object
	terrain     []terrainPiece
	Steel       []image.Rectangle // map x
	Title       string            // 32 characters as stored, padding included
	// Image is level rows 0-159 of the map, terrain and objects, as palette indices with the
	// tileset's palette; colour 0 is as stored, not black.
	Image *image.Paletted
	// Map is the game's 1632×176 terrain map at level start, in map rows (level row + 4):
	// terrain pieces and any special backdrop, no objects, kept only in rows 0-162.
	Map *image.Paletted
}

// Object is a placed object; X and Y are its top-left pixel in Image (map x). The format's
// upside-down flag (byte 7 bit 0x80) is not handled: every object on the Version A disc has
// byte 7 = 0x0F.
type Object struct {
	X, Y          int
	ID            int
	NoOverwrite   bool
	OnlyOnTerrain bool
	Slot          int  // index of its entry among the record's 32
	Entrance      bool // its definition's animation type is 3, a trapdoor
	Loop          bool // its definition's animation type is 2, cycling forever
	Effect        int  // its definition's trigger effect
	Pitch, Sample int  // its definition's bytes 14 and 15: a trap's sound
	// Preview is the frame its definition's preview-frame address names.
	Preview int
	// Trigger is the definition's trigger area in 4×4 blocks, relative to block (X/4, Y/4).
	Trigger image.Rectangle

	frames []piece
}

// Frames returns the number of animation frames of o's definition.
func (o *Object) Frames() int { return len(o.frames) }

// Draw draws frame f of o into img, whose row 0 is level row 0, applying o's placement flags
// against what img already holds.
func (o *Object) Draw(img *image.Paletted, f int) {
	p := o.frames[f]
	for yy := range p.h {
		for xx := range p.w {
			X, Y := o.X+xx, o.Y+yy
			if !(image.Point{X, Y}).In(img.Rect) {
				continue
			}
			v := p.pixel(xx, yy)
			if v == 0 {
				continue
			}
			i := img.PixOffset(X, Y)
			// Only-on-terrain wins over no-overwrite; both test objects drawn earlier too.
			switch {
			case o.OnlyOnTerrain:
				if img.Pix[i] != 0 {
					img.Pix[i] = v
				}
			case o.NoOverwrite:
				if img.Pix[i] == 0 {
					img.Pix[i] = v
				}
			default:
				img.Pix[i] = v
			}
		}
	}
}

// terrainPiece is a placed terrain piece; X and Y are its top-left pixel in Image (map x).
type terrainPiece struct {
	X, Y        int
	ID          int
	NoOverwrite bool
	UpsideDown  bool
	Erase       bool
}

// piece is 4bpp pixel data stored as consecutive 16-pixel-wide columns of h rows, low nibble left.
type piece struct {
	w, h int
	data []byte
}

func newPiece(file []byte, off, w, h int) (piece, error) {
	n := (w + 15) / 16 * h * 8
	if off < 0 || off+n > len(file) {
		return piece{}, fmt.Errorf("%dx%d image at %#x past end of file", w, h, off)
	}
	return piece{w, h, file[off : off+n]}, nil
}

func (p piece) pixel(x, y int) uint8 {
	b := p.data[x/16*p.h*8+y*8+x%16/2]
	return b >> (4 * (x % 2)) & 15
}

// Load reads level record n (0-99; record = LEVELnnn × 4 + index) and draws it.
func Load(d *disc.Disc, n int) (*Level, error) {
	dat, err := d.Unpack(levelDir + "LEVELDAT")
	if err != nil {
		return nil, err
	}
	return load(d, dat, n)
}

// load reads level record n and draws it with dat, the unpacked LEVELDAT.
func load(d *disc.Disc, dat []byte, n int) (*Level, error) {
	if n < 0 || n >= 100 {
		return nil, fmt.Errorf("level record %d out of range", n)
	}
	file, err := d.Unpack(fmt.Sprintf("%sLEVEL%03d", levelDir, n/4))
	if err != nil {
		return nil, err
	}
	if len(file) < 4*recordSize {
		return nil, fmt.Errorf("LEVEL%03d: %d bytes, want %d", n/4, len(file), 4*recordSize)
	}
	r := file[n%4*recordSize:][:recordSize]
	u16 := func(o int) int { return int(binary.BigEndian.Uint16(r[o:])) }

	l := &Level{
		ReleaseRate: u16(0),
		Lemmings:    u16(2),
		ToSave:      u16(4),
		Minutes:     u16(6),
		StartX:      u16(24),
		tileset:     u16(26),
		special:     u16(28),
	}
	for i := range l.Skills {
		l.Skills[i] = u16(8 + 2*i)
	}
	for i := range 32 {
		e := r[0x20+8*i:][:8]
		// The game skips any entry whose x word is 0, whatever its other fields hold.
		if e[0] == 0 && e[1] == 0 {
			continue
		}
		l.Objects = append(l.Objects, Object{
			X:             int(int16(binary.BigEndian.Uint16(e))),
			Y:             int(int16(binary.BigEndian.Uint16(e[2:]))),
			ID:            int(binary.BigEndian.Uint16(e[4:])),
			NoOverwrite:   e[6]&0x80 != 0,
			OnlyOnTerrain: e[6]&0x40 != 0,
			Slot:          i,
		})
	}
	for i := range 400 {
		e := r[0x120+4*i:][:4]
		if [4]byte(e) == [4]byte{0xff, 0xff, 0xff, 0xff} {
			break
		}
		mod := e[0] >> 4
		y := int(e[2])<<1 | int(e[3]>>7)
		if y >= 256 {
			y -= 512
		}
		l.terrain = append(l.terrain, terrainPiece{
			X:           (int(e[0])<<8 | int(e[1])) & 0xfff,
			Y:           y - 4,
			ID:          int(e[3] & 0x3f),
			NoOverwrite: mod&8 != 0,
			UpsideDown:  mod&4 != 0,
			Erase:       mod&2 != 0,
		})
	}
	for i := range 32 {
		e := r[0x760+4*i:][:4]
		if [4]byte(e) == [4]byte{} {
			continue
		}
		x := (int(e[0])<<1 | int(e[1]>>7)) * 4
		y := int(e[1]&0x7f) * 4
		l.Steel = append(l.Steel, image.Rect(x, y, x+int(e[2]>>4)*4+4, y+int(e[2]&15)*4+4))
	}
	l.Title = string(r[0x7e0:])

	if err := l.draw(d, dat); err != nil {
		return nil, fmt.Errorf("level record %d: %w", n, err)
	}
	return l, nil
}

// draw renders l.Image from the level's tileset, terrain and objects; all is the unpacked LEVELDAT.
func (l *Level) draw(d *disc.Disc, all []byte) error {
	if l.tileset > 4 {
		return fmt.Errorf("tileset %d out of range", l.tileset)
	}
	if len(all) < (l.tileset+1)*blockSize {
		return errors.New("LEVELDAT too short")
	}
	b := all[l.tileset*blockSize:][:blockSize]
	pal := make(color.Palette, 16)
	for i := range pal {
		c := binary.BigEndian.Uint16(b[2*i:])
		pal[i] = color.RGBA{uint8(c>>8&15) * 17, uint8(c>>4&15) * 17, uint8(c&15) * 17, 255}
	}
	// Terrain is drawn in map rows 0-163, level rows -4 to 159: Map keeps rows 0-162 and Image
	// shows level rows 0-159.
	terr := image.NewPaletted(image.Rect(0, 0, width, height+4), pal)

	grn, err := d.Unpack(fmt.Sprintf("%sArcGrn%dP", grafixDir, l.tileset+1))
	if err != nil {
		return err
	}
	var terrain []piece
	var tbase int
	for k := range 64 {
		e := b[0x290+12*k:]
		w, h := int(binary.BigEndian.Uint16(e)), int(binary.BigEndian.Uint16(e[2:]))
		if w == 0 || w == 0xffff {
			break
		}
		// Addresses are absolute load addresses; the first piece is at the start of the file.
		addr := int(binary.BigEndian.Uint32(e[4:]))
		if k == 0 {
			tbase = addr
		}
		p, err := newPiece(grn, addr-tbase, w, h)
		if err != nil {
			return fmt.Errorf("terrain piece %d: %w", k, err)
		}
		terrain = append(terrain, p)
	}

	obj, err := d.Unpack(grafixDir + objFiles[l.tileset])
	if err != nil {
		return err
	}
	var objects [16][]piece
	obase := -1
	for k := range objects {
		e := b[0x70+34*k:]
		if a := int(binary.BigEndian.Uint32(e[26:])); binary.BigEndian.Uint16(e[4:]) != 0 && (obase < 0 || a < obase) {
			obase = a
		}
	}
	for k := range objects {
		e := b[0x70+34*k:]
		n := int(binary.BigEndian.Uint16(e[4:]))
		w, h := int(binary.BigEndian.Uint16(e[6:])), int(binary.BigEndian.Uint16(e[8:]))
		stride := int(binary.BigEndian.Uint16(e[10:]))
		for f := range n {
			// The game reads each frame from its offset rounded down to a multiple of 4.
			p, err := newPiece(obj, (int(binary.BigEndian.Uint32(e[26:]))-obase+f*stride)&^3, w, h)
			if err != nil {
				return fmt.Errorf("object %d frame %d: %w", k, f, err)
			}
			objects[k] = append(objects[k], p)
		}
	}

	// The backdrop fills map rows 4-163; only terrain pieces are cut after map row 162.
	var backdrop []uint8
	if l.special > 0 {
		if err := drawSpecial(d, terr, l.special); err != nil {
			return err
		}
		backdrop = slices.Clone(terr.Pix[mapKept*width:][:width])
	}

	for i, t := range l.terrain {
		if t.ID >= len(terrain) {
			return fmt.Errorf("terrain %d: piece %d not in tileset %d", i, t.ID, l.tileset)
		}
		p := terrain[t.ID]
		for yy := range p.h {
			for xx := range p.w {
				X, Y := t.X+xx, t.Y+4+yy
				if X < 0 || X >= width || Y < 0 || Y >= height+4 {
					continue
				}
				sy := yy
				if t.UpsideDown {
					sy = p.h - 1 - yy
				}
				v := p.pixel(xx, sy)
				if v == 0 {
					continue
				}
				o := Y*terr.Stride + X
				// No-overwrite takes precedence over erase.
				switch {
				case t.NoOverwrite:
					if terr.Pix[o] == 0 {
						terr.Pix[o] = v
					}
				case t.Erase:
					terr.Pix[o] = 0
				default:
					terr.Pix[o] = v
				}
			}
		}
	}
	l.Map = image.NewPaletted(image.Rect(0, 0, width, mapHeight), pal)
	copy(l.Map.Pix, terr.Pix[:mapKept*width])
	copy(l.Map.Pix[mapKept*width:], backdrop)
	img := image.NewPaletted(image.Rect(0, 0, width, height), pal)
	copy(img.Pix, terr.Pix[4*width:])
	l.Image = img

	for i, ob := range l.Objects {
		if ob.ID >= len(objects) || objects[ob.ID] == nil {
			return fmt.Errorf("object %d: id %d not in tileset %d", i, ob.ID, l.tileset)
		}
		def := func(k int) int { return int(binary.BigEndian.Uint16(b[0x70+34*ob.ID+2*k:])) }
		o := &l.Objects[i]
		o.Entrance = def(0) == 3
		o.Loop = def(0) == 2
		o.Effect = def(12)
		o.Pitch, o.Sample = def(7)>>8, def(7)&0xff
		o.Trigger = image.Rect(def(8), def(9), def(8)+def(10), def(9)+def(11))
		o.frames = objects[ob.ID]
		if stride := def(5); stride > 0 {
			addr := func(k int) int { return int(binary.BigEndian.Uint32(b[0x70+34*ob.ID+k:])) }
			o.Preview = (addr(30) - addr(26)) / stride
		}
		if o.Preview < 0 || o.Preview >= len(o.frames) {
			return fmt.Errorf("object %d: preview frame %d of %d", i, o.Preview, len(o.frames))
		}
		o.Draw(img, 0)
	}
	return nil
}

// drawSpecial draws backdrop SPECIAL{n-1} into img, whose row 4 is level row 0, and puts its CMAP
// entries 1-7 in palette slots 9-15; slot 8 keeps the tileset's colour.
func drawSpecial(d *disc.Disc, img *image.Paletted, n int) error {
	if n > 4 {
		return fmt.Errorf("special graphic %d out of range", n)
	}
	name := fmt.Sprintf("SPECIAL%d", n-1)
	b, err := d.Unpack(grafixDir + name)
	if err != nil {
		return err
	}
	cmap, w, h, pix, err := readILBM(b)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if len(cmap) < 8*3 {
		return fmt.Errorf("%s: %d colours, want 8", name, len(cmap)/3)
	}
	if specialX+w > width {
		return fmt.Errorf("%s: %d pixels wide, does not fit at x %d", name, w, specialX)
	}
	for i := 1; i < 8; i++ {
		c := cmap[3*i:]
		img.Palette[8+i] = color.RGBA{c[0] >> 4 * 17, c[1] >> 4 * 17, c[2] >> 4 * 17, 255}
	}
	for y := range min(h, height) {
		for x := range w {
			if v := pix[y*w+x]; v != 0 {
				img.Pix[(y+4)*img.Stride+specialX+x] = 8 + v
			}
		}
	}
	return nil
}

// readILBM decodes an IFF ILBM image with ByteRun1 compression and no interleaved mask plane,
// returning its CMAP and its pixels as w×h colour indices.
func readILBM(b []byte) (cmap []byte, w, h int, pix []byte, err error) {
	if len(b) < 12 || string(b[:4]) != "FORM" || string(b[8:12]) != "ILBM" {
		return nil, 0, 0, nil, errors.New("not an IFF ILBM")
	}
	chunks := map[string][]byte{}
	for o := 12; o < len(b)-8; {
		n := int(binary.BigEndian.Uint32(b[o+4:]))
		if o+8+n > len(b) {
			return nil, 0, 0, nil, fmt.Errorf("chunk %q past end of file", b[o:o+4])
		}
		chunks[string(b[o:o+4])] = b[o+8 : o+8+n]
		o += 8 + n + n&1
	}
	bmhd, body := chunks["BMHD"], chunks["BODY"]
	cmap = chunks["CMAP"]
	if len(bmhd) < 11 || body == nil {
		return nil, 0, 0, nil, errors.New("missing BMHD or BODY")
	}
	w, h = int(binary.BigEndian.Uint16(bmhd)), int(binary.BigEndian.Uint16(bmhd[2:]))
	planes, mask, comp := int(bmhd[8]), bmhd[9], bmhd[10]
	if comp != 1 || mask == 1 || planes > 8 {
		return nil, 0, 0, nil, fmt.Errorf("unsupported ILBM: compression %d, masking %d, %d planes", comp, mask, planes)
	}
	errShort := errors.New("BODY truncated")
	rb := (w + 15) / 16 * 2
	pix = make([]byte, w*h)
	line := make([]byte, 0, rb+128)
	p := 0
	for y := range h {
		for k := range planes {
			line = line[:0]
			for len(line) < rb {
				if p >= len(body) {
					return nil, 0, 0, nil, errShort
				}
				c := int(body[p])
				switch {
				case c < 128:
					if p+c+2 > len(body) {
						return nil, 0, 0, nil, errShort
					}
					line = append(line, body[p+1:p+c+2]...)
					p += c + 2
				case c > 128:
					if p+1 >= len(body) {
						return nil, 0, 0, nil, errShort
					}
					for range 257 - c {
						line = append(line, body[p+1])
					}
					p += 2
				default:
					p++
				}
			}
			for x := range w {
				pix[y*w+x] |= (line[x>>3] >> (7 - x&7) & 1) << k
			}
		}
	}
	return cmap, w, h, pix, nil
}

// Levels is the number of words in LEVELDAT's level-order table: the 120 levels, 40 words of
// padding naming the last Mayhem level's record, and the 20 two-player levels.
const Levels = 180

// playOrder reads the level-order table from all, the unpacked LEVELDAT.
func playOrder(all []byte) ([Levels]int, error) {
	var w [Levels]int
	const table = 0x1BD0
	if len(all) < table+2*len(w) {
		return w, errors.New("LEVELDAT too short for the level-order table")
	}
	for i := range w {
		w[i] = int(binary.BigEndian.Uint16(all[table+2*i:]))
	}
	return w, nil
}

// PlayIndex returns the play-order index (0 = Fun 1) of level record rec: the position of 2·rec + 1
// among words 0-119 of LEVELDAT's level-order table.
func PlayIndex(d *disc.Disc, rec int) (int, error) {
	dat, err := d.Unpack(levelDir + "LEVELDAT")
	if err != nil {
		return 0, err
	}
	w, err := playOrder(dat)
	if err != nil {
		return 0, err
	}
	for i, v := range w[:120] {
		if v == 2*rec+1 {
			return i, nil
		}
	}
	return 0, fmt.Errorf("level record %d not in the level-order table", rec)
}

// LoadPlay reads the level at play-order index n (0 = Fun 1); OddTable levels take their header and
// title from OddTable.
func LoadPlay(d *disc.Disc, n int) (*Level, error) {
	if n < 0 || n >= Levels {
		return nil, fmt.Errorf("play index %d out of range", n)
	}
	dat, err := d.Unpack(levelDir + "LEVELDAT")
	if err != nil {
		return nil, err
	}
	w, err := playOrder(dat)
	if err != nil {
		return nil, err
	}
	v := w[n]
	if v%2 == 1 {
		return load(d, dat, v>>1)
	}
	const path = "/!Lemmings/Lemmings/OddTable"
	odd, ok := d.Files[path]
	if !ok {
		return nil, fmt.Errorf("%s: not on disc", path)
	}
	e := v>>1 - 1
	if e < 0 || 56*(e+1) > len(odd) {
		return nil, fmt.Errorf("play index %d: OddTable entry %d not in the table", n, e)
	}
	l, err := load(d, dat, e)
	if err != nil {
		return nil, err
	}
	r := odd[56*e:][:56]
	u16 := func(o int) int { return int(binary.BigEndian.Uint16(r[o:])) }
	l.ReleaseRate, l.Lemmings, l.ToSave, l.Minutes = u16(0), u16(2), u16(4), u16(6)
	for i := range l.Skills {
		l.Skills[i] = u16(8 + 2*i)
	}
	l.Title = string(r[24:])
	return l, nil
}

// Music returns the tune of the level at play-order index n from MUSINFO: the module (0 LEMMUS0,
// 1 LEMMUS1) and the order position it starts at.
func Music(d *disc.Disc, n int) (module, start int, err error) {
	const path = levelDir + "MUSINFO"
	b, ok := d.Files[path]
	if !ok {
		return 0, 0, fmt.Errorf("%s: not on disc", path)
	}
	if len(b) < 4 {
		return 0, 0, fmt.Errorf("%s: %d bytes", path, len(b))
	}
	count := int(binary.LittleEndian.Uint32(b))
	if count == 0 || len(b) < 4+2*count {
		return 0, 0, fmt.Errorf("%s: %d bytes for %d entries", path, len(b), count)
	}
	e := b[4+2*(n%count):]
	return int(e[0]), int(e[1]), nil
}
