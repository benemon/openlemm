package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/benemon/openlemm/internal/level"
)

// readFadeoutFixture decodes OPENLEMM_FIXTURES/fadeout/<name>.json into v.
func readFadeoutFixture(t *testing.T, name string, v any) {
	t.Helper()
	dir := os.Getenv("OPENLEMM_FIXTURES")
	if dir == "" {
		t.Skip("OPENLEMM_FIXTURES not set")
	}
	b, err := os.ReadFile(filepath.Join(dir, "fadeout", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

// bootFixture is the part of fadeout/boot.json the splashes need: one record per LemBoot frame,
// the n-th with step counter n + 1, and the hold and black-wait events.
type bootFixture struct {
	Frames []bootFrame
	Events []struct {
		Event, Splash, How string
		Counter            int `json:"frame_counter"`
		Left               int `json:"r8"` // hold frames left when a hold ends
	}
}

type bootFrame struct {
	Fade struct {
		In  int `json:"fade_in"`
		Out int `json:"fade_out"`
	}
	Working, Target []string
}

// TestBoot follows the splashes frame by frame through boot.json: each splash's fade-in and the
// frames after it up to the end of the engine's hold, aligned on the fade-in's start; its fade-out,
// aligned on the hold's end, to the next splash or the black wait; and the black wait's length.
// Frames are numbered by LemBoot's step counter, which the engine's frame counter takes, as fade
// steps fall on even frames. LemBoot shows Krisalis while LemProg loads and keeps the screen black
// while it loads each later splash; those load times are not compared.
func TestBoot(t *testing.T) {
	a, err := loadAssets(testDisc(t))
	if err != nil {
		t.Fatal(err)
	}
	var fx bootFixture
	readFadeoutFixture(t, "boot", &fx)
	var start, end []int
	waitStart, waitN := -1, -1
	for _, e := range fx.Events {
		switch e.Event {
		case "wait-start":
			start = append(start, e.Counter)
		case "wait-end":
			end = append(end, e.Counter)
		case "black-wait-start":
			waitStart = e.Counter
		case "black-wait-compare":
			waitN = e.Counter
		}
	}
	if len(start) != 3 || len(end) != 3 || waitStart < 0 || waitN < 0 {
		t.Fatalf("holds %v-%v, black wait from %d for %d: want 3 holds and the black wait", start, end, waitStart, waitN)
	}
	// compare checks the engine's palette against the record of frame n; the black wait's screen
	// is one black colour.
	compare := func(t *testing.T, f *flow, n int) {
		t.Helper()
		v, err := f.view()
		if err != nil {
			t.Fatal(err)
		}
		got, want := paletteHex(v.scr.Upper), fx.Frames[n-1].Working
		if f.ph == waiting {
			got = slices.Repeat(got, len(want))
		}
		if !slices.Equal(got, want) {
			t.Errorf("frame %d: palette %v, fixture %v", n, got, want)
		}
	}
	for k := range 3 {
		pal := paletteHex(a.menu.Splash(k).Upper)
		in := slices.IndexFunc(fx.Frames, func(r bootFrame) bool {
			return r.Fade.In != 0 && slices.Equal(r.Target, pal)
		})
		if in < 0 {
			t.Fatalf("splash %d: no fade-in to its palette", k)
		}
		t.Run(splashFiles[k]+"/fade-in", func(t *testing.T) {
			f := newFlow(a, nil)
			f.splash, f.frame = k-1, in
			f.enter(splashing)
			for n := in + 1; !f.leaving; n++ {
				run(t, f, input{}, 1)
				compare(t, f, n)
			}
		})
		t.Run(splashFiles[k]+"/hold", func(t *testing.T) {
			t.Skip("the fixture counts the loader's frames, which count two display frames twice in each hold; the hold itself is 240 display frames")
			f := newFlow(a, nil)
			f.splash = k - 1
			f.enter(splashing)
			n := 0
			for ; !f.leaving; n++ {
				run(t, f, input{}, 1)
			}
			if n != end[k]-start[k] {
				t.Errorf("hold %d frames, fixture %d", n, end[k]-start[k])
			}
		})
		t.Run(splashFiles[k]+"/fade-out", func(t *testing.T) {
			f := newFlow(a, nil)
			f.splash = k - 1
			f.enter(splashing)
			for !f.leaving {
				run(t, f, input{}, 1)
			}
			f.frame, f.begin = end[k], end[k]
			n := end[k] + 1
			for ; fx.Frames[n-1].Fade.Out != 0; n++ {
				run(t, f, input{}, 1)
				if f.ph != splashing || f.splash != k {
					t.Fatalf("frame %d: next screen open while the fixture is fading out", n)
				}
				compare(t, f, n)
			}
			run(t, f, input{}, 1)
			if k < 2 && (f.ph != splashing || f.splash != k+1) || k == 2 && f.ph != waiting {
				t.Fatalf("frame %d (fade-out over): phase %d splash %d", n, f.ph, f.splash)
			}
			compare(t, f, n)
			if k < 2 {
				return
			}
			if n != waitStart {
				t.Errorf("black wait from frame %d, fixture %d", n, waitStart)
			}
			if m := until(t, f, input{}, titleMenu, 1000); m != waitN {
				t.Errorf("black wait %d frames, fixture %d", m, waitN)
			}
		})
	}
}

// TestBootSkip replays boot-skip1-3.json, each with the left button pressed during one splash's
// hold: the hold ends on the frame the button is first held, the splash fades out as recorded, and
// the black wait follows it, the later splashes skipped. The black wait's length is not compared:
// the loader counted 101 and 102 frames for it in these runs, counting display frames twice.
func TestBootSkip(t *testing.T) {
	a, err := loadAssets(testDisc(t))
	if err != nil {
		t.Fatal(err)
	}
	for k := range 3 {
		name := splashFiles[k]
		t.Run(name, func(t *testing.T) {
			var fx bootFixture
			readFadeoutFixture(t, fmt.Sprintf("boot-skip%d", k+1), &fx)
			starts, end, held, waitStart := 0, -1, 0, -1
			for _, e := range fx.Events {
				switch e.Event {
				case "wait-start":
					starts++
				case "wait-end":
					if e.How == "button" {
						end, held = e.Counter, 240-e.Left
					}
				case "black-wait-start":
					waitStart = e.Counter
				}
			}
			if starts != k+1 || end < 0 || waitStart < 0 {
				t.Fatalf("%d holds, button end at %d, black wait from %d: want %d holds, the last ended by the button", starts, end, waitStart, k+1)
			}
			f := newFlow(a, nil)
			f.splash = k - 1
			f.enter(splashing)
			run(t, f, input{}, held-1)
			if f.leaving {
				t.Fatalf("hold over after %d frames with no button", held-1)
			}
			run(t, f, input{left: true}, 1)
			if !f.leaving {
				t.Fatalf("hold frame %d: button held, hold not over", held)
			}
			f.frame, f.begin = end, end
			n := end + 1
			for ; fx.Frames[n-1].Fade.Out != 0; n++ {
				run(t, f, input{}, 1)
				if f.ph != splashing || f.splash != k {
					t.Fatalf("frame %d: next screen open while the fixture is fading out", n)
				}
				v, err := f.view()
				if err != nil {
					t.Fatal(err)
				}
				if got, want := paletteHex(v.scr.Upper), fx.Frames[n-1].Working; !slices.Equal(got, want) {
					t.Errorf("frame %d: palette %v, fixture %v", n, got, want)
				}
			}
			run(t, f, input{}, 1)
			if f.ph != waiting {
				t.Fatalf("frame %d (fade-out over): phase %d splash %d, want the black wait", n, f.ph, f.splash)
			}
			if n != waitStart {
				t.Errorf("black wait from frame %d, fixture %d", n, waitStart)
			}
		})
	}
}

// TestTitleDesktop replays title-log.json's RETURN TO DESKTOP click, one frame per record, and
// compares the red-border window and the border colour on every record. Each record is taken at
// the title's click test, after that pass's animation and window count and before its click is
// answered, so the record before the click event still shows the border as before the click.
func TestTitleDesktop(t *testing.T) {
	var fx struct {
		Desktop []struct {
			Event  string
			Window int `json:"desktop_window"`
			Mouse  [3]int
			Border string
		}
	}
	readFadeoutFixture(t, "title-log", &fx)
	f := testFlow(t)
	openTitle(t, f)
	clicked, red := false, 0
	for i, r := range fx.Desktop {
		if r.Event != "" {
			continue
		}
		f.mx, f.my = float64(r.Mouse[0]), float64(r.Mouse[1])
		run(t, f, input{left: r.Mouse[2]&4 != 0}, 1)
		if i+1 < len(fx.Desktop) && fx.Desktop[i+1].Event == "desktop-click" {
			clicked = true
			continue
		}
		v, err := f.view()
		if err != nil {
			t.Fatal(err)
		}
		wantRed := strings.EqualFold(r.Border, "000F")
		if f.border != r.Window || (v.border == color.RGBA{0xFF, 0, 0, 0xFF}) != wantRed {
			t.Errorf("record %d: window %d, border %v; fixture %d, %s", i, f.border, v.border, r.Window, r.Border)
		}
		if wantRed {
			red++
		}
	}
	if !clicked || red == 0 {
		t.Fatalf("click seen %v, %d red records: the fixture's click is not replayed", clicked, red)
	}
}

// titleStartFixture is the part of fadeout/title-start.json the comparisons need.
type titleStartFixture struct {
	Frames []titleStartFrame
}

type titleStartFrame struct {
	Frame   int
	Counter int `json:"frame_counter"`
	Events  []string
	Fade    struct {
		Out int `json:"fade_out"`
	}
	Pal struct {
		Upper, Lower []string
	} `json:"palettes"`
	Pointer struct {
		Kind, Shape string
		MouseX      int `json:"mouse_x"`
		MouseY      int `json:"mouse_y"`
		Box         *[4]int
	}
	SHA256 string `json:"sha256"`
	Masked string `json:"sha256_pointer_masked"`
}

// TestTitleStart follows title-start.json from the START click to the pre-level screen: the
// title's fade-out with the busy pointer, aligned on the frame the title stopped animating, to the
// frame the fade-out ends; and the pre-level screen's fade-in, aligned on the frame the screen
// starts waiting, with both hashes on every frame. The black frames between the two, while the
// original prepares the pre-level screen, are not compared.
func TestTitleStart(t *testing.T) {
	a, err := loadAssets(testDisc(t))
	if err != nil {
		t.Fatal(err)
	}
	var fx titleStartFixture
	readFadeoutFixture(t, "title-start", &fx)
	event := func(name string) int {
		i := slices.IndexFunc(fx.Frames, func(r titleStartFrame) bool {
			return slices.Contains(r.Events, name)
		})
		if i < 0 {
			t.Fatalf("no %s event", name)
		}
		return i
	}
	click, off, wait := event("start-click"), event("animation-off"), event("screen-wait")

	t.Run("load", func(t *testing.T) {
		t.Skip("after START the original keeps the title's faces and scroller running, with the busy pointer, while the level loads")
	})

	t.Run("title fade-out", func(t *testing.T) {
		f := newFlow(a, nil)
		f.mx, f.my = float64(fx.Frames[click].Pointer.MouseX), float64(fx.Frames[click].Pointer.MouseY)
		openTitle(t, f)
		run(t, f, input{left: true}, 1)
		if !f.leaving || f.to != preLevel {
			t.Fatal("START did not start the fade-out to the pre-level screen")
		}
		f.frame, f.begin = fx.Frames[off].Counter, fx.Frames[off].Counter
		i := off + 1
		for ; fx.Frames[i].Fade.Out != 0; i++ {
			run(t, f, input{}, 1)
			g := fx.Frames[i]
			if f.ph != titleMenu {
				t.Fatalf("frame %d: phase %d while the fixture fades the title out", g.Frame, f.ph)
			}
			comparePalettes(t, f, g.Frame, g.Pal.Upper, g.Pal.Lower)
			if show, busy := f.fadePointer(); !show || busy != (g.Pointer.Shape == "busy") {
				t.Errorf("frame %d: pointer shown %v busy %v; fixture %s %s", g.Frame, show, busy, g.Pointer.Kind, g.Pointer.Shape)
			}
		}
		run(t, f, input{}, 1)
		if f.ph != preLevel {
			t.Errorf("frame %d (fade-out over): phase %d", fx.Frames[i].Frame, f.ph)
		}
		comparePalettes(t, f, fx.Frames[i].Frame, fx.Frames[i].Pal.Upper, fx.Frames[i].Pal.Lower)
	})

	t.Run("pre-level fade-in", func(t *testing.T) {
		f := newFlow(a, nil)
		l, err := level.LoadPlay(f.disc, 0)
		if err != nil {
			t.Fatal(err)
		}
		f.lvl = l
		g := fx.Frames[wait]
		f.mx, f.my = float64(g.Pointer.MouseX), float64(g.Pointer.MouseY)
		f.frame = g.Counter
		f.enter(preLevel)
		for _, g := range fx.Frames[wait+1:] {
			run(t, f, input{}, 1)
			v := comparePalettes(t, f, g.Frame, g.Pal.Upper, g.Pal.Lower)
			var box image.Rectangle
			if b := g.Pointer.Box; b != nil {
				box = image.Rect(b[0], b[1], b[2], b[3])
			}
			if flowHash(v.scr, box) != g.Masked {
				t.Errorf("frame %d: pointer-masked hash differs", g.Frame)
			}
			if flowHash(v.scr, image.Rectangle{}) != g.SHA256 {
				t.Errorf("frame %d: full hash differs (pointer %s)", g.Frame, g.Pointer.Kind)
			}
		}
		if f.dark > 0 {
			t.Errorf("pre-level screen not faded in at the fixture's last frame (dark %d)", f.dark)
		}
	})
}
