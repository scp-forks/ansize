package main

import "math"

// The CGA/VGA 16-color text-mode palette, indexed in SGR order so that
// index&7 is the ANSI color digit (30+n foreground, 40+n background) and
// index >= 8 means high intensity (SGR 1 for foreground, blink/iCE for
// background). Values follow the IBM 5153 monitor: each channel is 0xAA
// when its RGBI bit is set, +0x55 when the intensity bit is set. Color 3
// is the hardware special case: the 5153 halved green for I=0,R=1,G=1,B=0,
// giving brown #AA5500 instead of dark yellow #AAAA00.
var cgaPalette = [16][3]uint8{
	{0x00, 0x00, 0x00}, // 0  black
	{0xAA, 0x00, 0x00}, // 1  red
	{0x00, 0xAA, 0x00}, // 2  green
	{0xAA, 0x55, 0x00}, // 3  brown
	{0x00, 0x00, 0xAA}, // 4  blue
	{0xAA, 0x00, 0xAA}, // 5  magenta
	{0x00, 0xAA, 0xAA}, // 6  cyan
	{0xAA, 0xAA, 0xAA}, // 7  light gray
	{0x55, 0x55, 0x55}, // 8  dark gray
	{0xFF, 0x55, 0x55}, // 9  bright red
	{0x55, 0xFF, 0x55}, // 10 bright green
	{0xFF, 0xFF, 0x55}, // 11 yellow
	{0x55, 0x55, 0xFF}, // 12 bright blue
	{0xFF, 0x55, 0xFF}, // 13 bright magenta
	{0x55, 0xFF, 0xFF}, // 14 bright cyan
	{0xFF, 0xFF, 0xFF}, // 15 white
}

var srgbToLin [256]float64

func init() {
	for i := range srgbToLin {
		srgbToLin[i] = math.Pow(float64(i)/255, 2.2)
	}
}

func linToSrgb(l float64) uint8 {
	if l <= 0 {
		return 0
	}
	if l >= 1 {
		return 255
	}
	return uint8(math.Pow(l, 1/2.2)*255 + 0.5)
}

// mixColors blends fg over bg at the given coverage fraction. Averaging
// gamma-encoded sRGB values would make the mix too bright, so the blend
// happens in linear light.
func mixColors(fg, bg [3]uint8, coverage float64) [3]uint8 {
	var out [3]uint8
	for i := 0; i < 3; i++ {
		l := coverage*srgbToLin[fg[i]] + (1-coverage)*srgbToLin[bg[i]]
		out[i] = linToSrgb(l)
	}
	return out
}

// colorDist is the "redmean" weighted RGB distance
// (https://www.compuphase.com/cmetric.htm), a cheap approximation of
// perceptual difference that behaves well on the small CGA palette.
func colorDist(a, b [3]uint8) float64 {
	rmean := (float64(a[0]) + float64(b[0])) / 2
	dr := float64(a[0]) - float64(b[0])
	dg := float64(a[1]) - float64(b[1])
	db := float64(a[2]) - float64(b[2])
	return (2+rmean/256)*dr*dr + 4*dg*dg + (2+(255-rmean)/256)*db*db
}
