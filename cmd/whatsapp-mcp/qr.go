package main

import (
	"strings"

	"github.com/skip2/go-qrcode"
)

// qrBorder is the quiet zone around the code, in modules. The standard asks
// for 4; phones read WhatsApp's with 2, and a terminal has few rows to spare.
const qrBorder = 2

// qrModules is the QR code for text as rows of modules, true for dark, with
// the quiet zone around it.
func qrModules(text string) ([][]bool, error) {
	q, err := qrcode.New(text, qrcode.Low)
	if err != nil {
		return nil, err
	}
	q.DisableBorder = true
	inner := q.Bitmap()
	size := len(inner) + 2*qrBorder
	out := make([][]bool, size)
	for y := range out {
		out[y] = make([]bool, size)
		if y >= qrBorder && y < size-qrBorder {
			copy(out[y][qrBorder:], inner[y-qrBorder])
		}
	}
	return out, nil
}

// renderQR draws the QR code for text in a terminal, two rows of modules per
// line with half blocks, so it fits an 80-column window over SSH.
//
// With colours, every cell sets black and white itself, which reads the
// same on a dark terminal and a light one. Without them (NO_COLOR, a pipe,
// a log), the blocks are drawn in the terminal's own colour: the light
// modules, for the usual light text on a dark background, or the dark ones
// with invert, for dark text on a light background.
func renderQR(text string, colour, invert bool) (string, error) {
	m, err := qrModules(text)
	if err != nil {
		return "", err
	}
	if len(m)%2 == 1 {
		m = append(m, make([]bool, len(m))) // a light row to pair the last one with
	}
	var b strings.Builder
	for y := 0; y < len(m); y += 2 {
		for x := range m[y] {
			top, bottom := m[y][x], m[y+1][x]
			if colour {
				b.WriteString("\x1b[" + fg(top) + ";" + bg(bottom) + "m▀")
				continue
			}
			if !invert {
				top, bottom = !top, !bottom
			}
			switch {
			case top && bottom:
				b.WriteString("█")
			case top:
				b.WriteString("▀")
			case bottom:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		if colour {
			b.WriteString("\x1b[0m")
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

func fg(dark bool) string {
	if dark {
		return "30"
	}
	return "97"
}

func bg(dark bool) string {
	if dark {
		return "40"
	}
	return "107"
}
