package main

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/benemon/openlemm/internal/disc"
)

// TestPeriods's oracle is the table on the disc: entries 4-39 are ProTracker's 36 standard periods
// (C-1 856 to B-3 113), entries 0-3 the four below them as decoded from LemProg; a sample plays at
// 3,593,750 ÷ period, 8397 Hz at index 16.
func TestPeriods(t *testing.T) {
	prog, err := testDisc(t).Unpack("/!Lemmings/Lemmings/LemProg")
	if err != nil {
		t.Fatal(err)
	}
	p, err := readPeriods(prog)
	if err != nil {
		t.Fatal(err)
	}
	pt := [36]int{
		856, 808, 762, 720, 678, 640, 604, 570, 538, 508, 480, 453,
		428, 404, 381, 360, 339, 320, 302, 285, 269, 254, 240, 226,
		214, 202, 190, 180, 170, 160, 151, 143, 135, 127, 120, 113,
	}
	if [36]int(p[4:]) != pt || [4]int(p[:4]) != [4]int{1076, 1016, 960, 906} {
		t.Errorf("periods %v", p)
	}
	if r := math.Round(3593750 / float64(p[16])); r != 8397 {
		t.Errorf("rate at index 16: %v, want 8397", r)
	}
}

// TestEffectRule's oracle is the sound effect priority rule: priority 3 for &1D, 2 for &15, &16,
// &18, &1C, 0 for &01 and &14, 1 for the rest; a request of at least the playing effect's priority
// replaces it; a lower one is dropped while that effect plays, and plays once it has ended.
func TestEffectRule(t *testing.T) {
	for s, want := range map[int]int{0x1D: 3, 0x15: 2, 0x16: 2, 0x18: 2, 0x1C: 2, 0x01: 0, 0x14: 0, 0x17: 1, 0x1B: 1, 0x0A: 1} {
		if p := priority(s); p != want {
			t.Errorf("sample %#x: priority %d, want %d", s, p, want)
		}
	}
	var samples [31][]int8
	for i := range samples {
		samples[i] = make([]int8, 100)
	}
	var e effect
	e.request(&samples, 0x18, 8000, 1, 1) // 2
	e.request(&samples, 0x1B, 8000, 1, 1) // 1: dropped
	if e.prio != 2 {
		t.Errorf("lower request replaced the playing effect")
	}
	e.request(&samples, 0x1C, 8000, 1, 1) // 2: replaces
	if e.pos != 0 || e.prio != 2 || &e.data[0] != &samples[0x1C][0] {
		t.Errorf("equal request did not replace")
	}
	for e.playing() {
		e.next()
	}
	e.request(&samples, 0x14, 8000, 1, 1) // 0, after the end: plays
	if e.prio != 0 || !e.playing() {
		t.Errorf("lower request after the end did not play")
	}
}

func testDisc(t *testing.T) *disc.Disc {
	adf := os.Getenv("OPENLEMM_DISC")
	if adf == "" {
		t.Skip("OPENLEMM_DISC not set")
	}
	img, err := os.ReadFile(adf)
	if err != nil {
		t.Fatal(err)
	}
	d, err := disc.Read(img)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestSampdat's oracle is SampdatP's own bytes: the offsets chain and the last sample ends at the
// end of the file; entries 0 and 11-19 are empty; every number the program requests (1, 8, 10,
// 20-30) and the trap samples 2-7 and 9 have data.
func TestSampdat(t *testing.T) {
	b, err := testDisc(t).Unpack("/!Lemmings/Lemmings/SampdatP")
	if err != nil {
		t.Fatal(err)
	}
	s, err := readSamples(b)
	if err != nil {
		t.Fatal(err)
	}
	end := 31 * 16
	for i, d := range s {
		empty := i == 0 || i >= 11 && i <= 19
		if (len(d) == 0) != empty {
			t.Errorf("sample %d: %d bytes", i, len(d))
		}
		if len(d) > 0 {
			if off := int(uintptr(unsafe.Pointer(&d[0])) - uintptr(unsafe.Pointer(&b[0]))); off != end {
				t.Errorf("sample %d at %d, want %d", i, off, end)
			}
			end += len(d)
		}
	}
	if end != len(b) {
		t.Errorf("samples end at %d, file %d bytes", end, len(b))
	}
}

// TestMusic loads each level's module through libopenmpt at its MUSINFO start and checks that the
// first half second after the start is not silent. It skips without libopenmpt.
func TestMusic(t *testing.T) {
	d := testDisc(t)
	if err := loadOpenMPT(); err != nil {
		t.Skip(err)
	}
	prog, err := d.Unpack("/!Lemmings/Lemmings/LemProg")
	if err != nil {
		t.Fatal(err)
	}
	for n := range 2 { // Fun 1 and Fun 2
		m, err := loadSound(d, prog)
		if err != nil {
			t.Fatal(err)
		}
		tr, err := levelTune(d, n)
		if err != nil {
			t.Fatalf("level %d: %v", n, err)
		}
		m.play(tr)
		m.set(true, 255, false)
		p := make([]byte, 4*sampleRate/2)
		m.Read(p)
		loud := 0
		for _, v := range p {
			if v != 0 {
				loud++
			}
		}
		if loud < len(p)/4 {
			t.Errorf("level %d: %d of %d bytes non-zero", n, loud, len(p))
		}
	}
}

// TestEffectLoudness measures libopenmpt's output for one full-volume (64) centred note of
// LEMMUS0's first sample with nearest-sample playback, against the sample's own peak, and checks
// fxGain against it: an effect at volume 255 is exactly as loud as such a note.
func TestEffectLoudness(t *testing.T) {
	d := testDisc(t)
	if err := loadOpenMPT(); err != nil {
		t.Skip(err)
	}
	data := d.Files["/!Lemmings/Lemmings/LemLevels/LEMMUS0"]
	patterns := 0
	for _, o := range data[952 : 952+128] {
		patterns = max(patterns, int(o)+1)
	}
	off := 1084 + 1024*patterns
	peak := 0
	for _, b := range data[off : off+2*int(binary.BigEndian.Uint16(data[20+22:]))] {
		peak = max(peak, int(int8(b)), -int(int8(b)))
	}
	tr, err := newTracker(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	var ia [16]uintptr
	openmpt.iface(tr.ext, "interactive", unsafe.Pointer(&ia), unsafe.Sizeof(ia))
	var play func(ext uintptr, instrument, note int32, volume, panning float64) int32
	purego.RegisterFunc(&play, ia[14]) // play_note
	for ch := range 4 {
		tr.mute(tr.ext, int32(ch), 1)
	}
	play(tr.ext, 0, 24, 1, 0) // a low note, so nearest-sample playback holds every sample
	buf := make([]int16, 2*sampleRate)
	openmpt.read(tr.mod, sampleRate, sampleRate, &buf[0])
	out := 0
	for _, v := range buf {
		out = max(out, int(v), -int(v))
	}
	t.Logf("output peak %d for sample peak %d", out, peak)
	if g := float64(out) / float64(peak); math.Abs(g/fxGain-1) > 0.005 {
		t.Errorf("libopenmpt gain %.3f per sample unit (peak %d of %d), fxGain %.3f", g, out, peak, fxGain)
	}
}

// TestPan's oracle is the presentation choice documented on pan (the original centres every
// effect): no lemming is centred; the view's centre (column 160 of 320) is centred; the view's left
// edge and anything left of it hard left, anything right of the view hard right; linear in between;
// the view's position, not the map's, decides.
func TestPan(t *testing.T) {
	for _, c := range []struct {
		x, scroll int
		l, r      float64
	}{
		{-1, 500, 1, 1},
		{660, 500, 1, 1},
		{500, 500, 1, 0},
		{10, 500, 1, 0},
		{820, 500, 0, 1},
		{1600, 500, 0, 1},
		{580, 500, 1, 0.5},
		{740, 500, 0.5, 1},
		{160, 0, 1, 1},
	} {
		if l, r := pan(c.x, c.scroll); l != c.l || r != c.r {
			t.Errorf("pan(%d, %d) = %v, %v; want %v, %v", c.x, c.scroll, l, r, c.l, c.r)
		}
	}
}

// TestLibPaths checks that libopenmpt is looked for beside the executable first, under the name a
// release bundles it as, and then by the system's names.
func TestLibPaths(t *testing.T) {
	dir := filepath.Join("opt", "openlemm")
	exe := filepath.Join(dir, "openlemm")
	for goos, want := range map[string][]string{
		"darwin":  {filepath.Join(dir, "libopenmpt.dylib"), "libopenmpt.dylib", "/opt/homebrew/lib/libopenmpt.dylib", "/usr/local/lib/libopenmpt.dylib"},
		"linux":   {filepath.Join(dir, "libopenmpt.so.0"), "libopenmpt.so.0"},
		"windows": {filepath.Join(dir, "libopenmpt.dll"), "libopenmpt.dll"},
	} {
		if got := libPaths(goos, exe); !slices.Equal(got, want) {
			t.Errorf("%s: %q, want %q", goos, got, want)
		}
	}
}

// TestTrackerPlays plays a one-note module built here, needing no disc, through newTracker's
// settings and checks the first tenth of a second is not silent: libopenmpt loads and its
// functions, float arguments and results included, are called correctly on this system. It skips
// without libopenmpt.
func TestTrackerPlays(t *testing.T) {
	if err := loadOpenMPT(); err != nil {
		t.Skip(err)
	}
	// A 4-channel ProTracker module: sample 1 a looped 64-byte square wave at volume 64, one
	// pattern whose first row plays it on channel 1 at period 428.
	mod := make([]byte, 1084+1024+64)
	binary.BigEndian.PutUint16(mod[20+22:], 32)
	mod[20+25] = 64
	binary.BigEndian.PutUint16(mod[20+28:], 32)
	mod[950] = 1
	copy(mod[1080:], "M.K.")
	copy(mod[1084:], []byte{0x01, 0xAC, 0x10, 0x00})
	for i := range 64 {
		mod[1084+1024+i] = byte(int8(100 - 200*(i/32)))
	}
	tr, err := newTracker(mod, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer openmpt.destroy(tr.ext)
	buf := make([]int16, 2*sampleRate/10)
	openmpt.read(tr.mod, sampleRate, uintptr(len(buf)/2), &buf[0])
	peak := 0
	for _, v := range buf {
		peak = max(peak, int(v), -int(v))
	}
	if peak < 1000 {
		t.Errorf("output peak %d", peak)
	}
}
