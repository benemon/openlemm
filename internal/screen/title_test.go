package screen

import (
	"bytes"
	"slices"
	"testing"
)

// showsFace reports whether the screen shows frame f of face image k at (x, y).
func showsFace(m *Menu, s *Screen, k, f, x, y int) bool {
	want := newScreen(m.TitlePal)
	image4(want, m.at(facesAddr+512*k+128*f), 16, 16, 0, x, y, true)
	for yy := y; yy < y+16; yy++ {
		if !bytes.Equal(s.Pix[yy*screenW+x:][:16], want.Pix[yy*screenW+x:][:16]) {
			return false
		}
	}
	return true
}

// TestFaces's oracle is the title menu's face animation: the faces blink in turn in the order 4, 1,
// 6, 0, 5, 3, 2, 7 at their listed places; each shows frames 1, 2, 3 and 0 for 4 frames each, the
// very first face 3 frames of frame 1, while every other face shows frame 0: at display frame n
// after opening, face n div 16 mod 8 shows frame ((n mod 16) div 4 + 1) mod 4.
func TestFaces(t *testing.T) {
	_, m := testMenu(t)
	order := [8]struct{ k, x, y int }{
		{4, 88, 144}, {1, 128, 64}, {6, 216, 136}, {0, 16, 64},
		{5, 152, 144}, {3, 24, 144}, {2, 248, 56}, {7, 280, 136},
	}
	tl := m.NewTitle()
	for n := 1; n <= 16*8+20; n++ {
		m.Step(tl)
		s := m.TitleScreen(tl, 0, false)
		for i, o := range order {
			f := 0
			if i == n/16%8 {
				f = (n%16/4 + 1) % 4
			}
			if !showsFace(m, s, o.k, f, o.x, o.y) {
				t.Fatalf("display frame %d: face %d does not show frame %d", n, i, f)
			}
		}
	}
	tl = m.NewTitle()
	for n := 1; n <= 4; n++ {
		m.Step(tl)
		if want := []int{1, 1, 1, 2}[n-1]; !showsFace(m, m.TitleScreen(tl, 0, false), 4, want, 88, 144) {
			t.Errorf("display frame %d: first face not at frame %d", n, want)
		}
	}
}

// TestScroller's oracle is the title menu's scroller: each 32-character message scrolls in over 128
// frames and stands complete for 101 (the frame that takes the message end, then a pause of 100 in
// which nothing moves, the rollers included). The paper's texture repeats every 4 pixels, so every
// 2-pixel move shows.
func TestScroller(t *testing.T) {
	_, m := testMenu(t)
	tl := m.NewTitle()
	part := func(x0, x1 int) []uint8 {
		s := m.TitleScreen(tl, 0, false)
		var b []uint8
		for y := stripTop; y < stripTop+stripH; y++ {
			b = append(b, s.Pix[y*screenW+x0:y*screenW+x1]...)
		}
		return b
	}
	strip := func() []uint8 { return part(32, 288) }
	rollers := func() []uint8 { return append(part(0, 32), part(288, 320)...) }
	var runs []int
	moving, n := true, 0
	prevS, prevR := strip(), rollers()
	for range 2 * (128 + 101) {
		m.Step(tl)
		s, r := strip(), rollers()
		mv := !bytes.Equal(s, prevS)
		if mv != moving {
			runs = append(runs, n)
			moving, n = mv, 0
		}
		if !mv && n > 0 && !bytes.Equal(r, prevR) {
			t.Fatalf("rollers turned in the pause, %d frames into it", n)
		}
		n++
		prevS, prevR = s, r
	}
	runs = append(runs, n)
	if !slices.Equal(runs, []int{128, 101, 128, 101}) {
		t.Errorf("moving and standing runs %v, want [128 101 128 101]", runs)
	}
}

// TestSigns's oracle is the title menu's signs: the music sign at (192, 152) shows the note in
// music mode and "FX" in effects mode, the rating sign at (256, 152) the current rating, both 64×32
// and opaque.
func TestSigns(t *testing.T) {
	_, m := testMenu(t)
	tl := m.NewTitle()
	for r := range 4 {
		for mode, fx := range []bool{false, true} {
			s := m.TitleScreen(tl, r, fx)
			for y := range 32 {
				for x := range 64 {
					note := m.at(musicSigns + 1024*mode)[32*y+x/2] >> (4 * (x % 2)) & 15
					sign := m.at(ratingSigns + 1024*r)[32*y+x/2] >> (4 * (x % 2)) & 15
					if s.Pix[(152+y)*screenW+192+x] != note || s.Pix[(152+y)*screenW+256+x] != sign {
						t.Fatalf("rating %d, effects %v: sign pixel (%d, %d) differs", r, fx, x, y)
					}
				}
			}
		}
	}
}

// TestTitleOpening's oracle is the title menu's opening: before the first animated frame the strip
// is the bare paper (the paper image's 32 columns repeated) and x 0-31 and 288-319 show the picture
// on rows 208-215 and colour 0 below; the rollers appear with the first animated frame, which takes
// the first character.
func TestTitleOpening(t *testing.T) {
	_, m := testMenu(t)
	tl := m.NewTitle()
	s := m.TitleScreen(tl, 0, false)
	paper := m.scroller[16*512:]
	for y := range stripH {
		Y := stripTop + y
		for x := range screenW {
			var want uint8
			switch {
			case x >= 32 && x < 288:
				want = paper[(8+y)*16+(x-32)%32/2] >> (4 * (x % 2)) & 15
			case Y < 216:
				want = m.title[(Y-16)*screenW+x]
			}
			if got := s.Pix[Y*screenW+x]; got != want {
				t.Fatalf("before the first animated frame (%d, %d) is %d, want %d", x, Y, got, want)
			}
		}
	}
	m.Step(tl)
	s = m.TitleScreen(tl, 0, false)
	want := newScreen(m.TitlePal)
	image4(want, m.scroller, 32, 32, 8, 0, stripTop, true)
	for y := stripTop; y < stripTop+stripH; y++ {
		if !bytes.Equal(s.Pix[y*screenW:][:32], want.Pix[y*screenW:][:32]) {
			t.Fatalf("first animated frame: left roller row %d not drawn", y)
		}
	}
}
