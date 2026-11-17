package main

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// fadeoutFixture is the part of a fadeout/<name>.json fixture the ending needs.
type fadeoutFixture struct {
	Name, End string
	Play      int
	Saved     int                         `json:"saved_written"`
	Started   struct{ Frame, Update int } `json:"ending_started"`
	Left      struct{ Frame, Update int } `json:"level_left"`
	Frames    []struct {
		Frame   int
		Counter int `json:"frame_counter"`
		Music   struct{ Setting int }
		Pal     struct {
			Upper, Lower []string
		} `json:"palettes"`
		Pointer struct {
			MouseX int `json:"mouse_x"`
			MouseY int `json:"mouse_y"`
			Box    *[4]int
		}
		Masked string `json:"sha256_pointer_masked"`
	}
}

// TestFadeout plays each level ending the fixtures record and compares the shown palettes frame
// by frame from the frame the ending started to the fixture's last frame, the updates the ending
// took, and from the leave on the pointer-masked results screen on every frame. Frames are
// aligned on the ending's start, where the engine's frame counter takes the fixture's: fade steps
// fall on even frames counted from program start, and the
// results screen's fade-in begins 9 frames after the leave and its pointer shows from 12.
func TestFadeout(t *testing.T) {
	dir := os.Getenv("OPENLEMM_FIXTURES")
	if dir == "" {
		t.Skip("OPENLEMM_FIXTURES not set")
	}
	a, err := loadAssets(testDisc(t))
	if err != nil {
		t.Fatal(err)
	}
	// The level-ending runs are the play<n>-* files; the others in fadeout/ are start-up and
	// title records.
	names, err := filepath.Glob(filepath.Join(dir, "fadeout", "play*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 7 {
		t.Fatalf("%d level-ending fixtures, want 7", len(names))
	}
	for _, n := range names {
		b, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		var fx fadeoutFixture
		if err := json.Unmarshal(b, &fx); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		t.Run(fx.Name, func(t *testing.T) { fadeout(t, a, &fx) })
	}
}

func fadeout(t *testing.T, a *assets, fx *fadeoutFixture) {
	first := fx.Frames[0]
	f := newFlow(a, nil)
	f.sess.level, f.sess.volume = fx.Play, first.Music.Setting
	if err := f.startLevel(); err != nil {
		t.Fatal(err)
	}
	until(t, f, input{}, preLevel, 100)
	run(t, f, input{}, 1)
	run(t, f, input{left: true}, 1)
	until(t, f, input{}, playing, 100)
	f.mx, f.my = float64(first.Pointer.MouseX), float64(first.Pointer.MouseY)
	f.play.sim.Saved = fx.Saved
	// Between updates the engine's update counter is one ahead of the game's.
	switch fx.End {
	case "escape":
		for f.play.sim.Update < fx.Started.Update-1 {
			run(t, f, input{}, 1)
		}
		run(t, f, input{escape: true}, 1)
	case "finished":
		for f.play.sim.Update < fx.Started.Update {
			run(t, f, input{}, 1)
		}
		// The engine recounts the finished flag every update, so a written flag would not hold:
		// time-up stands in for it until the ending has started.
		f.play.sim.TimeUp = true
	}
	for n := 0; !f.leaving; n++ {
		if n > 6 {
			t.Fatal("the ending did not start")
		}
		run(t, f, input{}, 1)
	}
	if fx.End == "finished" {
		f.play.sim.TimeUp = false
	}
	start := f.play.sim.Update
	f.frame = fx.Frames[fx.Started.Frame].Counter
	f.begin = f.frame
	last := fx.Left.Frame - fx.Started.Frame
	for i, g := range fx.Frames[fx.Started.Frame : fx.Left.Frame+1] {
		if i > 0 {
			run(t, f, input{}, 1)
		}
		if (f.ph == results) != (i == last) {
			t.Fatalf("frame %d: phase %d; the fixture leaves the level at frame %d", g.Frame, f.ph, fx.Left.Frame)
		}
		comparePalettes(t, f, g.Frame, g.Pal.Upper, g.Pal.Lower)
	}
	if d, want := f.play.sim.Update-start, fx.Left.Update-fx.Started.Update; d != want {
		t.Errorf("left %d updates after the ending started, fixture %d", d, want)
	}
	for _, g := range fx.Frames[fx.Left.Frame+1:] {
		run(t, f, input{}, 1)
		v := comparePalettes(t, f, g.Frame, g.Pal.Upper, g.Pal.Lower)
		box := image.Rectangle{}
		if b := g.Pointer.Box; b != nil {
			box = image.Rect(b[0], b[1], b[2], b[3])
		}
		if flowHash(v.scr, box) != g.Masked {
			t.Errorf("frame %d (leave + %d): pointer-masked hash differs", g.Frame, g.Frame-fx.Left.Frame)
		}
	}
	if f.dark > 0 {
		t.Errorf("results screen not faded in at the fixture's last frame (dark %d)", f.dark)
	}
}

// comparePalettes compares the palettes f shows with a fixture frame's and returns the view.
func comparePalettes(t *testing.T, f *flow, frame int, upper, lower []string) view {
	t.Helper()
	v, err := f.view()
	if err != nil {
		t.Fatal(err)
	}
	up, lo := paletteHex(v.scr.Upper), paletteHex(v.scr.Lower)
	if !slices.Equal(up, upper) || !slices.Equal(lo, lower) {
		t.Errorf("frame %d: brightest %d, upper %v lower %v; fixture %v %v", frame,
			brightest(v.scr.Upper, v.scr.Lower), up, lo, upper, lower)
	}
	return v
}
