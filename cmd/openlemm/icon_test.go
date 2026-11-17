package main

import (
	"image"
	"image/color"
	"testing"

	"github.com/benemon/openlemm/internal/level"
	"github.com/benemon/openlemm/internal/sim"
)

// TestIconStates checks the icon's choices are the 16 actions, each its own LemGrafP block, and
// never the explosion.
func TestIconStates(t *testing.T) {
	if len(iconStates) != 16 {
		t.Errorf("%d choices, want 16", len(iconStates))
	}
	blocks := map[int]bool{}
	for _, st := range iconStates {
		s := spriteFor(&sim.Lemming{State: st})
		if st&sim.Exploding != 0 || s.off == 0x1B20 {
			t.Errorf("state %#x: the explosion is a choice", st)
		}
		if blocks[s.off] {
			t.Errorf("state %#x: block %#x already chosen", st, s.off)
		}
		blocks[s.off] = true
	}
}

// TestIconFrame's oracle is the decoded LemGrafP: each action's icon frame is the first frame of
// its block's right-facing set, 4bpp low nibble first, in tileset 0's palette (LEVELDAT words
// 0-15, 0RGB) with colour 0 transparent.
func TestIconFrame(t *testing.T) {
	d := testDisc(t)
	a, err := loadAssets(d)
	if err != nil {
		t.Fatal(err)
	}
	lvl, err := level.Load(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	dat, err := d.Unpack("/!Lemmings/Lemmings/LemLevels/LEVELDAT")
	if err != nil {
		t.Fatal(err)
	}
	pal := lvl.Map.Palette
	for _, st := range iconStates {
		s := spriteFor(&sim.Lemming{State: st})
		f := a.art.iconFrame(st, pal)
		rows := s.size / 8
		if f.Rect.Dx() != 16 || f.Rect.Dy() != rows {
			t.Fatalf("state %#x: %v, want 16 × %d", st, f.Rect, rows)
		}
		for y := range rows {
			for x := range 16 {
				want := a.art.sprites[s.off+y*8+x/2] >> (4 * (x % 2)) & 15
				if got := f.ColorIndexAt(x, y); got != want {
					t.Fatalf("state %#x (%d, %d): colour %d, want %d", st, x, y, got, want)
				}
			}
		}
		if f.Palette[0] != color.Transparent {
			t.Errorf("state %#x: colour 0 is %v, want transparent", st, f.Palette[0])
		}
		for i := 1; i < 16; i++ {
			c := int(dat[2*i])<<8 | int(dat[2*i+1])
			want := color.RGBA{uint8(c >> 8 & 15 * 17), uint8(c >> 4 & 15 * 17), uint8(c & 15 * 17), 255}
			if f.Palette[i] != want {
				t.Errorf("state %#x: colour %d is %v, want %v", st, i, f.Palette[i], want)
			}
		}
	}
}

// TestIcons checks a 16 × 10 frame is centred on a square, 3 rows down, and each pixel becomes a
// k × k block at 16, 32, 48 and 64 pixels.
func TestIcons(t *testing.T) {
	f := image.NewPaletted(image.Rect(5, 7, 21, 17), color.Palette{color.Transparent, color.White})
	for y := range 10 {
		for x := range 16 {
			if (x*3+y*5)%7 < 3 {
				f.SetColorIndex(5+x, 7+y, 1)
			}
		}
	}
	got := icons(f)
	if len(got) != 4 {
		t.Fatalf("%d icons, want 4", len(got))
	}
	for i, img := range got {
		p := img.(*image.Paletted)
		n, k := 16*(i+1), i+1
		if p.Rect != image.Rect(0, 0, n, n) {
			t.Fatalf("icon %d: %v, want %d × %d", i, p.Rect, n, n)
		}
		for y := range n {
			for x := range n {
				var want uint8
				if fy := y/k - 3; fy >= 0 && fy < 10 {
					want = f.ColorIndexAt(5+x/k, 7+fy)
				}
				if got := p.ColorIndexAt(x, y); got != want {
					t.Fatalf("icon %d (%d, %d): colour %d, want %d", i, x, y, got, want)
				}
			}
		}
	}
}
