package main

import (
	"encoding/binary"
	"io"
	"os"
	"time"
)

// writeSauce appends a SAUCE v00 record (the BBS art scene's metadata
// standard, https://www.acid.org/info/sauce/sauce.htm) to a finished .ans
// file: a DOS EOF byte so viewers stop rendering, then the 128-byte record.
func writeSauce(f *os.File, title, author, group string, width, lines int, iceColors bool) error {
	// FileSize is the length of the art alone, before the EOF byte and record.
	size, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	rec := make([]byte, 0, 129)
	rec = append(rec, 0x1A)
	rec = append(rec, "SAUCE00"...)
	rec = appendField(rec, title, 35, ' ')
	rec = appendField(rec, author, 20, ' ')
	rec = appendField(rec, group, 20, ' ')
	rec = appendField(rec, time.Now().Format("20060102"), 8, ' ')
	rec = binary.LittleEndian.AppendUint32(rec, uint32(size))
	rec = append(rec, 1, 1) // DataType: Character, FileType: ANSi
	rec = binary.LittleEndian.AppendUint16(rec, uint16(width)) // TInfo1: columns
	rec = binary.LittleEndian.AppendUint16(rec, uint16(lines)) // TInfo2: lines
	rec = binary.LittleEndian.AppendUint16(rec, 0)             // TInfo3
	rec = binary.LittleEndian.AppendUint16(rec, 0)             // TInfo4
	rec = append(rec, 0) // no comment block
	var flags byte
	if iceColors {
		flags |= 1 // non-blink mode: SGR 5 means bright background
	}
	rec = append(rec, flags)
	rec = appendField(rec, "IBM VGA", 22, 0) // TInfoS pads with zeros, not spaces
	_, err = f.Write(rec)
	return err
}

// appendField appends s truncated or padded to exactly n bytes. SAUCE
// strings are CP437; anything outside printable ASCII becomes '?'.
func appendField(b []byte, s string, n int, pad byte) []byte {
	for i := 0; i < n; i++ {
		switch {
		case i >= len(s):
			b = append(b, pad)
		case s[i] < 0x20 || s[i] > 0x7E:
			b = append(b, '?')
		default:
			b = append(b, s[i])
		}
	}
	return b
}
