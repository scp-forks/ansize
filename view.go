package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

// cp437 maps every byte to its Unicode glyph, including the dingbats DOS
// showed for control codes. NUL and 0xFF render blank, as on a real VGA.
var cp437 = []rune(
	" ☺☻♥♦♣♠•◘○◙♂♀♪♫☼" + // 0x00
		"►◄↕‼¶§▬↨↑↓→←∟↔▲▼" + // 0x10
		" !\"#$%&'()*+,-./" + // 0x20
		"0123456789:;<=>?" + // 0x30
		"@ABCDEFGHIJKLMNO" + // 0x40
		"PQRSTUVWXYZ[\\]^_" + // 0x50
		"`abcdefghijklmno" + // 0x60
		"pqrstuvwxyz{|}~⌂" + // 0x70
		"ÇüéâäàåçêëèïîìÄÅ" + // 0x80
		"ÉæÆôöòûùÿÖÜ¢£¥₧ƒ" + // 0x90
		"áíóúñÑªº¿⌐¬½¼¡«»" + // 0xA0
		"░▒▓│┤╡╢╖╕╣║╗╝╜╛┐" + // 0xB0
		"└┴┬├─┼╞╟╚╔╩╦╠═╬╧" + // 0xC0
		"╨╤╥╙╘╒╓╫╪┘┌█▄▌▐▀" + // 0xD0
		"αßΓπΣσµτΦΘΩδ∞φε∩" + // 0xE0
		"≡±≥≤⌠⌡÷≈°∙·√ⁿ²■ ") // 0xF0 (0xFF renders blank)

type sauceInfo struct {
	title, author, group, date string
	width                      int
	ice                        bool
}

// parseSauce splits a file into its art bytes and SAUCE metadata, honoring
// FileSize, an optional comment block, and the DOS EOF byte.
func parseSauce(data []byte) (*sauceInfo, []byte) {
	if len(data) < 128 || string(data[len(data)-128:len(data)-121]) != "SAUCE00" {
		if i := bytes.IndexByte(data, 0x1A); i >= 0 {
			return nil, data[:i]
		}
		return nil, data
	}
	rec := data[len(data)-128:]
	field := func(b []byte) string { return strings.TrimRight(string(b), " \x00") }
	info := &sauceInfo{
		title:  field(rec[7:42]),
		author: field(rec[42:62]),
		group:  field(rec[62:82]),
		date:   field(rec[82:90]),
		width:  int(binary.LittleEndian.Uint16(rec[96:98])),
		ice:    rec[105]&1 != 0,
	}
	art := data[:len(data)-128]
	if n := int(rec[104]); n > 0 {
		if c := len(art) - n*64 - 5; c >= 0 && string(art[c:c+5]) == "COMNT" {
			art = art[:c]
		}
	}
	if size := int(binary.LittleEndian.Uint32(rec[90:94])); size > 0 && size <= len(art) {
		art = art[:size]
	}
	return info, bytes.TrimSuffix(art, []byte{0x1A})
}

// screen replays an ANSI.SYS byte stream on a modern terminal: CP437
// glyphs become UTF-8, the 16 attribute colors become exact IBM 5153
// truecolor (or the terminal palette with -16), blink becomes a bright
// background when the SAUCE iCE flag is set, and lines wrap at column 80
// the way the hardware did.
type screen struct {
	out                  *bufio.Writer
	width, col           int
	fg, bg               int
	bold, blink, reverse bool
	ice, basic           bool
	state                int // 0 normal, 1 saw ESC, 2 in CSI
	params               []byte
}

func (s *screen) feed(b byte) {
	switch s.state {
	case 1:
		if b == '[' {
			s.state, s.params = 2, s.params[:0]
		} else {
			s.state = 0
		}
		return
	case 2:
		if b >= 0x40 && b <= 0x7E {
			s.state = 0
			s.csi(b)
		} else {
			s.params = append(s.params, b)
		}
		return
	}
	switch b {
	case 0x1B:
		s.state = 1
	case '\r':
		s.out.WriteByte('\r')
		s.col = 0
	case '\n':
		s.out.WriteByte('\n')
	case 0x07: // bell
	case 0x08:
		if s.col > 0 {
			s.col--
			s.out.WriteByte('\b')
		}
	case 0x09:
		next := (s.col/8 + 1) * 8
		if next >= s.width {
			next = s.width - 1
		}
		if next > s.col {
			fmt.Fprintf(s.out, "\x1b[%dC", next-s.col)
			s.col = next
		}
	default:
		s.out.WriteRune(cp437[b])
		s.col++
		// ANSI.SYS wrapped the instant column 80 was written, with no
		// deferred-wrap state -- the reason for the scene's 79-column rule.
		if s.col >= s.width {
			s.out.WriteString("\r\n")
			s.col = 0
		}
	}
}

func (s *screen) csi(final byte) {
	var nums []int
	for _, p := range strings.Split(string(s.params), ";") {
		n, _ := strconv.Atoi(p)
		nums = append(nums, n)
	}
	switch final {
	case 'm':
		s.sgr(nums)
	case 'A', 'B', 'C', 'D', 'H', 'f', 'J', 'K', 's', 'u':
		// The rest of the ANSI.SYS repertoire means the same thing on a
		// modern terminal; pass it through and keep the column estimate.
		s.out.WriteString("\x1b[" + string(s.params) + string(final))
		arg := 1
		if len(nums) > 0 && nums[0] > 0 {
			arg = nums[0]
		}
		switch final {
		case 'C':
			s.col += arg
		case 'D':
			s.col -= arg
		case 'H', 'f':
			s.col = 0
			if len(nums) > 1 && nums[1] > 0 {
				s.col = nums[1] - 1
			}
		case 'J':
			s.col = 0
		}
		if s.col < 0 {
			s.col = 0
		}
		if s.col >= s.width {
			s.col = s.width - 1
		}
	}
}

func (s *screen) sgr(nums []int) {
	if len(nums) == 0 {
		nums = []int{0}
	}
	for i := 0; i < len(nums); i++ {
		n := nums[i]
		switch {
		case n == 0:
			s.bold, s.blink, s.reverse, s.fg, s.bg = false, false, false, 7, 0
		case n == 1:
			s.bold = true
		case n == 5 || n == 6:
			s.blink = true
		case n == 7:
			s.reverse = true
		case n >= 30 && n <= 37:
			s.fg = n - 30
		case n >= 40 && n <= 47:
			s.bg = n - 40
		case n >= 90 && n <= 97:
			s.fg, s.bold = n-90, true
		case n >= 100 && n <= 107:
			s.bg, s.blink = n-100, true
		case n == 38 || n == 48:
			// 256/truecolor extension some modern files use; skip its
			// arguments so they aren't misread as attributes.
			if i+1 < len(nums) && nums[i+1] == 5 {
				i += 2
			} else if i+1 < len(nums) && nums[i+1] == 2 {
				i += 4
			}
		}
	}
	s.emitAttr()
}

func (s *screen) emitAttr() {
	fg, bg := s.fg, s.bg
	if s.bold {
		fg += 8
	}
	if s.blink && s.ice {
		bg += 8
	}
	if s.reverse {
		fg, bg = bg, fg
	}
	var sb strings.Builder
	sb.WriteString("\x1b[0")
	if s.blink && !s.ice {
		sb.WriteString(";5")
	}
	if s.basic {
		if fg >= 8 {
			sb.WriteString(";" + strconv.Itoa(90+fg-8))
		} else {
			sb.WriteString(";" + strconv.Itoa(30+fg))
		}
		if bg >= 8 {
			sb.WriteString(";" + strconv.Itoa(100+bg-8))
		} else {
			sb.WriteString(";" + strconv.Itoa(40+bg))
		}
	} else {
		f, b := cgaPalette[fg], cgaPalette[bg]
		fmt.Fprintf(&sb, ";38;2;%d;%d;%d;48;2;%d;%d;%d", f[0], f[1], f[2], b[0], b[1], b[2])
	}
	sb.WriteByte('m')
	s.out.WriteString(sb.String())
}

// viewFile plays a CP437 .ans/.asc file to the terminal. baud > 0 paces
// the bytes like a serial line (8N1: ten line bits per byte), so 2400
// baud delivers the authentic 240 characters per second.
func viewFile(name string, baud int, basic bool) error {
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	info, art := parseSauce(data)
	// The wrap column is the 80 of real hardware unless SAUCE says the
	// art was made for a wider screen (e.g. 132-column modes).
	width, ice := 80, false
	if info != nil {
		ice = info.ice
		if info.width > 80 {
			width = info.width
		}
	}
	out := bufio.NewWriter(os.Stdout)
	s := &screen{out: out, width: width, fg: 7, ice: ice, basic: basic}

	bps := baud / 10
	if bps > 0 {
		out.WriteString("\x1b[?25l")
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		defer signal.Stop(sig)
		go func() {
			<-sig
			os.Stdout.WriteString("\x1b[0m\x1b[?25h\n")
			os.Exit(130)
		}()
	}
	s.emitAttr()
	start := time.Now()
	for i, b := range art {
		s.feed(b)
		if bps > 0 {
			out.Flush()
			time.Sleep(time.Until(start.Add(time.Duration(i+1) * time.Second / time.Duration(bps))))
		}
	}
	out.WriteString("\x1b[0m")
	if s.col > 0 {
		out.WriteString("\r\n")
	}
	if bps > 0 {
		out.WriteString("\x1b[?25h")
	}
	if info != nil && (info.title != "" || info.author != "" || info.group != "") {
		f := info.title
		if info.author != "" {
			f += " by " + info.author
		}
		if info.group != "" {
			f += " (" + info.group + ")"
		}
		if info.date != "" {
			f += " · " + info.date
		}
		if info.ice {
			f += " · iCE"
		}
		fmt.Fprintf(out, "\x1b[2m%s\x1b[0m\n", strings.TrimLeft(f, " "))
	}
	return out.Flush()
}
