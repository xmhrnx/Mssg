// Package mediakit classifies attachments and makes their previews:
// kinds from MIME types, JPEG thumbnails, image sizes, voice note WAV
// files and their waveforms.
package mediakit

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	_ "image/png" // decoders for thumbnails
	"io"
	"math"
	"mime"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/xmhrnx/mssg/internal/store"
)

// MIMEOf guesses a MIME type from a name when the picker gave none.
func MIMEOf(name, given string) string {
	if given != "" && given != "application/octet-stream" {
		return given
	}
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); t != "" {
		if i := strings.IndexByte(t, ';'); i > 0 {
			t = t[:i]
		}
		return t
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".m4a":
		return "audio/mp4"
	case ".opus":
		return "audio/ogg"
	case ".heic":
		return "image/heic"
	case ".mov":
		return "video/quicktime"
	case ".mkv":
		return "video/x-matroska"
	}
	return "application/octet-stream"
}

// KindOf picks the message kind for a MIME type.
func KindOf(mimeType string) store.Kind {
	switch {
	case mimeType == "image/gif":
		return store.KindGIF
	case mimeType == "image/jpeg", mimeType == "image/png", mimeType == "image/webp":
		return store.KindImage
	case strings.HasPrefix(mimeType, "video/"):
		return store.KindVideo
	case strings.HasPrefix(mimeType, "audio/"):
		return store.KindAudio
	}
	return store.KindFile
}

// ThumbSide is the long side of thumbnails sent with messages.
const ThumbSide = 360

// Preview decodes an image (the first frame of a GIF) and returns its
// size and a small JPEG thumbnail.
func Preview(r io.Reader) (w, h int, thumb []byte, err error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return 0, 0, nil, err
	}
	b := img.Bounds()
	thumb, err = Thumbnail(img, ThumbSide, 70)
	return b.Dx(), b.Dy(), thumb, err
}

// Thumbnail scales img to fit side and encodes it as JPEG.
func Thumbnail(img image.Image, side, quality int) ([]byte, error) {
	small := Fit(img, side)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, small, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Fit scales img down so its long side is at most side.
func Fit(img image.Image, side int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= side && h <= side {
		return img
	}
	if w >= h {
		h, w = max(1, h*side/w), side
	} else {
		w, h = max(1, w*side/h), side
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// Frames is a decoded animated GIF, each frame composited in full.
type Frames struct {
	Images []*image.RGBA
	Delays []int // hundredths of a second
	Loop   bool
}

// DecodeGIF decodes and composites every frame of a GIF, scaled down to
// fit side to bound memory.
func DecodeGIF(r io.Reader, side int) (*Frames, error) {
	g, err := gif.DecodeAll(r)
	if err != nil {
		return nil, err
	}
	bounds := image.Rect(0, 0, g.Config.Width, g.Config.Height)
	if bounds.Empty() && len(g.Image) > 0 {
		bounds = g.Image[0].Bounds()
	}
	canvas := image.NewRGBA(bounds)
	f := &Frames{Loop: g.LoopCount >= 0}
	for i, frame := range g.Image {
		var restore *image.RGBA
		disposal := byte(0)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		if disposal == gif.DisposalPrevious {
			restore = image.NewRGBA(bounds)
			draw.Draw(restore, bounds, canvas, bounds.Min, draw.Src)
		}
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		scaled := Fit(canvas, side)
		out := image.NewRGBA(scaled.Bounds())
		draw.Draw(out, out.Bounds(), scaled, scaled.Bounds().Min, draw.Src)
		f.Images = append(f.Images, out)
		d := 10
		if i < len(g.Delay) && g.Delay[i] > 1 {
			d = g.Delay[i]
		}
		f.Delays = append(f.Delays, d)
		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvas, frame.Bounds(), image.Transparent, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			canvas = restore
		}
	}
	return f, nil
}

// WAV encodes 16-bit mono PCM as a WAV file.
func WAV(samples []int16, rate int) []byte {
	var buf bytes.Buffer
	dataLen := len(samples) * 2
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+dataLen))
	buf.WriteString("WAVEfmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // mono
	binary.Write(&buf, binary.LittleEndian, uint32(rate))
	binary.Write(&buf, binary.LittleEndian, uint32(rate*2))
	binary.Write(&buf, binary.LittleEndian, uint16(2))
	binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(dataLen))
	binary.Write(&buf, binary.LittleEndian, samples)
	return buf.Bytes()
}

// Waveform reduces samples to n loudness bars from 0 to 255.
func Waveform(samples []int16, n int) []byte {
	out := make([]byte, n)
	if len(samples) == 0 {
		return out
	}
	per := max(1, len(samples)/n)
	peak := 1.0
	levels := make([]float64, n)
	for i := range n {
		start := i * per
		if start >= len(samples) {
			break
		}
		end := min(len(samples), start+per)
		var sum float64
		for _, s := range samples[start:end] {
			sum += float64(s) * float64(s)
		}
		rms := sum / float64(end-start)
		levels[i] = rms
		peak = max(peak, rms)
	}
	for i, l := range levels {
		// Square root twice: loudness reads closer to how it sounds.
		v := math.Sqrt(math.Sqrt(l / peak))
		out[i] = byte(min(255, 20+v*235))
	}
	return out
}
