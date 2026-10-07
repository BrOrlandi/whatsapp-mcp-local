package mediaread

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"golang.org/x/image/bmp"
)

func gradient(w, h int) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	return m
}

func encodeJPEG(t *testing.T, m image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, m, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func encodePNG(t *testing.T, m image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, m); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decoded(t *testing.T, img Image) image.Image {
	t.Helper()
	m, format, err := image.Decode(bytes.NewReader(img.Data))
	if err != nil {
		t.Fatal(err)
	}
	if "image/"+format != img.MIME {
		t.Fatalf("data is %s, MIME says %s", format, img.MIME)
	}
	if b := m.Bounds(); b.Dx() != img.Width || b.Dy() != img.Height {
		t.Fatalf("data is %dx%d, Image says %dx%d", b.Dx(), b.Dy(), img.Width, img.Height)
	}
	return m
}

func TestReadImageDownscales(t *testing.T) {
	path := writeTemp(t, "big.jpg", encodeJPEG(t, gradient(3000, 2000)))
	img, err := ReadImage(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if img.MIME != "image/jpeg" || img.Width != 1568 || img.Height != 1045 || !img.Resized ||
		img.OriginalWidth != 3000 || img.OriginalHeight != 2000 {
		t.Fatalf("got %s %dx%d from %dx%d, resized %v", img.MIME, img.Width, img.Height, img.OriginalWidth, img.OriginalHeight, img.Resized)
	}
	decoded(t, img)

	img, err = ReadImage(path, 500)
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != 500 || img.Height != 333 {
		t.Fatalf("maxEdge 500: got %dx%d", img.Width, img.Height)
	}
}

func TestReadImageSmallJPEGUnchanged(t *testing.T) {
	data := encodeJPEG(t, gradient(200, 100))
	img, err := ReadImage(writeTemp(t, "small.jpg", data), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(img.Data, data) || img.Resized || img.MIME != "image/jpeg" || img.Width != 200 || img.Height != 100 {
		t.Fatalf("small JPEG changed: %s %dx%d resized %v", img.MIME, img.Width, img.Height, img.Resized)
	}
}

func TestReadImageAlphaStaysPNG(t *testing.T) {
	m := image.NewNRGBA(image.Rect(0, 0, 2000, 1000))
	for i := 0; i < len(m.Pix); i += 4 {
		m.Pix[i], m.Pix[i+3] = 200, 128
	}
	img, err := ReadImage(writeTemp(t, "alpha.png", encodePNG(t, m)), 0)
	if err != nil {
		t.Fatal(err)
	}
	if img.MIME != "image/png" || img.Width != 1568 || img.Height != 784 {
		t.Fatalf("got %s %dx%d", img.MIME, img.Width, img.Height)
	}
	if _, _, _, a := decoded(t, img).At(700, 400).RGBA(); a == 0xffff {
		t.Fatal("transparency lost")
	}
}

// withOrientation puts an APP1 Exif segment holding orientation o right
// after the JPEG's SOI marker.
func withOrientation(jpg []byte, o uint16) []byte {
	tiff := []byte("MM\x00\x2a\x00\x00\x00\x08")
	tiff = binary.BigEndian.AppendUint16(tiff, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, 0x0112)
	tiff = binary.BigEndian.AppendUint16(tiff, 3)
	tiff = binary.BigEndian.AppendUint32(tiff, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, o)
	tiff = append(tiff, 0, 0, 0, 0, 0, 0)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xFF, 0xE1}
	seg = binary.BigEndian.AppendUint16(seg, uint16(len(payload)+2))
	seg = append(seg, payload...)
	out := append([]byte{0xFF, 0xD8}, seg...)
	return append(out, jpg[2:]...)
}

func TestReadImageEXIFOrientation(t *testing.T) {
	// Left half red, right half blue; orientation 6 turns it clockwise, so
	// red ends on top.
	m := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			c := color.RGBA{255, 0, 0, 255}
			if x >= 20 {
				c = color.RGBA{0, 0, 255, 255}
			}
			m.Set(x, y, c)
		}
	}
	data := withOrientation(encodeJPEG(t, m), 6)
	if o := jpegOrientation(data); o != 6 {
		t.Fatalf("jpegOrientation = %d, want 6", o)
	}
	img, err := ReadImage(writeTemp(t, "rotated.jpg", data), 0)
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != 20 || img.Height != 40 || img.OriginalWidth != 20 || img.OriginalHeight != 40 || img.Resized {
		t.Fatalf("got %dx%d from %dx%d, resized %v", img.Width, img.Height, img.OriginalWidth, img.OriginalHeight, img.Resized)
	}
	out := decoded(t, img)
	if r, _, b, _ := out.At(10, 5).RGBA(); r < 0xc000 || b > 0x4000 {
		t.Fatalf("top is not red: r=%x b=%x", r, b)
	}
	if r, _, b, _ := out.At(10, 35).RGBA(); b < 0xc000 || r > 0x4000 {
		t.Fatalf("bottom is not blue: r=%x b=%x", r, b)
	}
}

func TestReadImageGIF(t *testing.T) {
	frame := func(i uint8) *image.Paletted {
		p := image.NewPaletted(image.Rect(0, 0, 50, 50), palette.Plan9)
		for j := range p.Pix {
			p.Pix[j] = i
		}
		return p
	}
	var still bytes.Buffer
	if err := gif.Encode(&still, frame(10), nil); err != nil {
		t.Fatal(err)
	}
	img, err := ReadImage(writeTemp(t, "still.gif", still.Bytes()), 0)
	if err != nil {
		t.Fatal(err)
	}
	if img.MIME != "image/gif" || !bytes.Equal(img.Data, still.Bytes()) {
		t.Fatalf("still GIF changed: %s", img.MIME)
	}

	var anim bytes.Buffer
	if err := gif.EncodeAll(&anim, &gif.GIF{Image: []*image.Paletted{frame(10), frame(200)}, Delay: []int{10, 10}}); err != nil {
		t.Fatal(err)
	}
	if n := gifFrames(anim.Bytes()); n != 2 {
		t.Fatalf("gifFrames = %d, want 2", n)
	}
	img, err = ReadImage(writeTemp(t, "anim.gif", anim.Bytes()), 0)
	if err != nil {
		t.Fatal(err)
	}
	if img.MIME != "image/jpeg" || img.Width != 50 || img.Height != 50 {
		t.Fatalf("animated GIF: got %s %dx%d", img.MIME, img.Width, img.Height)
	}
	decoded(t, img)
}

// Two 8x8 frames made with img2webp: the first one red, its right half at
// half opacity; lossy (ALPH + VP8 chunks) and lossless (VP8L).
const (
	animatedLossyWebP    = "52494646e400000057454250565038580a00000012000000070000070000414e494d06000000ffffffff0000414e4d465800000000000000000007000007000064000002414c50480c000000010ff0c0ff888850f888fe07565038202c0000009001009d012a0800080002c04c25a00274ba00039800feee431fee6c738b7057ff6d0fff5a1ffeb43fe94000414e4d465800000000000000000007000007000064000002414c50480a000000010750c088084444ff03565038202e000000b001009d012a0800080002c04c25a00274010efe02ec00fef8472a3ff6d0ffff6d0ffff6d0ffa8ef06c914e48000"
	animatedLosslessWebP = "524946468600000057454250565038580a00000012000000070000070000414e494d06000000ffffffff0000414e4d462a000000000000000000070000070000640000025650384c110000002f07c001100f10f31fff038c411f22fa1f00414e4d4628000000000000000000070000070000640000025650384c0f0000002f07c001100710d1ff020622a2ff0100"
)

func TestReadImageAnimatedWebP(t *testing.T) {
	for name, h := range map[string]string{"lossy": animatedLossyWebP, "lossless": animatedLosslessWebP} {
		data, _ := hex.DecodeString(h)
		img, err := ReadImage(writeTemp(t, name+".webp", data), 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if img.MIME != "image/png" || img.Width != 8 || img.Height != 8 {
			t.Fatalf("%s: got %s %dx%d", name, img.MIME, img.Width, img.Height)
		}
		out := decoded(t, img)
		if r, g, b, a := out.At(1, 1).RGBA(); a != 0xffff || r < 0xc000 || g > 0x4000 || b > 0x4000 {
			t.Fatalf("%s: first frame's left is not opaque red: %x %x %x %x", name, r, g, b, a)
		}
		if _, _, _, a := out.At(6, 1).RGBA(); a > 0xa000 || a < 0x6000 {
			t.Fatalf("%s: first frame's right is not half transparent: a=%x", name, a)
		}
	}
}

func TestReadImageBMP(t *testing.T) {
	var buf bytes.Buffer
	if err := bmp.Encode(&buf, gradient(30, 20)); err != nil {
		t.Fatal(err)
	}
	img, err := ReadImage(writeTemp(t, "pic.bmp", buf.Bytes()), 0)
	if err != nil {
		t.Fatal(err)
	}
	if img.MIME != "image/jpeg" || img.Width != 30 || img.Height != 20 || img.Resized {
		t.Fatalf("got %s %dx%d", img.MIME, img.Width, img.Height)
	}
}

func TestReadImageRefusesHugeBeforeDecoding(t *testing.T) {
	// A PNG header claiming 10000x10000 pixels and no image data.
	ihdr := binary.BigEndian.AppendUint32(nil, 10000)
	ihdr = binary.BigEndian.AppendUint32(ihdr, 10000)
	ihdr = append(ihdr, 8, 6, 0, 0, 0)
	chunk := binary.BigEndian.AppendUint32(nil, uint32(len(ihdr)))
	chunk = append(chunk, "IHDR"...)
	chunk = append(chunk, ihdr...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
	data := append([]byte("\x89PNG\r\n\x1a\n"), chunk...)
	_, err := ReadImage(writeTemp(t, "huge.png", data), 0)
	if err == nil || !strings.Contains(err.Error(), "64 million pixels") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadImageUnsupported(t *testing.T) {
	_, err := ReadImage(writeTemp(t, "note.txt", []byte("not a picture at all")), 0)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

// The parsers take any bytes from a chat: none may panic on a cut file.
func TestImageParsersSurviveTruncation(t *testing.T) {
	jpg := withOrientation(encodeJPEG(t, gradient(16, 16)), 6)
	var anim bytes.Buffer
	p := image.NewPaletted(image.Rect(0, 0, 4, 4), palette.Plan9)
	_ = gif.EncodeAll(&anim, &gif.GIF{Image: []*image.Paletted{p, p}, Delay: []int{1, 1}})
	webp, _ := hex.DecodeString(animatedLossyWebP)
	for i := range jpg {
		jpegOrientation(jpg[:i])
	}
	for i := range anim.Bytes() {
		gifFrames(anim.Bytes()[:i])
	}
	for i := range webp {
		_, _ = webpFirstFrame(webp[:i])
	}
}
