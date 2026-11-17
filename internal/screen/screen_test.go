package screen

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"os"
	"slices"
	"testing"

	"github.com/benemon/openlemm/internal/disc"
	"github.com/benemon/openlemm/internal/level"
)

// SHA-256 of screen rows 184-223 as RGB (row-major, 3 bytes per pixel, each 4-bit channel × 17,
// panel palette as displayed), from Arculator grabs of the running game on Fun 1 (record 73).
var frames = []struct {
	name string
	s    State
	sha  string
}{
	{"4-50", State{
		Scroll: 640, Buttons: [10]int{50, 50, 0, 0, 0, 0, 0, 0, 0, 10}, Skill: 7,
		InPlay: 3, Saved: 0, Minutes: 4, Seconds: 50,
		Lemmings: []image.Point{{842, 84}, {818, 84}, {765, 79}},
	}, "caee596377900c96ad9601b6998a0f05f479845bef97ab1aea60b962f05bf99d"},
	{"4-42", State{
		Scroll: 640, Buttons: [10]int{50, 50, 0, 0, 0, 0, 0, 0, 0, 9}, Skill: 7,
		InPlay: 5, Saved: 0, Minutes: 4, Seconds: 42,
		Lemmings: []image.Point{{844, 102}, {839, 102}, {844, 102}, {835, 82}, {782, 79}},
	}, "62be04c40aca213ab53a5aa1aee8c7581ac58dc991af30ab7509dc08bdca42de"},
}

func TestFun1Panel(t *testing.T) {
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
	l, err := level.Load(d, 73)
	if err != nil {
		t.Fatal(err)
	}
	prog, err := d.Unpack("/!Lemmings/Lemmings/LemProg")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Load(d, prog)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range frames {
		got, err := p.Draw(l, f.s)
		if err != nil {
			t.Errorf("%s: %v", f.name, err)
			continue
		}
		rgb := make([]byte, 0, len(got.Pix)*3)
		for _, v := range got.Pix {
			r, g, b, _ := got.Palette[v].RGBA()
			rgb = append(rgb, byte(r>>8), byte(g>>8), byte(b>>8))
		}
		s := sha256.Sum256(rgb)
		if h := hex.EncodeToString(s[:]); h != f.sha {
			t.Errorf("%s: panel %s, want %s", f.name, h, f.sha)
		}
	}
}

// TestLayout's oracle is the notice's wording and layout: wrapped at word boundaries to
// 40 columns of 8 pixels, each line centred, a blank line between paragraphs, the whole
// vertically centred on the rows given.
func TestLayout(t *testing.T) {
	got := Layout([]string{
		"OpenLemm is a tribute to Acorn Archimedes Lemmings.",
		"It is not maintained, supported, or endorsed by the original publishers.",
	}, 16, 216)
	want := []Line{
		{40, 84, "OpenLemm is a tribute to Acorn"},
		{80, 100, "Archimedes Lemmings."},
		{160, 116, ""},
		{20, 132, "It is not maintained, supported, or"},
		{16, 148, "endorsed by the original publishers."},
	}
	if !slices.Equal(got, want) {
		t.Errorf("notice layout:\n got %+v\nwant %+v", got, want)
	}
	got = Layout([]string{"abcdefghijklmnopqrs abcdefghijklmnopqrst y"}, 0, 32)
	want = []Line{{0, 0, "abcdefghijklmnopqrs abcdefghijklmnopqrst"}, {156, 16, "y"}}
	if !slices.Equal(got, want) {
		t.Errorf("40-column line:\n got %+v\nwant %+v", got, want)
	}
}
