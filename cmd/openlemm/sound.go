package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

const sampleRate = 48000

// readPeriods reads the 40-entry sound pitch table, Amiga periods, at &314 in the unpacked LemProg,
// checking the zero word after it.
func readPeriods(prog []byte) ([40]int, error) {
	var p [40]int
	const off = 0x314
	if len(prog) < off+4*41 || binary.LittleEndian.Uint32(prog[off+4*40:]) != 0 {
		return p, errors.New("LemProg: no sound pitch table at &314")
	}
	for i := range p {
		p[i] = int(binary.LittleEndian.Uint32(prog[off+4*i:]))
	}
	return p, nil
}

// fxGain is libopenmpt's output, in 16-bit units per signed 8-bit sample unit, for one centred note
// at full volume (64) with nearest-sample playback: measured by TestEffectLoudness. An effect at
// volume 255 plays at this gain.
const fxGain = 7232.0 / 114

// priority is the priority of effect sample s.
func priority(s int) int {
	switch s {
	case 0x1D:
		return 3
	case 0x15, 0x16, 0x18, 0x1C:
		return 2
	case 0x01, 0x14:
		return 0
	}
	return 1
}

// readSamples splits SampdatP into its 31 signed 8-bit samples.
func readSamples(b []byte) ([31][]int8, error) {
	var s [31][]int8
	if len(b) < 31*16 {
		return s, fmt.Errorf("SampdatP: %d bytes", len(b))
	}
	for i := range s {
		off := int(binary.LittleEndian.Uint32(b[16*i:]))
		n := int(binary.LittleEndian.Uint32(b[16*i+4:]))
		if off+n > len(b) {
			return s, fmt.Errorf("SampdatP sample %d: %d bytes at %d, past the end", i, n, off)
		}
		s[i] = unsafe.Slice((*int8)(unsafe.Pointer(unsafe.SliceData(b[off:]))), n)
	}
	return s, nil
}

// effect is the one sound effect playing, or nil data when none is.
type effect struct {
	data []int8
	pos  float64 // in samples
	step float64 // samples per output frame
	prio int
	l, r float64 // left and right gains
}

func (e *effect) playing() bool { return int(e.pos) < len(e.data) }

// request starts sample s at rate samples per second unless a higher-priority effect is still
// playing.
func (e *effect) request(samples *[31][]int8, s int, rate, l, r float64) {
	pr := priority(s)
	if e.playing() && pr < e.prio {
		return
	}
	*e = effect{data: samples[s], step: rate / sampleRate, prio: pr, l: l, r: r}
}

// pan returns the left and right gains of an effect for the lemming at map x, with the view's left
// edge at map x scroll; x < 0 is no lemming. The original centres every effect.
func pan(x, scroll int) (l, r float64) {
	if x < 0 {
		return 1, 1
	}
	p := min(max(float64(x-scroll-screenW/2)/(screenW/2), -1), 1)
	return min(1, 1-p), min(1, 1+p)
}

// next returns the effect's next output value at the nearest sample, in -128..127, and advances.
func (e *effect) next() int {
	v := int(e.data[int(e.pos)])
	e.pos += e.step
	return v
}

// tracker plays one module with libopenmpt, loaded with purego.
type tracker struct {
	ext, mod uintptr
	mute     func(ext uintptr, ch int32, mute int32) int32
}

var openmpt struct {
	once     sync.Once
	err      error
	create   func(data unsafe.Pointer, size uintptr, logf, logu, errf, erru uintptr, err, msg unsafe.Pointer, ctls uintptr) uintptr
	module   func(ext uintptr) uintptr
	iface    func(ext uintptr, id string, iface unsafe.Pointer, size uintptr) int32
	position func(mod uintptr, order, row int32) float64
	ctl      func(mod uintptr, ctl string, v float64) int32
	param    func(mod uintptr, param, v int32) int32
	repeat   func(mod uintptr, n int32) int32
	destroy  func(ext uintptr)
	read     func(mod uintptr, rate int32, count uintptr, buf *int16) uintptr
}

// libPaths returns where libopenmpt is looked for on goos, in order: beside the executable exe,
// where a release bundles it, then by the names the system's own search resolves.
func libPaths(goos, exe string) []string {
	var names []string
	switch goos {
	case "darwin":
		names = []string{"libopenmpt.dylib", "/opt/homebrew/lib/libopenmpt.dylib", "/usr/local/lib/libopenmpt.dylib"}
	case "windows":
		names = []string{"libopenmpt.dll"}
	default:
		names = []string{"libopenmpt.so.0"}
	}
	return append([]string{filepath.Join(filepath.Dir(exe), names[0])}, names...)
}

func loadOpenMPT() error {
	openmpt.once.Do(func() {
		exe, err := os.Executable()
		if err != nil {
			openmpt.err = err
			return
		}
		var lib uintptr
		for _, path := range libPaths(runtime.GOOS, exe) {
			if l, err := openLibrary(path); err == nil {
				lib = l
				break
			}
		}
		if lib == 0 {
			openmpt.err = errors.New("libopenmpt not found")
			return
		}
		purego.RegisterLibFunc(&openmpt.create, lib, "openmpt_module_ext_create_from_memory")
		purego.RegisterLibFunc(&openmpt.module, lib, "openmpt_module_ext_get_module")
		purego.RegisterLibFunc(&openmpt.destroy, lib, "openmpt_module_ext_destroy")
		purego.RegisterLibFunc(&openmpt.iface, lib, "openmpt_module_ext_get_interface")
		purego.RegisterLibFunc(&openmpt.position, lib, "openmpt_module_set_position_order_row")
		purego.RegisterLibFunc(&openmpt.ctl, lib, "openmpt_module_ctl_set_floatingpoint")
		purego.RegisterLibFunc(&openmpt.param, lib, "openmpt_module_set_render_param")
		purego.RegisterLibFunc(&openmpt.repeat, lib, "openmpt_module_set_repeat_count")
		purego.RegisterLibFunc(&openmpt.read, lib, "openmpt_module_read_interleaved_stereo")
	})
	return openmpt.err
}

// newTracker loads module data, positioned at the first row of order position start, with the
// original's playback settings.
func newTracker(data []byte, start int) (*tracker, error) {
	if err := loadOpenMPT(); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("empty module")
	}
	ext := openmpt.create(unsafe.Pointer(&data[0]), uintptr(len(data)), 0, 0, 0, 0, nil, nil, 0)
	if ext == 0 {
		return nil, errors.New("libopenmpt: module not loaded")
	}
	t := &tracker{ext: ext, mod: openmpt.module(ext)}

	// The C structs openmpt_module_ext_interface_interactive and ..._interactive2: 16 and 6
	// function pointers.
	var ia [16]uintptr
	var ia2 [6]uintptr
	if openmpt.iface(ext, "interactive", unsafe.Pointer(&ia), unsafe.Sizeof(ia)) == 0 ||
		openmpt.iface(ext, "interactive2", unsafe.Pointer(&ia2), unsafe.Sizeof(ia2)) == 0 {
		return nil, errors.New("libopenmpt: no interactive interface")
	}
	purego.RegisterFunc(&t.mute, ia[10]) // set_channel_mute_status
	var pan func(ext uintptr, ch int32, p float64) int32
	purego.RegisterFunc(&pan, ia2[2]) // set_channel_panning

	const (
		interpolation = 3                      // OPENMPT_MODULE_RENDER_INTERPOLATIONFILTER_LENGTH
		pitch         = 3593750.0 / 428 / 8363 // the effects' rate at period 428 over ProTracker's
	)
	ok := openmpt.repeat(t.mod, -1) == 1 &&
		openmpt.param(t.mod, interpolation, 1) == 1 &&
		openmpt.ctl(t.mod, "play.tempo_factor", 48.83/50) == 1 &&
		openmpt.ctl(t.mod, "play.pitch_factor", pitch) == 1 &&
		openmpt.position(t.mod, int32(start), 0) >= 0
	for ch, p := range [4]float64{0, -1, 1, 0} {
		ok = ok && pan(ext, int32(ch), p) == 1
	}
	if !ok {
		return nil, errors.New("libopenmpt: a playback setting was refused")
	}
	return t, nil
}

// mixer is the audio stream: the music, with channel 4 muted while an effect plays in its place,
// and the one effect.
type mixer struct {
	mu       sync.Mutex
	samples  *[31][]int8
	periods  [40]int
	music    *tracker
	on       bool // the music has started
	musicVol int
	fxVol    int
	fx       effect
	quiet    bool // the speaker is off
	buf      []int16
}

func (m *mixer) request(s, p, vol, x, scroll int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, r := pan(x, scroll)
	m.fx.request(m.samples, s, 3593750/float64(m.periods[p]), l, r)
	m.fxVol = vol
}

// play replaces the tune, which is silent until set starts it; nil is none.
func (m *mixer) play(t *tracker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.music != nil {
		openmpt.destroy(m.music.ext)
	}
	m.music, m.on = t, false
}

func (m *mixer) set(musicOn bool, musicVol int, quiet bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.on = m.on || musicOn
	m.musicVol = musicVol
	m.quiet = quiet
}

// Read fills p with 16-bit little-endian stereo frames.
func (m *mixer) Read(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(p) / 4
	if cap(m.buf) < 2*n {
		m.buf = make([]int16, 2*n)
	}
	buf := m.buf[:2*n]
	clear(buf)
	if t := m.music; t != nil && m.on && m.musicVol > 0 {
		// A channel's level is about (note volume × setting ÷ 256) ÷ 63 against a note's ÷ 64.
		mB := 2000 * math.Log10(float64(m.musicVol)/256*64/63)
		openmpt.param(t.mod, 1, int32(math.Round(mB))) // OPENMPT_MODULE_RENDER_MASTERGAIN_MILLIBEL
		t.mute(t.ext, 3, b2i(m.fx.playing()))
		openmpt.read(t.mod, sampleRate, uintptr(n), &buf[0])
	}
	for i := range n {
		if !m.fx.playing() {
			break
		}
		v := float64(m.fx.next()) * fxGain * float64(m.fxVol) / 255
		buf[2*i] = clip(int(buf[2*i]) + int(math.Round(v*m.fx.l)))
		buf[2*i+1] = clip(int(buf[2*i+1]) + int(math.Round(v*m.fx.r)))
	}
	for i, v := range buf {
		if m.quiet {
			v = 0
		}
		binary.LittleEndian.PutUint16(p[2*i:], uint16(v))
	}
	return 4 * n, nil
}

func b2i(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

func clip(v int) int16 { return int16(min(max(v, math.MinInt16), math.MaxInt16)) }
