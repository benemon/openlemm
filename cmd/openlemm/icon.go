package main

import (
	"image"
	"image/color"

	"github.com/benemon/openlemm/internal/sim"
)

// iconStates are the actions whose first frame can be the window icon: all but the explosion.
var iconStates = []int{
	0, sim.Falling, sim.Climbing, sim.Hoisting, sim.Floating, sim.Building, sim.Bashing,
	sim.Mining, sim.Shrugging, sim.Digging, sim.Blocking, sim.Splatting, sim.Drowning,
	sim.Exiting, sim.Burning, sim.OhNo,
}

// iconFrame returns the first right-facing frame of a lemming in state, in the level palette
// pal with colour 0 transparent.
func (a *art) iconFrame(state int, pal color.Palette) *image.Paletted {
	l := &sim.Lemming{State: state, Dir: 1}
	s := spriteFor(l)
	l.X, l.Y = -s.ox, 4-s.oy
	view := image.NewPaletted(image.Rect(0, 0, mapW, viewRows), append(color.Palette{color.Transparent}, pal[1:]...))
	a.drawLemming(view, l, false)
	return view.SubImage(image.Rect(0, 0, s.w, s.size/(s.w/2))).(*image.Paletted)
}

// icons centres f on a transparent square and scales it by whole pixels to 16, 32, 48 and 64
// pixels a side.
func icons(f *image.Paletted) []image.Image {
	w, h := f.Rect.Dx(), f.Rect.Dy()
	side := max(w, h)
	var out []image.Image
	for _, n := range []int{16, 32, 48, 64} {
		k := n / side
		img := image.NewPaletted(image.Rect(0, 0, n, n), f.Palette)
		for y := range h * k {
			for x := range w * k {
				img.SetColorIndex((side-w)/2*k+x, (side-h)/2*k+y, f.ColorIndexAt(f.Rect.Min.X+x/k, f.Rect.Min.Y+y/k))
			}
		}
		out = append(out, img)
	}
	return out
}
