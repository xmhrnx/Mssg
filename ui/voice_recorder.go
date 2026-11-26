package ui

import (
	"bytes"
	"sync"
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/audio"

	"github.com/xmhrnx/mssg/internal/mediakit"
	"github.com/xmhrnx/mssg/internal/messenger"
	"github.com/xmhrnx/mssg/internal/store"
)

const (
	voiceRate   = 16000
	voiceMax    = 10 * time.Minute
	voiceLevels = 48
)

// voiceRecorder records a voice message from the microphone.
type voiceRecorder struct {
	a        *App
	peer     string
	on       bool
	in       *audio.Input
	started  time.Time
	gen      int
	stopTick chan struct{}

	mu      sync.Mutex
	samples []int16
	recent  []float32

	discard *giode.IconButtonElement
	send    *giode.IconButtonElement
	pulse   *giode.KeyframeAnimation
}

func newVoiceRecorder(a *App) *voiceRecorder {
	r := &voiceRecorder{a: a}
	r.discard = iconButton("trash-2", 22, "Discard recording", r.cancel)
	r.send = giode.IconButton(icon("send-horizontal", 20, "#FFFFFF"), giode.Styles{
		Width: 44, Height: 44, BorderRadius: 999, BackgroundGradient: gradientMine,
	}).Label("Send voice message").OnClick(func() { r.finish(true) })
	r.pulse = giode.Keyframes(1100*time.Millisecond,
		giode.Keyframe{At: 0, Styles: giode.Styles{Opacity: 1}},
		giode.Keyframe{At: 0.5, Styles: giode.Styles{Opacity: 0.25}},
		giode.Keyframe{At: 1, Styles: giode.Styles{Opacity: 1}},
	).Repeat(0)
	return r
}

func (r *voiceRecorder) active() bool { return r.on }

func (r *voiceRecorder) start(peer string) {
	if r.on {
		return
	}
	r.on, r.peer, r.started = true, peer, time.Now()
	r.gen++
	gen := r.gen
	r.mu.Lock()
	r.samples, r.recent = r.samples[:0], nil
	r.mu.Unlock()
	r.a.voice.stop()
	go func() {
		in, err := audio.OpenInput(r.a.ctx(), r.a.g, audio.Options{SampleRate: voiceRate, Voice: true}, r.onData)
		r.a.g.Post(func() {
			if err != nil {
				r.on = false
				r.a.toastError(err)
				return
			}
			if gen != r.gen || !r.on {
				in.Close()
				return
			}
			r.in = in
			r.started = time.Now()
			r.stopTick = make(chan struct{})
			go ticker(r.a.g, 80*time.Millisecond, r.stopTick)
		})
	}()
}

func (r *voiceRecorder) onData(s []int16) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.samples) >= voiceRate*int(voiceMax/time.Second) {
		return
	}
	r.samples = append(r.samples, s...)
	r.recent = append(r.recent, float32(audio.Level(s)))
	if len(r.recent) > voiceLevels {
		r.recent = r.recent[len(r.recent)-voiceLevels:]
	}
}

func (r *voiceRecorder) cancel() { r.finish(false) }

// finish stops recording and, if asked, sends what was recorded.
func (r *voiceRecorder) finish(send bool) {
	if !r.on {
		return
	}
	r.on = false
	r.gen++
	if r.in != nil {
		r.in.Close()
		r.in = nil
	}
	if r.stopTick != nil {
		close(r.stopTick)
		r.stopTick = nil
	}
	r.mu.Lock()
	samples := append([]int16(nil), r.samples...)
	r.mu.Unlock()
	if !send || len(samples) < voiceRate/2 {
		return
	}
	peer := r.peer
	go func() {
		wav := mediakit.WAV(samples, voiceRate)
		meta := messenger.FileMeta{
			Name: "Voice message.wav", MIME: "audio/wav", Kind: store.KindAudio,
			DurationMs: int64(len(samples)) * 1000 / voiceRate,
			Waveform:   mediakit.Waveform(samples, 40),
		}
		if _, err := r.a.svc.SendFile(peer, bytes.NewReader(wav), meta); err != nil {
			r.a.g.Post(func() { r.a.toastError(err) })
		}
	}()
}

func (r *voiceRecorder) view() giode.Element {
	r.mu.Lock()
	levels := append([]float32(nil), r.recent...)
	r.mu.Unlock()
	meter := giode.Canvas(giode.Styles{Height: 28, FlexGrow: 1}, func(p *giode.Painter) {
		bw, gap := float32(3), float32(2)
		n := int(p.Width() / (bw + gap))
		for i := 0; i < n; i++ {
			l := float32(0)
			if j := len(levels) - n + i; j >= 0 && j < len(levels) {
				l = levels[j]
			}
			h := max(3, p.Height()*min(1, l*2.2))
			p.Rect(float32(i)*(bw+gap), (p.Height()-h)/2, bw, h, 1.5, "theme.primary")
		}
	})
	return giode.Row(giode.Styles{
		Width: -1, Gap: 12, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap,
		Padding: giode.Inset{Top: 10, Bottom: 12, Left: 8, Right: 10}, Background: "theme.background",
	},
		r.discard,
		giode.Box(giode.Styles{Width: 10, Height: 10, BorderRadius: 999, Background: "theme.danger"}).Animation(r.pulse),
		text(duration(time.Since(r.started)), fsBody, "theme.text", giode.FontWeightMedium),
		meter,
		r.send,
	)
}
