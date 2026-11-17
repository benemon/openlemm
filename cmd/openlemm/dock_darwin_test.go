package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// TestDockPNG checks the PNG handed to the Dock decodes to exactly the largest icon.
func TestDockPNG(t *testing.T) {
	f := image.NewPaletted(image.Rect(0, 0, 16, 13), color.Palette{color.Transparent, color.RGBA{0xff, 0xdd, 0xdd, 0xff}, color.RGBA{0, 0xbb, 0, 0xff}})
	for i := range f.Pix {
		f.Pix[i] = uint8(i * 7 % 11 % 3)
	}
	icon := icons(f)
	b, err := dockPNG(icon)
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	want := icon[len(icon)-1]
	if got.Bounds() != image.Rect(0, 0, 64, 64) {
		t.Fatalf("%v, want 64 × 64", got.Bounds())
	}
	for y := range 64 {
		for x := range 64 {
			if g, w := color.RGBAModel.Convert(got.At(x, y)), color.RGBAModel.Convert(want.At(x, y)); g != w {
				t.Fatalf("(%d, %d): %v, want %v", x, y, g, w)
			}
		}
	}
}
