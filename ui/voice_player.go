package ui

import (
	"time"

	"github.com/xmhrnx/giode/media"

	"github.com/xmhrnx/mssg/internal/store"
)

// voicePlayer plays one voice message or audio file at a time, in place.
type voicePlayer struct {
	a        *App
	current  string
	p        *media.Player
	stopTick chan struct{}
}

func newVoicePlayer(a *App) *voicePlayer { return &voicePlayer{a: a} }

// toggle plays a message's audio, or pauses it if it's playing.
func (v *voicePlayer) toggle(m store.Message) {
	if v.current == m.ID && v.p != nil {
		if v.p.State() == media.Ended {
			v.p.Seek(0)
		}
		v.p.Toggle()
		return
	}
	v.stop()
	id := m.ID
	p, err := media.Open(v.a.ctx(), v.a.g, m.Attachment.Path, media.Options{
		Autoplay: true,
		OnEnded:  func() { v.a.g.Post(func() { v.ended(id) }) },
		OnError:  func(err error) { v.a.g.Post(func() { v.a.toastError(err); v.stop() }) },
	})
	if err != nil {
		v.a.toastError(err)
		return
	}
	v.current, v.p = id, p
	v.stopTick = make(chan struct{})
	go ticker(v.a.g, 100*time.Millisecond, v.stopTick)
}

func (v *voicePlayer) ended(id string) {
	if v.current == id {
		v.stop()
	}
}

func (v *voicePlayer) stop() {
	if v.p != nil {
		v.p.Close()
		v.p = nil
	}
	if v.stopTick != nil {
		close(v.stopTick)
		v.stopTick = nil
	}
	v.current = ""
}

func (v *voicePlayer) playing() bool {
	return v.p != nil && (v.p.State() == media.Playing || v.p.State() == media.Loading)
}

func (v *voicePlayer) position() time.Duration {
	if v.p == nil {
		return 0
	}
	return v.p.Position()
}

func (v *voicePlayer) progress() float32 {
	if v.p == nil || v.p.Duration() <= 0 {
		return 0
	}
	return min(1, float32(v.p.Position())/float32(v.p.Duration()))
}
