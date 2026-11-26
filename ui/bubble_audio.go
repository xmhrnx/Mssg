package ui

import (
	"time"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/store"
)

// audioBubble is a voice note with its waveform, or an audio file, with
// a play button that plays it in place.
func (a *App) audioBubble(m store.Message, corners giode.Corners) giode.Element {
	att := m.Attachment
	st := bubbleStyle(m.Outgoing, corners)
	st.Padding = giode.Inset{Top: 10, Bottom: 8, Left: 10, Right: 14}
	st.FlexDirection = giode.FlexDirectionColumn
	st.Gap = 4
	fg, track, btnBg := "theme.primary", giode.Mix("theme.muted", "theme.bubbleIn", 0.45), "theme.primary"
	btnFg := "#FFFFFF"
	if m.Outgoing {
		fg, track, btnBg, btnFg = "#FFFFFF", "#FFFFFF55", "#FFFFFF", "#6A4DFF"
	}
	playing := a.voice.current == m.ID && a.voice.playing()
	progress := float32(0)
	if a.voice.current == m.ID {
		progress = a.voice.progress()
	}
	var button giode.Element
	if needsDownload(m) {
		button = a.transferBadge(m, 44)
	} else {
		name := "play"
		if playing {
			name = "pause"
		}
		button = giode.Box(giode.Styles{Width: 44, Height: 44, BorderRadius: 999, Background: btnBg, Align: giode.AlignCenter, Justify: giode.JustifyCenter},
			icon(name, 20, btnFg))
	}
	dur := time.Duration(att.DurationMs) * time.Millisecond
	label := duration(dur)
	if a.voice.current == m.ID && a.voice.position() > 0 {
		label = duration(a.voice.position())
	}
	var middle giode.Element
	if len(att.Waveform) > 0 {
		middle = waveform(att.Waveform, progress, 170, 30, fg, track)
	} else {
		middle = giode.Stack(giode.Styles{Gap: 6, Width: 170},
			line(att.Name, fsSmall, st.Color, giode.FontWeightMedium),
			giode.Progress(progress, giode.Styles{Width: 170, Height: 4, Color: fg, Background: track}))
	}
	if att.DurationMs == 0 {
		label = fileSize(att.Size)
	}
	return giode.Box(st,
		giode.Row(giode.Styles{Gap: 10, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap}, button, middle),
		giode.Row(giode.Styles{Width: -1, Justify: giode.JustifySpaceBetween, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap, Padding: giode.Inset{Left: 54}},
			text(label, fsTiny, mutedOn(m.Outgoing)), metaLine(m, m.Outgoing)),
	)
}

func mutedOn(out bool) string {
	if out {
		return "#FFFFFFB8"
	}
	return "theme.muted"
}

// waveform draws loudness bars, the played part in the foreground color.
func waveform(levels []byte, progress float32, w, h int, fg, track string) giode.Element {
	return giode.Canvas(giode.Styles{Width: wpx(w), Height: hpx(h)}, func(p *giode.Painter) {
		bars := len(levels)
		gap := float32(2)
		bw := (p.Width() - gap*float32(bars-1)) / float32(bars)
		for i, l := range levels {
			bh := max(3, p.Height()*float32(l)/255)
			x := float32(i) * (bw + gap)
			color := track
			if float32(i)/float32(bars) < progress {
				color = fg
			}
			p.Rect(x, (p.Height()-bh)/2, bw, bh, bw/2, color)
		}
	})
}
