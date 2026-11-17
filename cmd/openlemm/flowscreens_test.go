package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/benemon/openlemm/internal/level"
	"github.com/benemon/openlemm/internal/screen"
)

// flowFixture is the part of a flow/<name>.json fixture the screens and transitions need.
type flowFixture struct {
	Grab, Kind, PC, Border string
	Transition             struct {
		Previous string
		Inputs   []string
		Writes   []struct {
			Addr  string
			Value int
		}
		Via string
	}
	State struct {
		Splash    struct{ File string }
		Level     int
		Rating    int
		MusicMode string `json:"music_mode"`
		Cheat     bool
		Show      bool
		Title     struct {
			RatingSign int `json:"rating_sign_image"`
			MusicSign  int `json:"music_sign_image"`
			Faces      struct{ Face, Counter int }
			Scroller   struct {
				Frames int `json:"frames_since_open"`
			}
		}
		Prelevel struct {
			Label int
			Title string
		}
		Results struct {
			Outcome string
			Played  int `json:"level_played"`
			Header  string
			Rescued int `json:"rescued_pct"`
			Needed  int `json:"needed_pct"`
			Verdict *int
			Code    *struct{ Row, Label int }
		}
		Code struct {
			Stage string
			Field string
			Place int `json:"cursor_place"`
			Match int
		}
	}
	Pointer struct {
		Shown   bool
		Shape   string
		TopLeft [2]int `json:"top_left"`
		Box     *[4]int
		MouseX  int `json:"mouse_x"`
		MouseY  int `json:"mouse_y"`
	}
	Palettes struct {
		Upper, Lower []string
	}
	SHA256 string `json:"sha256"`
	Masked string `json:"sha256_pointer_masked"`
}

func loadFlowFixtures(t *testing.T) map[string]*flowFixture {
	t.Helper()
	dir := os.Getenv("OPENLEMM_FIXTURES")
	if dir == "" {
		t.Skip("OPENLEMM_FIXTURES not set")
	}
	names, err := filepath.Glob(filepath.Join(dir, "flow", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	fx := map[string]*flowFixture{}
	for _, n := range names {
		b, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		var f flowFixture
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		fx[f.Grab] = &f
	}
	return fx
}

// flowHash is the SHA-256 of s as RGB, rows above s.Split in s.Upper and the rest in s.Lower,
// with the pixels inside box black.
func flowHash(s *screen.Screen, box image.Rectangle) string {
	h := sha256.New()
	for i, v := range s.Pix {
		p := s.Upper
		if i >= s.Split*screenW {
			p = s.Lower
		}
		c := p[v]
		if (image.Point{i % screenW, i / screenW}).In(box) {
			c = color.Black
		}
		r, g, b, _ := c.RGBA()
		h.Write([]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8)})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// splashFiles names the splash files on the disc in the order they are shown.
var splashFiles = []string{"Krisalis", "Psygno", "DmaLogo"}

// fixtureScreen composes the screen a fixture's state describes with the screen package, pointer
// included.
func fixtureScreen(t *testing.T, a *assets, f *flowFixture) *screen.Screen {
	m := a.menu
	st := &f.State
	var s *screen.Screen
	switch f.Kind {
	case "prelevel":
		l, err := level.LoadPlay(a.disc, st.Level)
		if err != nil {
			t.Fatalf("%s: %v", f.Grab, err)
		}
		if l.Title != st.Prelevel.Title {
			t.Errorf("%s: title %q, fixture %q", f.Grab, l.Title, st.Prelevel.Title)
		}
		s = m.PreLevel(l, st.Level)
	case "results":
		r := st.Results
		res := screen.Results{TimeUp: r.Header == "time-up", Needed: r.Needed, Rescued: r.Rescued, Next: r.Played + 1}
		if res.Passed() != (r.Outcome != "fail") || (r.Outcome == "final") != (res.Passed() && r.Played == 119) {
			t.Errorf("%s: outcome %s, engine passed %v", f.Grab, r.Outcome, res.Passed())
		}
		if r.Verdict != nil && *r.Verdict != screen.Verdict(r.Rescued, r.Needed) {
			t.Errorf("%s: verdict %d, engine %d", f.Grab, *r.Verdict, screen.Verdict(r.Rescued, r.Needed))
		}
		s = m.ResultsScreen(res)
	case "title":
		tl := m.NewTitle()
		for range st.Title.Scroller.Frames {
			m.Step(tl)
		}
		s = m.TitleScreen(tl, st.Title.RatingSign, st.Title.MusicSign == 1)
	case "code":
		res := screen.Typing
		if st.Code.Stage == "result" {
			switch r := st.Code.Match; {
			case r < 0:
				res = screen.Incorrect
			case r < 120:
				res = screen.Found
			case r == 120:
				res = screen.ShowMode
			default:
				res = screen.CheatMode
			}
		}
		s = m.AccessCode([10]byte([]byte(st.Code.Field)), st.Code.Place, res, st.Level)
	case "congrats":
		s = m.Congratulations()
	case "splash":
		k := slices.Index(splashFiles, st.Splash.File)
		if k < 0 {
			t.Fatalf("%s: splash file %q", f.Grab, st.Splash.File)
		}
		s = m.Splash(k)
	default:
		t.Fatalf("%s: kind %q", f.Grab, f.Kind)
	}
	if f.Pointer.Shown {
		m.Pointer(s, f.Pointer.TopLeft[0], f.Pointer.TopLeft[1], f.Pointer.Shape == "busy")
	}
	return s
}

// midRedraw reports whether a grab stopped while the frame interrupt was redrawing the pointer,
// so that only its pointer-masked hash describes the pointer state: the fixture's recorded PC
// in &24478-&2454C, or at &2A928 or &2A9C8.
func midRedraw(pc string) bool {
	var a int
	fmt.Sscanf(strings.TrimPrefix(pc, "&"), "%X", &a)
	return a >= 0x24478 && a <= 0x2454C || a == 0x2A928 || a == 0x2A9C8
}

// flowScreenCounts is the number of flow fixtures of each kind TestFlowScreens composes: the
// transition sweep's 511 grabs and the start-up run's three splashes and three title passes.
var flowScreenCounts = map[string]int{
	"prelevel": 250, "results": 241, "title": 16, "code": 6, "congrats": 1, "splash": 3,
}

// TestFlowScreens composes every flow fixture's screen from its recorded state and compares its
// hashes with the running game's (OPENLEMM_FIXTURES/flow), and its palettes with the displayed
// ones.
func TestFlowScreens(t *testing.T) {
	a, err := loadAssets(testDisc(t))
	if err != nil {
		t.Fatal(err)
	}
	fx := loadFlowFixtures(t)
	type count struct{ n, masked, full, fullTried int }
	counts := map[string]*count{}
	var names []string
	for n, f := range fx {
		if _, ok := flowScreenCounts[f.Kind]; ok {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		f := fx[n]
		c := counts[f.Kind]
		if c == nil {
			c = &count{}
			counts[f.Kind] = c
		}
		c.n++
		s := fixtureScreen(t, a, f)
		if got := strings.Join(paletteHex(s.Upper), " "); got != strings.Join(f.Palettes.Upper, " ") {
			t.Errorf("%s: upper palette %s, fixture %v", n, got, f.Palettes.Upper)
		}
		if got := strings.Join(paletteHex(s.Lower), " "); got != strings.Join(f.Palettes.Lower, " ") {
			t.Errorf("%s: lower palette %s, fixture %v", n, got, f.Palettes.Lower)
		}
		var box image.Rectangle
		if b := f.Pointer.Box; b != nil {
			box = image.Rect(b[0], b[1], b[2], b[3])
		}
		if flowHash(s, box) == f.Masked {
			c.masked++
		} else {
			t.Errorf("%s: pointer-masked hash differs", n)
		}
		if midRedraw(f.PC) {
			continue
		}
		c.fullTried++
		if flowHash(s, image.Rectangle{}) == f.SHA256 {
			c.full++
		} else {
			t.Errorf("%s: full hash differs (PC %s)", n, f.PC)
		}
	}
	total := 0
	for _, k := range []string{"prelevel", "results", "title", "code", "congrats", "splash"} {
		c := counts[k]
		if c == nil {
			c = &count{}
		}
		if c.n != flowScreenCounts[k] {
			t.Errorf("%d %s grabs, want %d", c.n, k, flowScreenCounts[k])
		}
		total += c.masked
		t.Logf("%-8s %3d grabs: pointer-masked %3d, full %3d of %3d not caught mid pointer redraw", k, c.n, c.masked, c.full, c.fullTried)
	}
	t.Logf("pointer-masked %d/%d", total, len(names))
	if c := counts["prelevel"]; c != nil && c.n-c.fullTried != 34 {
		t.Errorf("%d pre-level grabs caught mid pointer redraw, README says 34", c.n-c.fullTried)
	}
}
