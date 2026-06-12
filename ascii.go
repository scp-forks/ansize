package main

import (
	"image"
	"strings"

	"github.com/nfnt/resize"
)

// Paul Bourke's 10-level grayscale ramp, densest ink first. 1970s text art
// was monochrome 7-bit ASCII on 80-column terminals and line printers;
// density ramps like this are how video-terminal art shaded once
// overprinting was no longer possible.
const asciiRamp = "@%#*+=-:. "

// renderASCII maps luma to ramp density. By default bright pixels get the
// densest glyphs (light text on a dark terminal); invert flips that for
// dark-on-light output, e.g. printing on paper.
func renderASCII(img image.Image, width int, invert bool) []string {
	imgW := float64(img.Bounds().Dx())
	imgH := float64(img.Bounds().Dy())
	// Character cells are about twice as tall as wide, so sample half as
	// often vertically.
	rows := int(float64(width)*imgH/imgW*0.5 + 0.5)
	if rows < 1 {
		rows = 1
	}
	m := resize.Resize(uint(width), uint(rows), img, resize.Lanczos3)
	lines := make([]string, rows)
	var sb strings.Builder
	for y := 0; y < rows; y++ {
		sb.Reset()
		for x := 0; x < width; x++ {
			r, g, b, _ := m.At(m.Bounds().Min.X+x, m.Bounds().Min.Y+y).RGBA()
			// Rec.601 luma on 8-bit channels
			luma := 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
			frac := luma / 255
			if !invert {
				frac = 1 - frac
			}
			sb.WriteByte(asciiRamp[int(frac*float64(len(asciiRamp)-1)+0.5)])
		}
		lines[y] = strings.TrimRight(sb.String(), " ")
	}
	return lines
}
