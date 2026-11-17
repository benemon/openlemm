package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"

	"github.com/benemon/openlemm/internal/disc"
	"github.com/benemon/openlemm/internal/level"
	"github.com/benemon/openlemm/internal/screen"
	"github.com/benemon/openlemm/internal/sim"
)

// art is what composing a screen reads from the disc.
type art struct {
	sprites []byte // LemGrafP
	prog    []byte // LemProg
	panel   *screen.Panel
}

func loadArt(d *disc.Disc, prog []byte) (*art, error) {
	p, err := screen.Load(d, prog)
	if err != nil {
		return nil, err
	}
	sprites, err := d.Unpack("/!Lemmings/Lemmings/LemGrafP")
	if err != nil {
		return nil, err
	}
	if len(sprites) != 0x7C00 {
		return nil, fmt.Errorf("LemGrafP: %d bytes, want %d", len(sprites), 0x7C00)
	}
	if len(prog) < crossAddr+maskOff+128 {
		return nil, errors.New("LemProg: too short for the pointer images")
	}
	return &art{sprites, prog, p}, nil
}

// scene is the state one screen shows.
type scene struct {
	lvl      *level.Level
	terrain  []uint8 // the map, 1632 × 176 map rows
	frames   []int   // the frame shown of each of lvl.Objects
	lemmings []sim.Lemming
	panel    screen.State
	scroll   int         // map x of screen column 0
	pointer  image.Point // screen position of the pointer's centre
	brackets bool        // the pointer shape when a lemming is under it
	nuked    bool        // the nuke has been triggered: explosions have 40 particles
}

// compose returns the screen showing s as 320 × 256 colour indices, rows 0-183 in the level
// palette lp and rows 184-255 in the panel palette pp.
func (a *art) compose(s *scene) (pix []uint8, lp, pp color.Palette, err error) {
	lp = append(color.Palette{color.Black}, s.lvl.Map.Palette[1:]...)
	view := image.NewPaletted(image.Rect(0, 0, mapW, 160), lp)
	copy(view.Pix, s.terrain[4*mapW:])
	// Objects are clipped at level row 158, one row short of the view.
	objView := view.SubImage(image.Rect(0, 0, mapW, viewRows-1)).(*image.Paletted)
	for i := range s.lvl.Objects {
		s.lvl.Objects[i].Draw(objView, s.frames[i])
	}
	for i := range s.lemmings {
		if l := &s.lemmings[i]; !l.Removed {
			a.drawLemming(view, l, s.nuked)
		}
	}

	pix = make([]uint8, screenW*screenH)
	for y := range viewRows {
		copy(pix[(viewTop+y)*screenW:][:screenW], view.Pix[y*mapW+s.scroll:])
	}
	p, err := a.panel.Draw(&level.Level{Map: &image.Paletted{Pix: s.terrain, Stride: mapW, Rect: image.Rect(0, 0, mapW, 176)}}, s.panel)
	if err != nil {
		return nil, nil, nil, err
	}
	for y := range p.Rect.Dy() {
		copy(pix[(panelTop+y)*screenW:][:screenW], p.Pix[y*p.Stride:])
	}

	ptr := crossAddr
	if s.brackets {
		ptr = bracketsAddr
	}
	for y := range 16 {
		for x := range 16 {
			X, Y := s.pointer.X-hotspot+x, s.pointer.Y-hotspot+y
			i := ptr + y*8 + x/2
			if X < 0 || X >= screenW || Y < 0 || Y >= screenH || a.prog[i+maskOff]>>(4*(x%2))&15 == 0 {
				continue
			}
			pix[Y*screenW+X] = a.prog[i] >> (4 * (x % 2)) & 15
		}
	}
	return pix, lp, p.Palette, nil
}

func (a *art) drawLemming(view *image.Paletted, l *sim.Lemming, nuked bool) {
	s := spriteFor(l)
	if l.State&sim.Exploding != 0 && l.Frame > 0 {
		a.drawParticles(view, l, nuked)
		return
	}
	if l.Frame >= s.frames {
		return
	}
	base := s.off + l.Frame*s.size
	if l.Dir < 0 && s.left != 0 {
		base += s.left
	}
	for y := range s.size / (s.w / 2) {
		// Level row = map row - 4. Lemmings never draw on level rows 158 and 159.
		Y := l.Y + s.oy + y - 4
		if Y < 0 || Y >= viewRows-2 {
			continue
		}
		for x := range s.w {
			X := l.X + s.ox + x
			v := a.sprites[base+y*s.w/2+x/2] >> (4 * (x % 2)) & 15
			if v != 0 && X >= 0 && X < mapW {
				view.Pix[Y*mapW+X] = v
			}
		}
	}
	if d := fuseDigit(l.Fuse()); d > 0 {
		glyph := a.prog[fuseAddr+10*(d-1):]
		for y := range 5 {
			Y := l.Y + s.oy - 6 + y - 4
			if Y < 0 || Y >= viewRows-1 {
				continue
			}
			bits := int(glyph[2*y+1])<<8 | int(glyph[2*y]) // a little-endian word, bit 15 leftmost
			for x := range 16 {
				if X := l.X + s.ox + x; bits&(0x8000>>x) != 0 && X >= 0 && X < mapW {
					view.Pix[Y*mapW+X] = fuseColour
				}
			}
		}
	}
}

// drawParticles draws exploding frame f (1-51) as particle frame f - 1, around map (x, y + 4):
// its 80 particles, or the first 40 once the nuke has been triggered.
func (a *art) drawParticles(view *image.Paletted, l *sim.Lemming, nuked bool) {
	pairs := a.prog[particleAddr+160*(l.Frame-1):][:160]
	n := 80
	if nuked {
		n = 40
	}
	for k := range n {
		if pairs[2*k] == 0x80 {
			continue
		}
		// Map row y + 4 + dy is level row y + dy.
		X, Y := l.X+int(int8(pairs[2*k])), l.Y+int(int8(pairs[2*k+1]))
		if X >= 0 && X < mapW && Y >= 0 && Y < viewRows {
			particle(view.Pix[Y*mapW:][:mapW], X, n-1-k)
		}
	}
}

// particle combines value c with the pixel at x of row and disturbs its neighbour in the same
// byte, as the game writes a particle into its 4bpp screen.
func particle(row []uint8, x, c int) {
	row[x] |= uint8(c & 15)
	if x%2 == 0 {
		row[x+1] = uint8(c >> 4)
	} else {
		row[x-1] = 0
	}
}
