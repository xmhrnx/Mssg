package mediakit

import (
	"bytes"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"testing"

	"github.com/xmhrnx/mssg/internal/store"
)

func TestKinds(t *testing.T) {
	for mt, want := range map[string]store.Kind{
		"image/gif": store.KindGIF, "image/jpeg": store.KindImage, "video/mp4": store.KindVideo,
		"audio/wav": store.KindAudio, "application/pdf": store.KindFile,
	} {
		if got := KindOf(mt); got != want {
			t.Errorf("%s: %s", mt, got)
		}
	}
	if got := MIMEOf("clip.MOV", ""); got != "video/quicktime" {
		t.Errorf("mov: %s", got)
	}
}

func TestDecodeGIF(t *testing.T) {
	var g gif.GIF
	for i := range 3 {
		p := image.NewPaletted(image.Rect(0, 0, 40, 20), palette.Plan9)
		p.Set(i, 0, color.White)
		g.Image = append(g.Image, p)
		g.Delay = append(g.Delay, 0)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &g); err != nil {
		t.Fatal(err)
	}
	fr, err := DecodeGIF(&buf, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(fr.Images) != 3 || fr.Images[0].Bounds().Dx() != 20 || fr.Delays[0] != 10 {
		t.Fatalf("frames %d, width %d, delay %d", len(fr.Images), fr.Images[0].Bounds().Dx(), fr.Delays[0])
	}
}

func TestWAVAndWaveform(t *testing.T) {
	samples := make([]int16, 16000)
	for i := 8000; i < 16000; i++ {
		samples[i] = 12000
	}
	if wav := WAV(samples, 16000); len(wav) != 44+32000 || string(wav[:4]) != "RIFF" {
		t.Fatalf("wav of %d bytes", len(wav))
	}
	w := Waveform(samples, 10)
	if w[0] >= w[9] {
		t.Fatalf("silence %d should be quieter than tone %d", w[0], w[9])
	}
}
