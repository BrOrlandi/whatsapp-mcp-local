package main

import (
	"regexp"
	"strings"
	"testing"
)

// A WhatsApp pairing code is long: four comma-separated parts after "2@".
const sampleQR = "2@Qm9vZ2llV29vZ2llQm9vZ2llV29vZ2llQm9vZ2llV29vZ2llQm9vZ2llV29vZ2llQm9vZ2llV29vZ2llQm9vZ2llV29vZ2llQm9vZ2llV29vZ2ll," +
	"pZ8Gv1mH3o0nA9hQ2d0eJ7m8m2hV6xYV0aZ2YQq0uWQ=,Wk5mA0b1c2d3e4f5g6h7i8j9k0l1m2n3o4p5q6r7s8t=,Q1w2E3r4T5y6U7i8O9p0A1s2D3f4G5h6J7k8L9z0X1c="

// readBack turns a drawing back into modules, to compare with the code.
func readBack(t *testing.T, drawn string, colour, invert bool) [][]bool {
	t.Helper()
	cell := regexp.MustCompile(`\x1b\[(\d+);(\d+)m▀`)
	var rows [][]bool
	for _, line := range strings.Split(strings.TrimSuffix(drawn, "\n"), "\n") {
		var top, bottom []bool
		if colour {
			for _, m := range cell.FindAllStringSubmatch(line, -1) {
				top, bottom = append(top, m[1] == "30"), append(bottom, m[2] == "40")
			}
		} else {
			for _, r := range line {
				lit := map[rune][2]bool{'█': {true, true}, '▀': {true, false}, '▄': {false, true}, ' ': {false, false}}[r]
				if invert {
					top, bottom = append(top, lit[0]), append(bottom, lit[1])
				} else {
					top, bottom = append(top, !lit[0]), append(bottom, !lit[1])
				}
			}
		}
		rows = append(rows, top, bottom)
	}
	return rows
}

func TestRenderQRDrawsEveryModule(t *testing.T) {
	want, err := qrModules(sampleQR)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name           string
		colour, invert bool
	}{{"colour", true, false}, {"dark terminal", false, false}, {"light terminal", false, true}} {
		drawn, err := renderQR(sampleQR, c.colour, c.invert)
		if err != nil {
			t.Fatal(err)
		}
		got := readBack(t, drawn, c.colour, c.invert)
		if len(got) < len(want) {
			t.Fatalf("%s: %d rows, want %d", c.name, len(got), len(want))
		}
		for y := range want {
			if len(got[y]) != len(want[y]) {
				t.Fatalf("%s: row %d has %d modules, want %d", c.name, y, len(got[y]), len(want[y]))
			}
			for x := range want[y] {
				if got[y][x] != want[y][x] {
					t.Fatalf("%s: module %d,%d differs", c.name, x, y)
				}
			}
		}
		// The padding row, when the size is odd, is light.
		for y := len(want); y < len(got); y++ {
			for _, dark := range got[y] {
				if dark {
					t.Fatalf("%s: the padding row has a dark module", c.name)
				}
			}
		}
	}
}

// The code has to fit a common 80-column terminal, quiet zone included.
func TestRenderQRFitsATerminal(t *testing.T) {
	drawn, err := renderQR(sampleQR, false, false)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(drawn, "\n"), "\n")
	if w := len([]rune(lines[0])); w > 80 {
		t.Fatalf("%d columns wide", w)
	}
	if len(lines) > 40 {
		t.Fatalf("%d lines tall", len(lines))
	}
	m, _ := qrModules(sampleQR)
	for _, dark := range m[0] {
		if dark {
			t.Fatal("the quiet zone has a dark module")
		}
	}
}
