package mediaread

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// jpegOrientation returns the EXIF orientation (1 to 8) of a JPEG, read from
// its APP1 "Exif" segment, or 1 when it has none.
func jpegOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	for i := 2; i+4 <= len(b); {
		if b[i] != 0xFF {
			return 1
		}
		m := b[i+1]
		if m == 0xFF { // fill byte
			i++
			continue
		}
		i += 2
		switch {
		case m == 0x01 || m >= 0xD0 && m <= 0xD8: // markers without a length
			continue
		case m == 0xDA || m == 0xD9: // the image data starts: EXIF comes before it
			return 1
		}
		n := int(binary.BigEndian.Uint16(b[i:]))
		if n < 2 || i+n > len(b) {
			return 1
		}
		if seg := b[i+2 : i+n]; m == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			return tiffOrientation(seg[6:])
		}
		i += n
	}
	return 1
}

// tiffOrientation reads tag 0x0112 from IFD0 of the TIFF structure EXIF is.
func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	if bo.Uint16(t[2:]) != 42 {
		return 1
	}
	ifd := int64(bo.Uint32(t[4:]))
	if ifd < 8 || ifd+2 > int64(len(t)) {
		return 1
	}
	n := int(bo.Uint16(t[ifd:]))
	for k := 0; k < n; k++ {
		e := int(ifd) + 2 + 12*k
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 && bo.Uint16(t[e+2:]) == 3 { // SHORT
			if o := int(bo.Uint16(t[e+8:])); o >= 1 && o <= 8 {
				return o
			}
			return 1
		}
	}
	return 1
}

// gifFrames counts the frames of a GIF, stopping at 2, without decoding them;
// 0 means it does not parse.
func gifFrames(b []byte) int {
	if len(b) < 13 || !bytes.HasPrefix(b, []byte("GIF")) {
		return 0
	}
	i := 13
	if b[10]&0x80 != 0 {
		i += 3 << (b[10]&7 + 1)
	}
	frames := 0
	for i >= 0 && i < len(b) {
		switch b[i] {
		case 0x21: // extension: label, then sub-blocks
			i = skipSubBlocks(b, i+2)
		case 0x2C: // image descriptor
			if frames++; frames > 1 {
				return frames
			}
			if i+10 > len(b) {
				return 0
			}
			flags := b[i+9]
			i += 10
			if flags&0x80 != 0 {
				i += 3 << (flags&7 + 1)
			}
			i = skipSubBlocks(b, i+1) // after the LZW minimum code size
		case 0x3B: // trailer
			return frames
		default:
			return 0
		}
	}
	if i < 0 {
		return 0
	}
	return frames
}

func skipSubBlocks(b []byte, i int) int {
	for i < len(b) {
		n := int(b[i])
		i += 1 + n
		if n == 0 {
			return i
		}
	}
	return -1
}

// webpAnimated reports whether a WebP holds an animation, which
// golang.org/x/image/webp does not decode.
func webpAnimated(b []byte) bool {
	return len(b) >= 21 && string(b[:4]) == "RIFF" && string(b[8:16]) == "WEBPVP8X" && b[20]&0x02 != 0
}

// webpFirstFrame rebuilds the first frame of an animated WebP (a WhatsApp
// sticker, often) as a still WebP.
func webpFirstFrame(b []byte) ([]byte, error) {
	for i := 12; i+8 <= len(b); {
		n := int64(binary.LittleEndian.Uint32(b[i+4:]))
		body := int64(i + 8)
		if body+n > int64(len(b)) {
			break
		}
		if string(b[i:i+4]) == "ANMF" && n > 16 {
			hdr := b[body : body+16]
			frame := b[body+16 : body+n]
			var payload []byte
			if bytes.HasPrefix(frame, []byte("ALPH")) {
				// Alpha in its own chunk needs the extended header in front.
				vp8x := make([]byte, 18)
				copy(vp8x, "VP8X")
				binary.LittleEndian.PutUint32(vp8x[4:], 10)
				vp8x[8] = 0x10 // alpha
				copy(vp8x[12:18], hdr[6:12])
				payload = append(vp8x, frame...)
			} else {
				payload = frame
			}
			out := make([]byte, 12, 12+len(payload))
			copy(out, "RIFF")
			binary.LittleEndian.PutUint32(out[4:], uint32(4+len(payload)))
			copy(out[8:], "WEBP")
			return append(out, payload...), nil
		}
		i = int(body + n + n&1)
	}
	return nil, errors.New("the animated WebP has no readable frame")
}
