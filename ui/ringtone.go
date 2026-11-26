package ui

import (
	"math"
	"time"

	"github.com/xmhrnx/giode"
)

// Tones made once: a two-note ring for incoming calls and a soft
// ringback while calling out.
var (
	ringTone     = tone([]float64{880, 1320}, 0.9, 0.35)
	ringbackTone = tone([]float64{440, 480}, 1.2, 0.12)
)

// tone mixes frequencies into a WAV with soft attack and release.
func tone(freqs []float64, seconds, volume float64) []byte {
	const rate = 22050
	n := int(seconds * rate)
	samples := make([]float32, n)
	for i := range samples {
		t := float64(i) / rate
		env := math.Min(1, math.Min(t*20, (seconds-t)*8))
		var v float64
		for _, f := range freqs {
			v += math.Sin(2 * math.Pi * f * t)
		}
		// A gentle pulse, like a phone's ring.
		pulse := 0.6 + 0.4*math.Sin(2*math.Pi*12*t)
		samples[i] = float32(volume * env * pulse * v / float64(len(freqs)))
	}
	return giode.EncodeWAV(samples, rate)
}

func (m *callManager) startRinging(incoming bool) {
	m.stopRinging()
	stop := make(chan struct{})
	m.ringing = stop
	sound, every := ringbackTone, 3*time.Second
	if incoming {
		sound, every = ringTone, 2*time.Second
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			// Phones without a system call screen still ring; a sound
			// that can't play leaves the screen to do the ringing.
			_ = m.a.g.PlaySound(sound)
			if incoming {
				giode.Haptic(giode.HapticHeavy)
			}
			select {
			case <-stop:
				return
			case <-t.C:
			}
		}
	}()
}

func (m *callManager) stopRinging() {
	if m.ringing != nil {
		close(m.ringing)
		m.ringing = nil
	}
}
