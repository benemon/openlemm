package main

import (
	"bytes"
	"image"
	"image/png"
	"log"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

// dockPNG returns the largest of icons as PNG.
func dockPNG(icons []image.Image) ([]byte, error) {
	var b bytes.Buffer
	err := png.Encode(&b, icons[len(icons)-1])
	return b.Bytes(), err
}

// setDockIcon shows the largest of icons in the Dock, where ebiten.SetWindowIcon has no effect.
// It needs the running NSApplication.
func setDockIcon(icons []image.Image) {
	b, err := dockPNG(icons)
	if err != nil {
		log.Print(err)
		return
	}
	data := objc.ID(objc.GetClass("NSData")).Send(objc.RegisterName("dataWithBytes:length:"), unsafe.Pointer(&b[0]), uint(len(b)))
	img := objc.ID(objc.GetClass("NSImage")).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithData:"), data)
	app := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	// Update runs off the main thread, which AppKit requires; waiting could deadlock with it.
	app.Send(objc.RegisterName("performSelectorOnMainThread:withObject:waitUntilDone:"),
		objc.RegisterName("setApplicationIconImage:"), img, false)
}
