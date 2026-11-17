package sim

import (
	"image"
	"testing"

	"github.com/benemon/openlemm/internal/level"
)

// TestClock checks the level clock and the end of a level: one tick per display frame, 3 per
// update, from N minutes, 0 seconds and 49 ticks; N:00 shows for 49 frames; the time-up flag is set
// on frame 3000 × N + 50; the level is left 16 updates after the one in which the flag is first
// seen. Its oracle is that rule.
func TestClock(t *testing.T) {
	g := New(&level.Level{Map: image.NewPaletted(image.Rect(0, 0, mapW, mapH), nil), Minutes: 2}, &Tables{})
	for k := 1; k <= 2017+16; k++ {
		g.Step()
		frames := 3 * k
		m, s := g.Clock()
		switch {
		case frames < 50 && (m != 2 || s != 0):
			t.Fatalf("frame %d: %d:%02d, want 2:00", frames, m, s)
		case frames >= 50 && frames < 100 && (m != 1 || s != 59):
			t.Fatalf("frame %d: %d:%02d, want 1:59", frames, m, s)
		}
		if want := frames >= 6050; g.TimeUp != want {
			t.Fatalf("frame %d: time up %v, want %v", frames, g.TimeUp, want)
		}
		// The flag is first seen in step 2017 (frames 6049-6051).
		if want := k >= 2017+16; g.Over != want {
			t.Fatalf("step %d: over %v, want %v", k, g.Over, want)
		}
	}
}

// TestEscape checks that the Escape key's time-up starts the same 16-update ending.
func TestEscape(t *testing.T) {
	g := New(&level.Level{Map: image.NewPaletted(image.Rect(0, 0, mapW, mapH), nil), Minutes: 5}, &Tables{})
	g.Step()
	g.TimeUp = true
	for range 16 {
		g.Step()
	}
	if g.Over {
		t.Errorf("over 15 updates after the flag was first seen")
	}
	g.Step()
	if !g.Over {
		t.Errorf("not over 16 updates after the flag was first seen")
	}
}
