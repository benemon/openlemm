package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/benemon/openlemm/internal/disc"
	"github.com/benemon/openlemm/internal/level"
	"github.com/benemon/openlemm/internal/screen"
	"github.com/benemon/openlemm/internal/sim"
)

// screenFixture is the part of a screens/L<nnn>.json fixture a composition needs.
type screenFixture struct {
	Record  int
	Scroll  int
	Objects struct {
		Instances []struct{ Slot, Frame int }
	}
	Lemmings struct {
		Displayed []struct{ X, Y, Dir, Frame, State, Removed int }
	}
	Panel struct {
		Buttons       [10]int
		SelectedSkill int `json:"selected_skill"`
		Status        struct {
			Text                  string
			Out, Minutes, Seconds int
			SavedPercent          int `json:"saved_percent"`
		}
	}
	Pointer struct {
		X, Y  int
		Shape string
		Box   [4]int
	}
	Palettes struct{ Level, Panel []string }
	SHA256   string `json:"sha256"`
	Masked   string `json:"sha256_pointer_masked"`
}

// sceneFor builds the scene a fixture describes, with the level's terrain at level start.
func sceneFor(t *testing.T, d *disc.Disc, f *screenFixture) *scene {
	l, err := level.Load(d, f.Record)
	if err != nil {
		t.Fatal(err)
	}
	s := &scene{lvl: l, terrain: l.Map.Pix, frames: make([]int, len(l.Objects)), scroll: f.Scroll}
	frame := map[int]int{}
	for _, in := range f.Objects.Instances {
		frame[in.Slot] = in.Frame
	}
	if len(frame) != len(l.Objects) {
		t.Fatalf("record %d: %d object instances, the level has %d objects", f.Record, len(frame), len(l.Objects))
	}
	for i, o := range l.Objects {
		fr, ok := frame[o.Slot]
		if !ok {
			t.Fatalf("record %d: no instance for slot %d", f.Record, o.Slot)
		}
		s.frames[i] = fr
	}
	for _, m := range f.Lemmings.Displayed {
		s.lemmings = append(s.lemmings, sim.Lemming{X: m.X, Y: m.Y, Dir: m.Dir, Frame: m.Frame, State: m.State, Removed: m.Removed != 0})
		if m.Removed == 0 {
			s.panel.Lemmings = append(s.panel.Lemmings, image.Pt(m.X, m.Y))
		}
	}
	st := f.Panel.Status
	s.panel.Scroll = f.Scroll
	s.panel.Buttons = f.Panel.Buttons
	s.panel.Skill = f.Panel.SelectedSkill
	if st.Text[:12] != strings.Repeat(":", 12) {
		s.panel.Type = strings.TrimRight(st.Text[:8], ":")
		fmt.Sscan(strings.Trim(st.Text[9:12], ":"), &s.panel.Count)
	}
	s.panel.InPlay, s.panel.Saved = st.Out, st.SavedPercent
	s.panel.Minutes, s.panel.Seconds = st.Minutes, st.Seconds
	// The fixture's pointer is its centre in level coordinates: view column, level row.
	s.pointer = image.Pt(f.Pointer.X, f.Pointer.Y+viewTop)
	// Brackets when a lemming is under the pointer, by the box the game chooses lemmings with.
	px, py := f.Scroll+f.Pointer.X, f.Pointer.Y+4
	for _, l := range s.lemmings {
		if !l.Removed && px >= l.X-5 && px <= l.X+4 && py >= l.Y-11 && py <= l.Y {
			s.brackets = true
		}
	}
	if s.brackets != (f.Pointer.Shape == "brackets") {
		t.Errorf("record %d: brackets %v by the rule, fixture shape %s", f.Record, s.brackets, f.Pointer.Shape)
	}
	return s
}

func paletteHex(p color.Palette) []string {
	var s []string
	for _, c := range p {
		r, g, b, _ := c.RGBA()
		s = append(s, fmt.Sprintf("%X%X%X", r>>12, g>>12, b>>12))
	}
	return s
}

// TestScreens composes each whole-screen fixture's state and compares the SHA-256 of the result
// with the real displayed screen, with the pointer box blacked out and in full. L014 and L034
// were grabbed mid pointer redraw, so only their masked hash is compared.
func TestScreens(t *testing.T) {
	adf, dir := os.Getenv("OPENLEMM_DISC"), os.Getenv("OPENLEMM_FIXTURES")
	if adf == "" || dir == "" {
		t.Skip("OPENLEMM_DISC or OPENLEMM_FIXTURES not set")
	}
	img, err := os.ReadFile(adf)
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
	a, err := loadArt(d, prog)
	if err != nil {
		t.Fatal(err)
	}
	masked, full := 0, 0
	for n := range 120 {
		name := fmt.Sprintf("L%03d", n)
		b, err := os.ReadFile(filepath.Join(dir, "screens", name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var f screenFixture
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		pix, lp, pp, err := a.compose(sceneFor(t, d, &f))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := paletteHex(lp); strings.Join(got, " ") != strings.Join(f.Palettes.Level, " ") {
			t.Errorf("%s: level palette %v, fixture %v", name, got, f.Palettes.Level)
		}
		if got := paletteHex(pp); strings.Join(got, " ") != strings.Join(f.Palettes.Panel, " ") {
			t.Errorf("%s: panel palette %v, fixture %v", name, got, f.Palettes.Panel)
		}
		s := &screen.Screen{Pix: pix, Upper: lp, Lower: pp, Split: panelTop}
		box := image.Rect(f.Pointer.Box[0], f.Pointer.Box[1], f.Pointer.Box[2], f.Pointer.Box[3])
		if flowHash(s, box) == f.Masked {
			masked++
		} else {
			t.Errorf("%s: pointer-masked hash differs", name)
		}
		if name == "L014" || name == "L034" {
			continue
		}
		if flowHash(s, image.Rectangle{}) == f.SHA256 {
			full++
		} else {
			t.Errorf("%s: full hash differs", name)
		}
	}
	t.Logf("pointer-masked %d/120, full %d/118", masked, full)
}
