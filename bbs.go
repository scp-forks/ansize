package main

import (
	"bufio"
	"image"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/nfnt/resize"
)

// The CP437 working set of classic ANSI art. Half blocks paint the
// foreground on one half of the cell and leave the background on the
// other, doubling vertical resolution; shade blocks mix foreground over
// background at a fixed coverage (their VGA glyphs literally light 25%,
// 50%, 75% of the cell's pixels).
const (
	glyphSpace = iota
	glyphLight  // ░ 25% fg
	glyphMedium // ▒ 50% fg
	glyphDark   // ▓ 75% fg
	glyphFull   // █
	glyphUpper  // ▀ fg on top half
	glyphLower  // ▄ fg on bottom half
)

var (
	glyphCP437 = [7]byte{0x20, 0xB0, 0xB1, 0xB2, 0xDB, 0xDF, 0xDC}
	glyphRune  = [7]rune{' ', '░', '▒', '▓', '█', '▀', '▄'}
)

// dontCare marks an attribute the glyph makes invisible (foreground of a
// space, background of a full block) so the emitter can keep whatever
// attribute is already set instead of wasting escape bytes.
const dontCare = -1

type cell struct {
	glyph  int
	fg, bg int
}

type bbsMatcher struct {
	nBg     int
	shade   [16][16][3][3]uint8 // [fg][bg][shade index] -> perceived color
	clash   [16][16]float64     // penalty for shading visually distant pairs
}

// shadeClashWeight scales the penalty for shade blocks whose two colors
// clash. A ▒ mixing complementary colors can land numerically close to a
// target, but at cell size the eye sees the speckled texture, not the mix.
const shadeClashWeight = 0.05

// newBBSMatcher precomputes the perceived color of every shade-block
// combination. Without iCE colors only the 8 low-intensity backgrounds
// exist (the attribute high bit means blink, not bright background).
func newBBSMatcher(iceColors bool) *bbsMatcher {
	m := &bbsMatcher{nBg: 8}
	if iceColors {
		m.nBg = 16
	}
	coverage := [3]float64{0.25, 0.50, 0.75}
	for f := 0; f < 16; f++ {
		for b := 0; b < m.nBg; b++ {
			for s, c := range coverage {
				m.shade[f][b][s] = mixColors(cgaPalette[f], cgaPalette[b], c)
			}
			m.clash[f][b] = shadeClashWeight * colorDist(cgaPalette[f], cgaPalette[b])
		}
	}
	return m
}

// bestCell picks the (glyph, fg, bg) triple whose top and bottom halves
// come closest to the two target samples.
func (m *bbsMatcher) bestCell(top, bot [3]uint8) cell {
	var dTop, dBot [16]float64
	for i := 0; i < 16; i++ {
		dTop[i] = colorDist(cgaPalette[i], top)
		dBot[i] = colorDist(cgaPalette[i], bot)
	}
	best := cell{glyphSpace, dontCare, 0}
	bestErr := math.Inf(1)
	for b := 0; b < m.nBg; b++ {
		if e := dTop[b] + dBot[b]; e < bestErr {
			bestErr, best = e, cell{glyphSpace, dontCare, b}
		}
	}
	for f := 0; f < 16; f++ {
		if e := dTop[f] + dBot[f]; e < bestErr {
			bestErr, best = e, cell{glyphFull, f, dontCare}
		}
	}
	for f := 0; f < 16; f++ {
		for b := 0; b < m.nBg; b++ {
			if f == b {
				continue
			}
			if e := dTop[f] + dBot[b]; e < bestErr {
				bestErr, best = e, cell{glyphUpper, f, b}
			}
			if e := dTop[b] + dBot[f]; e < bestErr {
				bestErr, best = e, cell{glyphLower, f, b}
			}
			for s := 0; s < 3; s++ {
				mix := m.shade[f][b][s]
				if e := colorDist(mix, top) + colorDist(mix, bot) + m.clash[f][b]; e < bestErr {
					bestErr, best = e, cell{glyphLight + s, f, b}
				}
			}
		}
	}
	return best
}

var bayer4 = [4][4]float64{
	{0, 8, 2, 10},
	{12, 4, 14, 6},
	{3, 11, 1, 9},
	{15, 7, 13, 5},
}

// ditherStrength ~ 255 / effective tone levels of the shade-mix palette.
const ditherStrength = 24.0

func sampleAt(m image.Image, x, y int, dither bool) [3]uint8 {
	// RGBA is alpha-premultiplied, so transparent pixels read as black --
	// the unpainted screen. ANSI has no alpha; this is its transparency.
	r, g, b, _ := m.At(m.Bounds().Min.X+x, m.Bounds().Min.Y+y).RGBA()
	c := [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
	if dither {
		t := (bayer4[y%4][x%4]/16 - 0.5) * ditherStrength
		for i := range c {
			v := float64(c[i]) + t
			if v < 0 {
				v = 0
			} else if v > 255 {
				v = 255
			}
			c[i] = uint8(v)
		}
	}
	return c
}

// renderBBS samples the image at two vertical points per character cell.
// Cells in an 8x16 DOS font are twice as tall as wide, so the half-cell
// sample grid is width x (width * imgH/imgW) -- square virtual pixels.
func renderBBS(img image.Image, width int, iceColors, dither bool) [][]cell {
	imgW := float64(img.Bounds().Dx())
	imgH := float64(img.Bounds().Dy())
	rows := int(float64(width)*imgH/imgW/2 + 0.5)
	if rows < 1 {
		rows = 1
	}
	m := resize.Resize(uint(width), uint(rows*2), img, resize.Lanczos3)
	matcher := newBBSMatcher(iceColors)
	grid := make([][]cell, rows)
	for y := range grid {
		grid[y] = make([]cell, width)
		for x := range grid[y] {
			top := sampleAt(m, x, 2*y, dither)
			bot := sampleAt(m, x, 2*y+1, dither)
			grid[y][x] = matcher.bestCell(top, bot)
		}
	}
	return grid
}

// ansiWriter emits cells as ANSI.SYS-compatible output, tracking the
// current SGR state to keep escapes minimal. In file mode it writes raw
// CP437 bytes with CRLF line endings and expresses bright backgrounds as
// blink (SGR 5), the iCE colors convention. In terminal mode it writes
// UTF-8 glyphs and aixterm bright backgrounds (100-107) so the preview
// doesn't blink on modern emulators.
type ansiWriter struct {
	w        *bufio.Writer
	terminal bool
	fg, bg   int
}

func newAnsiWriter(w io.Writer, terminal bool) *ansiWriter {
	return &ansiWriter{w: bufio.NewWriter(w), terminal: terminal, fg: 7, bg: 0}
}

func (w *ansiWriter) setAttr(fg, bg int) {
	var p []string
	// ANSI.SYS has no "bold off" (22) or "blink off" (25): dropping
	// intensity or blink requires a full reset, then rebuilding state.
	reset := (fg != dontCare && fg < 8 && w.fg >= 8) ||
		(!w.terminal && bg != dontCare && bg < 8 && w.bg >= 8)
	if reset {
		p = append(p, "0")
		w.fg, w.bg = 7, 0
	}
	if fg != dontCare && fg != w.fg {
		if fg >= 8 && w.fg < 8 {
			p = append(p, "1")
		}
		if fg&7 != w.fg&7 {
			p = append(p, strconv.Itoa(30+fg&7))
		}
		w.fg = fg
	}
	if bg != dontCare && bg != w.bg {
		if w.terminal {
			if bg >= 8 {
				p = append(p, strconv.Itoa(100+bg&7))
			} else {
				p = append(p, strconv.Itoa(40+bg))
			}
		} else {
			if bg >= 8 && w.bg < 8 {
				p = append(p, "5")
			}
			if bg&7 != w.bg&7 {
				p = append(p, strconv.Itoa(40+bg&7))
			}
		}
		w.bg = bg
	}
	if len(p) > 0 {
		w.w.WriteString("\x1b[" + strings.Join(p, ";") + "m")
	}
}

func (w *ansiWriter) writeCell(c cell) {
	w.setAttr(c.fg, c.bg)
	if w.terminal {
		w.w.WriteRune(glyphRune[c.glyph])
	} else {
		w.w.WriteByte(glyphCP437[c.glyph])
	}
}

func (w *ansiWriter) newline() {
	if w.terminal {
		w.w.WriteByte('\n')
	} else {
		w.w.WriteString("\r\n")
	}
}

func (w *ansiWriter) reset() {
	w.w.WriteString("\x1b[0m")
	w.fg, w.bg = 7, 0
}

func writeBBSGrid(grid [][]cell, w *ansiWriter) error {
	w.reset()
	for _, row := range grid {
		// Rows end at the last visible cell; untouched cells stay
		// screen-background black, the way TheDraw saved its output.
		end := len(row)
		for end > 0 && row[end-1].glyph == glyphSpace && row[end-1].bg == 0 {
			end--
		}
		for x := 0; x < end; x++ {
			w.writeCell(row[x])
		}
		w.newline()
	}
	w.reset()
	return w.w.Flush()
}
