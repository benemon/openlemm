package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/benemon/openlemm/internal/disc"
)

// knownImage returns the bytes of the player's disc image.
func knownImage(t *testing.T) []byte {
	t.Helper()
	p := os.Getenv("OPENLEMM_DISC")
	if p == "" {
		t.Skip("OPENLEMM_DISC not set")
	}
	img, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// alteredImage returns the player's disc image with the first byte of MUSINFO flipped: it reads,
// but Identify refuses it.
func alteredImage(t *testing.T) []byte {
	t.Helper()
	img := knownImage(t)
	d, err := disc.Read(img)
	if err != nil {
		t.Fatal(err)
	}
	d.Files["/!Lemmings/Lemmings/LemLevels/MUSINFO"][0] ^= 0xff // Files alias a sector image
	if d, err := disc.Read(img); err != nil {
		t.Fatal(err)
	} else if _, err := d.Identify(); err == nil {
		t.Skip("OPENLEMM_DISC is not a sector image: cannot alter it in place")
	}
	return img
}

func writeFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestFindDisc's oracle is the player's disc (a known release) and the same disc with one byte
// altered (an unknown one), placed in temporary folders.
func TestFindDisc(t *testing.T) {
	known, altered := knownImage(t), alteredImage(t)
	for _, c := range []struct {
		name  string
		files map[string][]byte // by path relative to the temporary root
		dirs  []string          // relative to the root
		found bool
		skip  []string // the skipped images, in order, relative to the root
	}{
		{"a known image in a later folder after an unknown one",
			map[string][]byte{"a/x.adf": altered, "b/y.adf": known},
			[]string{"a", "b"}, true, []string{"a/x.adf"}},
		{"working dir before executable dir before personal",
			map[string][]byte{"wd/a.adf": altered, "wd/b.adf": known, "exe/a.adf": altered, "exe/b.adf": known,
				"home/a.adf": altered, "home/b.adf": known},
			[]string{"wd", "exe", "home"}, true, []string{"wd/a.adf"}},
		{"by name within a folder",
			map[string][]byte{"a/b.adf": known, "a/a.adf": altered, "a/c.adf": altered},
			[]string{"a"}, true, []string{"a/a.adf"}},
		{"not recursive",
			map[string][]byte{"a/sub/x.adf": known, "a/sub.adf/y.adf": known},
			[]string{"a"}, false, nil},
		{"extension in any case",
			map[string][]byte{"a/x.ADF": altered, "a/y.JfD": known},
			[]string{"a"}, true, []string{"a/x.ADF"}},
		{"other extensions ignored",
			map[string][]byte{"a/x.img": known, "a/x.adf.zip": known},
			[]string{"a"}, false, nil},
		{"the same folder listed twice is searched once",
			map[string][]byte{"a/x.adf": altered},
			[]string{"a", "missing", "a"}, false, []string{"a/x.adf"}},
		{"none found returns every image skipped",
			map[string][]byte{"a/x.adf": altered, "b/y.jfd": []byte("not a disc"), "b/z.txt": known},
			[]string{"missing", "a", "b"}, false, []string{"a/x.adf", "b/y.jfd"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			for p, b := range c.files {
				writeFile(t, filepath.Join(root, p), b)
			}
			var dirs []string
			for _, d := range c.dirs {
				dirs = append(dirs, filepath.Join(root, d))
			}
			d, skipped := findDisc(dirs)
			if found := d != nil; found != c.found {
				t.Errorf("found %v, want %v", found, c.found)
			}
			if d != nil {
				if _, err := d.Identify(); err != nil {
					t.Errorf("the disc found is not a known release: %v", err)
				}
			}
			if len(skipped) != len(c.skip) {
				t.Fatalf("skipped %v, want %v", skipped, c.skip)
			}
			for i, s := range c.skip {
				if p := filepath.Join(root, s) + ": "; !strings.HasPrefix(skipped[i].Error(), p) {
					t.Errorf("skipped[%d] = %v, want it to start %q", i, skipped[i], p)
				}
			}
		})
	}
}

// TestFindDiscLinkedFolder checks a folder reached twice, once through a symbolic link, is
// searched once.
func TestFindDiscLinkedFolder(t *testing.T) {
	root := t.TempDir()
	a, link := filepath.Join(root, "a"), filepath.Join(root, "link")
	writeFile(t, filepath.Join(a, "x.adf"), []byte("not a disc"))
	if err := os.Symlink(a, link); err != nil {
		t.Skip(err)
	}
	if d, skipped := findDisc([]string{a, link}); d != nil || len(skipped) != 1 {
		t.Errorf("found %v, skipped %v; want nothing found and one image skipped", d != nil, skipped)
	}
}

// TestDiscDirs checks the folders are the working directory, the executable's directory and the
// personal folder, in that order.
func TestDiscDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_DATA_HOME", "")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		t.Fatal(err)
	}
	got := discDirs("linux")
	want := []string{wd, filepath.Dir(exe), filepath.Join(home, ".local", "share", "openlemm")}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("discDirs = %q, want %q", got, want)
	}
}

// TestPersonalDir's oracle is the settled folder per system and, on Linux, the XDG base directory
// spec: $XDG_DATA_HOME when absolute, else ~/.local/share.
func TestPersonalDir(t *testing.T) {
	j := filepath.Join
	for _, c := range []struct{ goos, home, xdg, want string }{
		{"darwin", "/Users/p", "/x", j("/Users/p", "Documents", "OpenLemm")},
		{"windows", `C:\Users\p`, "", j(`C:\Users\p`, "Documents", "OpenLemm")},
		{"linux", "/home/p", "", j("/home/p", ".local", "share", "openlemm")},
		{"linux", "/home/p", "/data", j("/data", "openlemm")},
		{"linux", "/home/p", "data", j("/home/p", ".local", "share", "openlemm")},
		{"freebsd", "/home/p", "", j("/home/p", ".local", "share", "openlemm")},
	} {
		if got := personalDir(c.goos, c.home, c.xdg); got != c.want {
			t.Errorf("personalDir(%q, %q, %q) = %q, want %q", c.goos, c.home, c.xdg, got, c.want)
		}
	}
}

// TestNoDiscText checks the message: a plain reason per passed-over image,
// the first three listed and the rest counted.
func TestNoDiscText(t *testing.T) {
	skipped := []error{
		&skip{"a.adf", fmt.Errorf("%w: %w", errNotDisc, errors.New("no directory at 0x400 (/)"))},
		&skip{"b.adf", fmt.Errorf("%w: %w", errOtherRelease, errors.New("unknown version"))},
		&skip{"c.jfd", fmt.Errorf("%w: %w", errUnreadable, errors.New("permission denied"))},
		&skip{"d.adf", errNotDisc},
		&skip{"e.adf", errNotDisc},
	}
	text := strings.Join(noDiscText([]string{"/x"}, skipped), "\n")
	for _, want := range []string{
		"  a.adf: not a disc image\n",
		"  b.adf: a different disc or release\n",
		"  c.jfd: could not be read\n",
		"  and 2 more\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("message lacks %q:\n%s", want, text)
		}
	}
	for _, not := range []string{"0x400", "unknown version", "permission", "d.adf"} {
		if strings.Contains(text, not) {
			t.Errorf("message shows %q:\n%s", not, text)
		}
	}
	if strings.Contains(strings.Join(noDiscText([]string{"/x"}, skipped[:3]), "\n"), "more") {
		t.Error("three images skipped: message says more")
	}
}

// TestSkipReason checks the plain reason findDisc gives each kind of image passed over: a disc of
// another release, a file that is no disc, and a file that cannot be read.
func TestSkipReason(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.adf"), alteredImage(t))
	writeFile(t, filepath.Join(root, "b.adf"), []byte("not a disc"))
	if err := os.Mkdir(filepath.Join(root, "c.adf"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "gone"), filepath.Join(root, "d.adf")); err != nil {
		t.Fatal(err)
	}
	_, skipped := findDisc([]string{root})
	var got []string
	for _, err := range skipped {
		got = append(got, filepath.Base(err.(*skip).path)+": "+skipReason(err))
	}
	want := []string{"a.adf: a different disc or release", "b.adf: not a disc image", "d.adf: could not be read"}
	if !slices.Equal(got, want) {
		t.Errorf("skipped %v, want %v", got, want)
	}
}
