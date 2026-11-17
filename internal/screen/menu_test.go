package screen

import (
	"bytes"
	"fmt"
	"image/color"
	"os"
	"slices"
	"testing"

	"github.com/benemon/openlemm/internal/disc"
	"github.com/benemon/openlemm/internal/level"
)

func testMenu(t *testing.T) (*disc.Disc, *Menu) {
	t.Helper()
	name := os.Getenv("OPENLEMM_DISC")
	if name == "" {
		t.Skip("OPENLEMM_DISC not set")
	}
	img, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	d, err := disc.Read(img)
	if err != nil {
		t.Fatal(err)
	}
	prog, err := d.Unpack("/!Lemmings/Lemmings/LemProg")
	if err != nil {
		t.Fatal(err)
	}
	m, err := LoadMenu(d, prog)
	if err != nil {
		t.Fatal(err)
	}
	return d, m
}

// TestVerdict's oracle is the results screen's verdict rule: first match wins of k = 8 if r = 100;
// 0 if r = 0; 1 if r < q ÷ 2; 2 if r < q − 5; 3 if r < q − 1; 4 if r = q − 1; 5 if r = q; 6 if r <
// q + 20; otherwise 7.
func TestVerdict(t *testing.T) {
	for _, c := range []struct{ r, q, want int }{
		{100, 100, 8}, {100, 50, 8}, {0, 0, 0}, {0, 50, 0},
		{24, 50, 1}, {25, 50, 2}, {44, 50, 2}, {45, 50, 3}, {48, 50, 3}, {49, 50, 4},
		{50, 50, 5}, {51, 50, 6}, {69, 50, 6}, {70, 50, 7}, {99, 10, 7},
		{1, 3, 3}, {1, 4, 1}, {1, 2, 4}, {1, 1, 5},
	} {
		if got := Verdict(c.r, c.q); got != c.want {
			t.Errorf("Verdict(%d, %d) = %d, want %d", c.r, c.q, got, c.want)
		}
	}
}

// literal draws text by the drawing rule: character n of a line
// in the 8×16 cell at x = 8 × (column + n), y = 16 + 16 × row, colour set (row + 1) mod 3, with
// the font from the disc; spaces draw nothing (transparent).
func literal(m *Menu, s *Screen, col, row int, text string) {
	for n, c := range []byte(text) {
		b := m.font[((row+1)%3*91+int(c)-32)*64:]
		for y := range 16 {
			for x := range 8 {
				if v := b[4*y+x/2] >> (4 * (x % 2)) & 15; v != 0 {
					s.Pix[(16+16*row+y)*screenW+8*(col+n)+x] = v
				}
			}
		}
	}
}

// tiled returns a screen with the tile on rows y0 to y1-1, the rule written out: screen row
// y shows tile row (y - 16) mod 64, column x tile column x mod 64.
func tiled(m *Menu, y0, y1 int) *Screen {
	s := newScreen(m.TitlePal)
	t := m.at(tileAddr)
	for y := y0; y < y1; y++ {
		for x := range screenW {
			s.Pix[y*screenW+x] = t[32*((y-16)%64)+x%64/2] >> (4 * (x % 2)) & 15
		}
	}
	return s
}

// TestPreLevelText's oracle is the pre-level screen text, as worded: numbers close up (no field
// padding shows), the percentage is to-save × 100 ÷ lemmings rounded down, "Minute" for 1, the
// rating name padded to 8.
// The expected screens draw each line literally; only rows 32-71 (the preview) are not compared.
func TestPreLevelText(t *testing.T) {
	d, m := testMenu(t)
	for _, n := range []int{0, 29, 30, 92, 119} {
		l, err := level.LoadPlay(d, n)
		if err != nil {
			t.Fatal(err)
		}
		want := tiled(m, 80, 224)
		literal(m, want, 0, 4, fmt.Sprintf("Level %d", n%30+1))
		literal(m, want, 8, 4, l.Title)
		literal(m, want, 10, 6, fmt.Sprintf("Number of Lemmings %d", l.Lemmings))
		literal(m, want, 10, 7, fmt.Sprintf("%d%% To Be Saved", l.ToSave*100/l.Lemmings))
		literal(m, want, 10, 8, fmt.Sprintf("Release Rate %d", l.ReleaseRate))
		unit := " Minutes"
		if l.Minutes == 1 {
			unit = " Minute"
		}
		literal(m, want, 10, 9, fmt.Sprintf("Time    %d%s", l.Minutes, unit))
		// The game's rating names each start with a space, not only Fun's.
		literal(m, want, 10, 10, "Rating "+[]string{" Fun", " Tricky", " Taxing", " Mayhem"}[n/30])
		literal(m, want, 5, 12, "Press mouse button to continue")
		got := m.PreLevel(l, n)
		clear(got.Pix[32*screenW : 72*screenW])
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Errorf("level %d (%d minutes): text or tile differs", n, l.Minutes)
			for i := range got.Pix {
				if got.Pix[i] != want.Pix[i] {
					t.Logf("first difference at (%d, %d)", i%screenW, i/screenW)
					break
				}
			}
		}
		if got.Split < 72 || got.Split > 80 || !slices.Equal(got.Lower, m.TitlePal) || got.Upper[0] != color.Black {
			t.Errorf("level %d: split at row %d, lower palette title %v, upper colour 0 %v", n, got.Split,
				slices.Equal(got.Lower, m.TitlePal), got.Upper[0])
		}
	}
}

// TestPreview's oracle is the pre-level preview rule: preview pixel (i, j) is level x 5i − 16,
// level row 4j of the level with its objects drawn, trapdoors half open and every other object at
// its first frame; rows 72-79 are colour 0. level.Image holds the level with every object at frame
// 0 (checked against the running game), so the two agree everywhere except within a trapdoor.
func TestPreview(t *testing.T) {
	d, m := testMenu(t)
	for _, n := range []int{0, 21, 77} {
		l, err := level.LoadPlay(d, n)
		if err != nil {
			t.Fatal(err)
		}
		s := m.PreLevel(l, n)
		differ := 0
		for j := range 40 {
			for i := range screenW {
				x, y := 5*i, 4*j // map x, level row
				if s.Pix[(32+j)*screenW+i] == l.Image.Pix[y*l.Image.Stride+x] {
					continue
				}
				differ++
				in := false
				for _, o := range l.Objects {
					in = in || o.Entrance && x >= o.X && x < o.X+64 && y >= o.Y && y < o.Y+32
				}
				if !in {
					t.Errorf("level %d: preview (%d, %d) differs from the level outside any trapdoor", n, i, j)
				}
			}
		}
		if differ == 0 {
			t.Errorf("level %d: trapdoors show their first frame", n)
		}
		for y := 72; y < 80; y++ {
			if !slices.Equal(s.Pix[y*screenW:(y+1)*screenW], make([]uint8, screenW)) {
				t.Errorf("level %d: row %d not colour 0", n, y)
			}
		}
	}
}

// TestResultsText's oracle is the results screen rule: the header by how the level ended, the two
// percentages closed up, then for a pass the next level's place and code and "Press mouse button to
// continue", for a fail the two button lines, and after the last level no verdict and no code. The
// verdict lines and the code come from the disc.
func TestResultsText(t *testing.T) {
	_, m := testMenu(t)
	for _, c := range []struct {
		r    Results
		name string
	}{
		{Results{TimeUp: true, Needed: 50, Rescued: 7, Next: 31}, "fail"},
		{Results{Needed: 5, Rescued: 100, Next: 30}, "pass"},
		{Results{Needed: 100, Rescued: 100, Next: 120}, "last"},
	} {
		want := tiled(m, 16, 224)
		if c.r.TimeUp {
			literal(m, want, 12, 0, "Your time is up!")
		} else {
			literal(m, want, 6, 0, "All lemmings accounted for.")
		}
		literal(m, want, 12, 2, fmt.Sprintf("You needed  %d%%", c.r.Needed))
		literal(m, want, 12, 3, fmt.Sprintf("You rescued %d%%", c.r.Rescued))
		switch c.name {
		case "fail":
			literal(m, want, 5, 11, "Press left button to try again")
			literal(m, want, 5, 12, "Press right button for menu")
		case "pass":
			literal(m, want, 6, 8, fmt.Sprintf("Your access code for Level %d", c.r.Next%30+1))
			literal(m, want, 13, 9, "is "+m.Code(c.r.Next))
			literal(m, want, 5, 12, "Press mouse button to continue")
		case "last":
			literal(m, want, 5, 12, "Press mouse button to continue")
		}
		if c.name != "last" {
			for _, r := range m.texts[verdictAddr+87*Verdict(c.r.Rescued, c.r.Needed)] {
				literal(m, want, r.col, r.row, string(r.text))
			}
		}
		if got := m.ResultsScreen(c.r); !bytes.Equal(got.Pix, want.Pix) {
			t.Errorf("%s: screen differs", c.name)
		}
	}
}

// TestCodes's oracle is the level-code rule: rows 1-121 are all different, the first exact match
// selects the row, a partly filled field or one of dashes never matches.
func TestCodes(t *testing.T) {
	_, m := testMenu(t)
	for r := 1; r < 122; r++ {
		if got := m.Match([10]byte([]byte(m.Code(r)))); got != r {
			t.Errorf("code of row %d matches row %d", r, got)
		}
		part := [10]byte([]byte(m.Code(r)))
		part[9] = '-'
		if got := m.Match(part); got != 0 {
			t.Errorf("row %d with its last place empty matches row %d", r, got)
		}
	}
	if got := m.Match([10]byte([]byte("----------"))); got != 0 {
		t.Errorf("empty field matches row %d", got)
	}
}

// TestAccessCodeScreen's oracle is the access-code screen layout: the prompt, the field opaque at
// column 14 row 7, the cursor a '-' at column 14 + p drawn opaquely 6 pixels above row 8 under the
// field; after a match "Code for level n" and the rating, then "Press mouse button to continue"; no
// pointer.
func TestAccessCodeScreen(t *testing.T) {
	_, m := testMenu(t)
	field := [10]byte([]byte("ABC-------"))
	want := tiled(m, 16, 224)
	literal(m, want, 8, 2, "Enter the access code")
	for y := 138; y < 154; y++ {
		clear(want.Pix[y*screenW+112 : y*screenW+192])
	}
	cur := newScreen(m.TitlePal)
	literal(m, cur, 14+3, 8, "-")
	for y := 138; y < 154; y++ {
		copy(want.Pix[y*screenW+136:y*screenW+144], cur.Pix[(y+6)*screenW+136:])
	}
	for y := 128; y < 144; y++ {
		clear(want.Pix[y*screenW+112 : y*screenW+192])
	}
	literal(m, want, 14, 7, string(field[:]))
	if got := m.AccessCode(field, 3, Typing, 0); !bytes.Equal(got.Pix, want.Pix) {
		t.Error("typing: screen differs")
		for i := range got.Pix {
			if got.Pix[i] != want.Pix[i] {
				t.Logf("first difference at (%d, %d)", i%screenW, i/screenW)
				break
			}
		}
	}

	got := m.AccessCode(field, 3, Found, 37)
	for y := 144; y < 154; y++ {
		if !slices.Equal(got.Pix[y*screenW+112:y*screenW+192], make([]uint8, 80)) {
			t.Errorf("found: cursor row %d not blanked", y)
		}
	}
	res := tiled(m, 16, 224)
	literal(m, res, 10, 9, "Code for level 8")
	literal(m, res, 5, 12, "Press mouse button to continue")
	for _, y := range []int{16 + 16*9 + 4, 16 + 16*12 + 4} {
		for x := range screenW {
			if w := res.Pix[y*screenW+x]; w != tiled(m, 16, 224).Pix[y*screenW+x] && got.Pix[y*screenW+x] != w {
				t.Fatalf("found: text pixel (%d, %d) is %d, want %d", x, y, got.Pix[y*screenW+x], w)
			}
		}
	}
}
