package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nfnt/resize"
)

const (
	ANSI_BASIC_BASE  int     = 16
	ANSI_COLOR_SPACE uint32  = 6
	ANSI_FOREGROUND  string  = "38"
	ANSI_RESET       string  = "\x1b[0m"
	CHARACTERS       string  = "01"
	MODERN_WIDTH     int     = 100
	PROPORTION       float32 = 0.46
	RGBA_COLOR_SPACE uint32  = 1 << 16
	// BBS-era ANSIs kept to 79 columns: printing in column 80 triggered
	// auto-wrap on 80-column screens and corrupted the art.
	BBS_WIDTH int = 79
)

func toAnsiCode(c color.Color) string {
	r, g, b, _ := c.RGBA()
	code := int(ANSI_BASIC_BASE + toAnsiSpace(r)*36 + toAnsiSpace(g)*6 + toAnsiSpace(b))
	if code == ANSI_BASIC_BASE {
		return ANSI_RESET
	}
	return "\033[" + ANSI_FOREGROUND + ";5;" + strconv.Itoa(code) + "m"
}

func toAnsiSpace(val uint32) int {
	return int(float32(ANSI_COLOR_SPACE) * (float32(val) / float32(RGBA_COLOR_SPACE)))
}

func writeAnsiImage(img image.Image, file *os.File, width int, charset string) {
	imgW, imgH := float32(img.Bounds().Dx()), float32(img.Bounds().Dy())
	height := float32(width) * (imgH / imgW) * PROPORTION
	m := resize.Resize(uint(width), uint(height), img, resize.Lanczos3)
	var current, previous string
	bounds := m.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			current = toAnsiCode(m.At(x, y))
			if current != previous {
				fmt.Print(current)
				file.WriteString(current)
				previous = current
			}
			if ANSI_RESET != current {
				char := string(charset[rand.Int()%len(charset)])
				fmt.Print(char)
				file.WriteString(char)
			} else {
				fmt.Print(" ")
				file.WriteString(" ")
			}
		}
		fmt.Print("\n")
		file.WriteString("\n")
	}
	fmt.Print(ANSI_RESET)
	file.WriteString(ANSI_RESET)
}

func main() {
	mode := flag.String("mode", "bbs", `output style: "bbs" (1980s 16-color CP437 .ans), "ascii" (1970s 7-bit ASCII), "modern" (256-color, the original ansize behavior)`)
	widthFlag := flag.Int("width", 0, "output width in columns (default: 79 for bbs/ascii, 100 for modern)")
	ice := flag.Bool("ice", false, "bbs: iCE colors -- all 16 background colors, with blink reinterpreted as bright background")
	dither := flag.Bool("dither", true, "bbs: ordered (Bayer 4x4) dithering")
	noSauce := flag.Bool("no-sauce", false, "bbs: do not append a SAUCE metadata record")
	title := flag.String("title", "", "SAUCE title (default: output file name)")
	author := flag.String("author", "", "SAUCE author")
	group := flag.String("group", "", "SAUCE group")
	invert := flag.Bool("invert", false, "ascii: dense glyphs for dark pixels, for dark-on-light output")
	charset := flag.String("charset", CHARACTERS, "modern: characters to draw with")
	baud := flag.Int("baud", 0, "view: pace playback like a modem at this rate (300, 1200, 2400...; 0 = instant)")
	basic := flag.Bool("16", false, "view: terminal's own 16 colors instead of exact CGA truecolor")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: ansize [options] <image> <output> [width]   convert an image")
		fmt.Fprintln(os.Stderr, "       ansize [options] <art.ans|art.asc>          view a CP437 ANSI/ASCII file")
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	if len(args) == 1 {
		switch strings.ToLower(filepath.Ext(args[0])) {
		case ".png", ".jpg", ".jpeg", ".gif":
			fmt.Println("Converting an image needs an output file: ansize " + args[0] + " out.ans")
			return
		}
		if err := viewFile(args[0], *baud, *basic); err != nil {
			fmt.Println("Could not view " + args[0] + ": " + err.Error())
		}
		return
	}
	if len(args) < 2 {
		flag.Usage()
		return
	}
	imageName, outputName := args[0], args[1]
	width := *widthFlag
	if len(args) >= 3 {
		var err error
		width, err = strconv.Atoi(args[2])
		if err != nil {
			fmt.Println("Invalid width " + args[2] + ". Please enter an integer.")
			return
		}
	}
	if width <= 0 {
		if *mode == "modern" {
			width = MODERN_WIDTH
		} else {
			width = BBS_WIDTH
		}
	}

	imageFile, err := os.Open(imageName)
	if err != nil {
		fmt.Println("Could not open image " + imageName)
		return
	}
	defer imageFile.Close()
	outFile, err := os.Create(outputName)
	if err != nil {
		fmt.Println("Could not open " + outputName + " for writing")
		return
	}
	defer outFile.Close()
	img, _, err := image.Decode(bufio.NewReader(imageFile))
	if err != nil {
		fmt.Println("Could not decode image")
		return
	}

	switch *mode {
	case "bbs":
		grid := renderBBS(img, width, *ice, *dither)
		writeBBSGrid(grid, newAnsiWriter(os.Stdout, true))
		if err := writeBBSGrid(grid, newAnsiWriter(outFile, false)); err != nil {
			fmt.Println("Could not write " + outputName)
			return
		}
		if !*noSauce {
			t := *title
			if t == "" {
				t = strings.TrimSuffix(filepath.Base(outputName), filepath.Ext(outputName))
			}
			if err := writeSauce(outFile, t, *author, *group, width, len(grid), *ice); err != nil {
				fmt.Println("Could not write SAUCE record")
			}
		}
	case "ascii":
		lines := renderASCII(img, width, *invert)
		w := bufio.NewWriter(outFile)
		for _, line := range lines {
			fmt.Println(line)
			w.WriteString(line)
			w.WriteString("\r\n")
		}
		if err := w.Flush(); err != nil {
			fmt.Println("Could not write " + outputName)
		}
	case "modern":
		writeAnsiImage(img, outFile, width, *charset)
	default:
		fmt.Println("Unknown mode " + *mode + `. Use "bbs", "ascii", or "modern".`)
	}
}
