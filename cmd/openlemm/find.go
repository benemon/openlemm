package main

import (
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/benemon/openlemm/internal/disc"
)

// openDisc reads the image at path and checks it is a known release.
func openDisc(path string) (*disc.Disc, error) {
	img, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUnreadable, err)
	}
	d, err := disc.Read(img)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNotDisc, err)
	}
	if _, err := d.Identify(); err != nil {
		return nil, fmt.Errorf("%w: %w", errOtherRelease, err)
	}
	return d, nil
}

// The reasons a player is shown for an image passed over; the wrapped error goes to the terminal.
var (
	errUnreadable   = errors.New("could not be read")
	errNotDisc      = errors.New("not a disc image")
	errOtherRelease = errors.New("a different disc or release")
)

// shownSkips is how many passed-over images the message lists before "and N more".
const shownSkips = 3

// skipReason is the plain reason for an image findDisc passed over.
func skipReason(err error) string {
	for _, r := range []error{errUnreadable, errNotDisc, errOtherRelease} {
		if errors.Is(err, r) {
			return r.Error()
		}
	}
	return errUnreadable.Error()
}

// personalDir is the player's own folder for the disc image on goos.
func personalDir(goos, home, xdgDataHome string) string {
	switch goos {
	case "darwin", "windows":
		return filepath.Join(home, "Documents", "OpenLemm")
	}
	// The XDG base directory spec says to ignore a relative path; its paths are Unix paths.
	if !strings.HasPrefix(xdgDataHome, "/") {
		xdgDataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(xdgDataHome, "openlemm")
}

// discDirs is the folders searched for a disc image, in order: the working directory, the
// executable's directory and the personal folder.
func discDirs(goos string) []string {
	var dirs []string
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	if exe, err := os.Executable(); err == nil {
		if exe, err = filepath.EvalSymlinks(exe); err == nil {
			dirs = append(dirs, filepath.Dir(exe))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, personalDir(goos, home, os.Getenv("XDG_DATA_HOME")))
	}
	return dirs
}

// skip is an image or folder findDisc passed over.
type skip struct {
	path string
	err  error
}

func (s *skip) Error() string { return s.path + ": " + s.err.Error() }
func (s *skip) Unwrap() error { return s.err }

// findDisc returns the first image directly in dirs, taken in order and by name within each,
// that is a known release, and the error of each image passed over.
func findDisc(dirs []string) (*disc.Disc, []error) {
	var skipped []error
	var seen []os.FileInfo
	for _, dir := range dirs {
		fi, err := os.Stat(dir)
		if err != nil || slices.ContainsFunc(seen, func(s os.FileInfo) bool { return os.SameFile(s, fi) }) {
			continue
		}
		seen = append(seen, fi)
		entries, err := os.ReadDir(dir)
		if err != nil {
			skipped = append(skipped, &skip{dir, fmt.Errorf("%w: %w", errUnreadable, err)})
			continue
		}
		for _, e := range entries {
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if e.IsDir() || ext != ".adf" && ext != ".jfd" {
				continue
			}
			path := filepath.Join(dir, e.Name())
			d, err := openDisc(path)
			if err == nil {
				return d, skipped
			}
			skipped = append(skipped, &skip{path, err})
		}
	}
	return nil, skipped
}

// noDiscText is the message shown when no folder in dirs holds a known release.
func noDiscText(dirs []string, skipped []error) []string {
	lines := []string{"No disc image of the Archimedes release was found.", "",
		"Put your disc image (.adf or .jfd) in one of these folders:"}
	for _, d := range dirs {
		lines = append(lines, "  "+d)
	}
	if len(skipped) > 0 {
		lines = append(lines, "", "Skipped:")
		for _, err := range skipped[:min(len(skipped), shownSkips)] {
			var sk *skip
			if errors.As(err, &sk) {
				lines = append(lines, "  "+sk.path+": "+skipReason(err))
			}
		}
		if n := len(skipped) - shownSkips; n > 0 {
			lines = append(lines, fmt.Sprintf("  and %d more", n))
		}
	}
	return append(lines, "", "Click or press a key to quit.")
}

// noDisc shows a text screen on the farewell's 640-wide screen until a click or key.
type noDisc struct {
	pix []uint8
	img *ebiten.Image
}

func (n *noDisc) Update() error {
	if len(inpututil.AppendJustPressedKeys(nil)) > 0 ||
		inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) ||
		inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) ||
		inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonMiddle) {
		return ebiten.Termination
	}
	return nil
}

func (n *noDisc) Draw(dst *ebiten.Image) {
	if n.img == nil {
		rgba := make([]byte, 4*len(n.pix))
		for i, c := range n.pix {
			r, g, b, _ := farewellPal[c].RGBA()
			rgba[4*i], rgba[4*i+1], rgba[4*i+2], rgba[4*i+3] = byte(r>>8), byte(g>>8), byte(b>>8), 0xff
		}
		n.img = ebiten.NewImage(wideW, screenH)
		n.img.WritePixels(rgba)
	}
	bd := dst.Bounds()
	hs, vs, ox, oy := fitWide(bd.Dx(), bd.Dy(), menuH)
	dst.Fill(farewellPal[0])
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(hs), float64(vs))
	op.GeoM.Translate(float64(ox), float64(oy))
	dst.DrawImage(n.img.SubImage(image.Rect(0, viewTop, wideW, viewTop+menuH)).(*ebiten.Image), op)
}

func (n *noDisc) Layout(int, int) (int, int) { panic("unused: LayoutF is called instead") }

func (n *noDisc) LayoutF(w, h float64) (float64, float64) {
	d := ebiten.Monitor().DeviceScaleFactor()
	return w * d, h * d
}
