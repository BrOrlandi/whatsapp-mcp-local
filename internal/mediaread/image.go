package mediaread

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// DefaultMaxEdge is the long edge, in pixels, vision models read images at.
const DefaultMaxEdge = 1568

const (
	maxPixels    = 64_000_000
	maxImageFile = 128 << 20
	// passThrough is the largest file handed over as it is, when it already
	// fits: re-encoding it would only lose quality.
	passThrough = 1 << 20
	jpegQuality = 85
)

// Image is a picture ready to hand to a vision model.
type Image struct {
	Data []byte
	// MIME is image/jpeg or image/png, or the file's own type (image/gif,
	// image/webp) when it is returned unchanged.
	MIME                          string
	Width, Height                 int // of Data
	OriginalWidth, OriginalHeight int
	Resized                       bool // Data is scaled down from the original
}

// ReadImage decodes an image file (JPEG, PNG, GIF, WebP, TIFF, BMP), applies
// the EXIF orientation of a JPEG, scales it so its long edge is at most maxEdge
// (DefaultMaxEdge when maxEdge <= 0), drops metadata, and encodes it as JPEG
// (quality 85), or PNG when it has transparency. An animation keeps its first
// frame. A JPEG/PNG/GIF/WebP file that already fits, is at most 1 MiB and needs
// no rotation is returned unchanged (Resized false). Images over 64 million
// pixels are refused before they are decoded.
func ReadImage(path string, maxEdge int) (Image, error) {
	if maxEdge <= 0 {
		maxEdge = DefaultMaxEdge
	}
	data, err := readFile(path, maxImageFile)
	if err != nil {
		return Image{}, err
	}
	return decodeImage(data, maxEdge)
}

func decodeImage(data []byte, maxEdge int) (Image, error) {
	src := data
	animated := webpAnimated(data)
	if animated {
		frame, err := webpFirstFrame(data)
		if err != nil {
			return Image{}, err
		}
		src = frame
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		if errors.Is(err, image.ErrFormat) {
			return Image{}, fmt.Errorf("%w: not a JPEG, PNG, GIF, WebP, TIFF or BMP image", ErrUnsupported)
		}
		return Image{}, fmt.Errorf("reading the image: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return Image{}, errors.New("the image has no pixels")
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return Image{}, fmt.Errorf("the image is %dx%d, more than the 64 million pixels this reads", cfg.Width, cfg.Height)
	}
	orientation := 1
	if format == "jpeg" {
		orientation = jpegOrientation(data)
	}
	w, h := cfg.Width, cfg.Height
	if orientation >= 5 {
		w, h = h, w
	}
	if !animated && orientation == 1 && max(w, h) <= maxEdge && len(data) <= passThrough && unchangedOK(format, cfg, data) {
		return Image{Data: data, MIME: "image/" + format, Width: w, Height: h, OriginalWidth: w, OriginalHeight: h}, nil
	}

	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return Image{}, fmt.Errorf("decoding the %s image: %w", format, err)
	}
	img = fit(img, maxEdge)
	if orientation > 1 {
		img = orient(toRGBA(img), orientation)
	}
	out := Image{
		Width: img.Bounds().Dx(), Height: img.Bounds().Dy(),
		OriginalWidth: w, OriginalHeight: h,
	}
	out.Resized = out.Width != w || out.Height != h
	var buf bytes.Buffer
	if o, ok := img.(interface{ Opaque() bool }); ok && !o.Opaque() {
		out.MIME = "image/png"
		err = png.Encode(&buf, img)
	} else {
		out.MIME = "image/jpeg"
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality})
	}
	if err != nil {
		return Image{}, fmt.Errorf("encoding the image: %w", err)
	}
	out.Data = buf.Bytes()
	return out, nil
}

// unchangedOK says whether a file that already fits can go as it is: not an
// animation, and a JPEG every model decodes (not CMYK).
func unchangedOK(format string, cfg image.Config, data []byte) bool {
	switch format {
	case "jpeg":
		return cfg.ColorModel == color.YCbCrModel || cfg.ColorModel == color.GrayModel
	case "png", "webp":
		return true
	case "gif":
		return gifFrames(data) == 1
	}
	return false
}

func readFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > limit {
		return nil, fmt.Errorf("the file is %d MiB, more than the %d MiB this reads", st.Size()>>20, limit>>20)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("the file is more than the %d MiB this reads", limit>>20)
	}
	return data, nil
}

// fit scales img down so its long edge is at most maxEdge.
func fit(img image.Image, maxEdge int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if max(w, h) <= maxEdge {
		return img
	}
	scale := float64(maxEdge) / float64(max(w, h))
	nw := max(1, int(float64(w)*scale+0.5))
	nh := max(1, int(float64(h)*scale+0.5))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

func toRGBA(img image.Image) *image.RGBA {
	if m, ok := img.(*image.RGBA); ok && m.Rect.Min == (image.Point{}) {
		return m
	}
	b := img.Bounds()
	m := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(m, m.Bounds(), img, b.Min, draw.Src)
	return m
}

// orient turns an image stored with EXIF orientation o (2 to 8) upright.
func orient(m *image.RGBA, o int) *image.RGBA {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	ow, oh := w, h
	if o >= 5 {
		ow, oh = h, w
	}
	out := image.NewRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			default:
				dx, dy = x, y
			}
			s := y*m.Stride + x*4
			d := dy*out.Stride + dx*4
			copy(out.Pix[d:d+4], m.Pix[s:s+4])
		}
	}
	return out
}
