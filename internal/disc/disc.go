// Package disc reads the original game's ADFS D-format floppy image, as a sector image or a
// JASPP JFD, and unpacks its ByteKiller-compressed files.
package disc

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
)

const (
	rootDir    = 0x400
	dirEntries = 77
	entrySize  = 26
	attrDir    = 1 << 3

	tracks      = 160 // 80 cylinders × 2 sides
	sectors     = 5   // per track
	sectorBytes = 1024
	noEntry     = 0xFFFFFFFF
)

// Disc holds the files of a disc image by full path, such as "/!Lemmings/Lemmings/LemProg".
type Disc struct {
	// Files slices alias the sector image.
	Files map[string][]byte
}

// Read parses the ADFS directory tree of img, a sector image or a JASPP JFD (gzip-compressed or not).
func Read(img []byte) (*Disc, error) {
	if bytes.HasPrefix(img, []byte{0x1f, 0x8b}) || bytes.HasPrefix(img, []byte("JFDI")) {
		var err error
		if img, err = fromJFD(img); err != nil {
			return nil, err
		}
	}
	d := &Disc{Files: map[string][]byte{}}
	if err := d.readDir(img, rootDir, "", map[int]bool{}); err != nil {
		return nil, err
	}
	return d, nil
}

// fromJFD rebuilds the sector image from a JASPP JFD, gzip-compressed or not (layout per
// MAME's jfd_dsk.cpp). Tracks and sectors the JFD lacks stay zero.
func fromJFD(b []byte) ([]byte, error) {
	if bytes.HasPrefix(b, []byte{0x1f, 0x8b}) {
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("jfd: %w", err)
		}
		if b, err = io.ReadAll(zr); err != nil {
			return nil, fmt.Errorf("jfd: %w", err)
		}
	}
	if len(b) < 36 || string(b[:4]) != "JFDI" {
		return nil, errors.New("jfd: no JFDI header")
	}
	u32 := func(off int) (int, error) {
		if off < 0 || off+4 > len(b) {
			return 0, fmt.Errorf("jfd: table entry at %#x past end of file", off)
		}
		return int(binary.LittleEndian.Uint32(b[off:])), nil
	}
	trackTab, sectorTab, dataTab := int(binary.LittleEndian.Uint32(b[24:])),
		int(binary.LittleEndian.Uint32(b[28:])), int(binary.LittleEndian.Uint32(b[32:]))
	img := make([]byte, tracks*sectors*sectorBytes)
	for t := range (sectorTab - trackTab) / 4 {
		list, err := u32(trackTab + 4*t)
		if err != nil {
			return nil, err
		}
		if list == noEntry {
			continue
		}
		for e := sectorTab + list; ; e += 8 {
			h, err := u32(e)
			if err != nil {
				return nil, err
			}
			if h == noEntry {
				break
			}
			at, err := u32(e + 4)
			if err != nil {
				return nil, err
			}
			if at == noEntry {
				continue
			}
			s, size := h>>8&0xff, 128<<(h&0xf)
			if t >= tracks || s >= sectors || size != sectorBytes {
				return nil, fmt.Errorf("jfd: track %d sector %d of %d bytes is not ADFS D/E", t, s, size)
			}
			src := dataTab + at
			if src+size > len(b) {
				return nil, fmt.Errorf("jfd: track %d sector %d data past end of file", t, s)
			}
			copy(img[(t*sectors+s)*sectorBytes:], b[src:src+size])
		}
	}
	return img, nil
}

func (d *Disc) readDir(img []byte, off int, path string, seen map[int]bool) error {
	if seen[off] {
		return fmt.Errorf("directory loop at %#x (%s)", off, path)
	}
	seen[off] = true
	if off+5+dirEntries*entrySize > len(img) || string(img[off+1:off+5]) != "Hugo" {
		return fmt.Errorf("no directory at %#x (%s)", off, path)
	}
	for i := range dirEntries {
		e := img[off+5+i*entrySize:][:entrySize]
		if e[0] == 0 {
			break
		}
		name := make([]byte, 0, 10)
		for _, b := range e[:10] {
			b &= 0x7f
			if b == '\r' || b == 0 {
				break
			}
			name = append(name, b)
		}
		p := path + "/" + string(name)
		length := int(binary.LittleEndian.Uint32(e[18:]))
		start := int(e[22]) | int(e[23])<<8 | int(e[24])<<16
		start *= 256
		if e[25]&attrDir != 0 {
			if err := d.readDir(img, start, p, seen); err != nil {
				return err
			}
			continue
		}
		if start+length > len(img) {
			return fmt.Errorf("%s: extends past end of image", p)
		}
		d.Files[p] = img[start : start+length]
	}
	return nil
}

// Identify returns the disc version whose file checksums all match.
func (d *Disc) Identify() (string, error) {
	names := make([]string, 0, len(versionA))
	for n := range versionA {
		names = append(names, n)
	}
	for n := range d.Files {
		if _, ok := versionA[n]; !ok {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	for _, n := range names {
		data, ok := d.Files[n]
		sum := sha256.Sum256(data)
		if !ok || hex.EncodeToString(sum[:]) != versionA[n] {
			return "", fmt.Errorf("unknown version: %s does not match any known disc", n)
		}
	}
	return "A", nil
}

// Unpack decompresses the ByteKiller-packed file at path.
func (d *Disc) Unpack(path string) ([]byte, error) {
	f, ok := d.Files[path]
	if !ok {
		return nil, fmt.Errorf("%s: not on disc", path)
	}
	b, err := Unpack(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return b, nil
}

var errChecksum = errors.New("bytekiller: checksum mismatch")

// Unpack decompresses a ByteKiller stream that ends at the end of src.
func Unpack(src []byte) ([]byte, error) {
	if len(src) < 12 || len(src)%4 != 0 {
		return nil, errors.New("bytekiller: no trailer")
	}
	word := func(o int) uint32 { return binary.BigEndian.Uint32(src[o:]) }
	p := len(src) - 4
	size := int(word(p))
	if size <= 0 || size >= 1<<20 {
		return nil, errors.New("bytekiller: implausible size")
	}
	bits := word(p - 8)
	// The packer leaves a marker bit in the initial bit buffer. Without this check, two places in
	// LemProg whose last three words read 0, 0, n pass the checksum as n-byte streams.
	if bits == 0 {
		return nil, errors.New("bytekiller: empty bit buffer")
	}
	crc := word(p-4) ^ bits
	p -= 12
	errShort := errors.New("bytekiller: stream truncated")
	var err error

	bit := func() uint32 {
		c := bits & 1
		bits >>= 1
		if bits == 0 {
			if p < 0 {
				err = errShort
				return 0
			}
			w := word(p)
			p -= 4
			crc ^= w
			c = w & 1
			bits = 0x80000000 | w>>1
		}
		return c
	}
	get := func(n int) int {
		v := 0
		for range n {
			v = v<<1 | int(bit())
		}
		return v
	}

	dst := make([]byte, size)
	d := size - 1
	literal := func(count int) {
		count = min(count, size)
		size -= count
		for range count {
			dst[d] = byte(get(8))
			d--
		}
	}
	copyBack := func(count, offset int) {
		count = min(count, size)
		size -= count
		for range count {
			if d+offset >= len(dst) {
				err = errors.New("bytekiller: offset past end of output")
				return
			}
			dst[d] = dst[d+offset]
			d--
		}
	}

	for size > 0 && err == nil {
		if bit() == 0 {
			if bit() == 0 {
				literal(get(3) + 1)
			} else {
				copyBack(2, get(8))
			}
			continue
		}
		switch get(2) {
		case 3:
			literal(get(8) + 9)
		case 2:
			n := get(8) + 1
			copyBack(n, get(12))
		case 1:
			copyBack(4, get(10))
		default:
			copyBack(3, get(9))
		}
	}
	if err != nil {
		return nil, err
	}
	if crc != 0 {
		return nil, errChecksum
	}
	return dst, nil
}
