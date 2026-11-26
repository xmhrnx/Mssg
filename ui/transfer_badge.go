package ui

import (
	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/store"
)

// transferBadge is the round control over media that isn't here yet:
// download, progress, or retry.
func (a *App) transferBadge(m store.Message, size int) giode.Element {
	att := m.Attachment
	var inner giode.Element
	switch {
	case a.svc.Downloading(m.ID):
		frac := float32(0)
		if att.Size > 0 {
			frac = float32(att.Received) / float32(att.Size)
		}
		inner = progressRing(size, frac, "#FFFFFF")
	case att.State == store.TransferFailed:
		inner = icon("refresh-cw", size/2, "#FFFFFF")
	default:
		inner = icon("download", size/2, "#FFFFFF")
	}
	return giode.Box(giode.Styles{
		Width: wpx(size), Height: hpx(size), BorderRadius: 999, Background: "#000000AA",
		Align: giode.AlignCenter, Justify: giode.JustifyCenter,
	}, inner)
}

// progressRing draws a fraction as an arc.
func progressRing(size int, frac float32, color string) giode.Element {
	return giode.Canvas(giode.Styles{Width: wpx(size), Height: hpx(size)}, func(p *giode.Painter) {
		c := p.Width() / 2
		r := c - 5
		p.Arc(c, c, r, 3, 0, 360, "#FFFFFF33")
		p.Arc(c, c, r, 3, 0, 360*max(frac, 0.03), color)
	})
}

// needsDownload reports an incoming attachment not on this device yet.
func needsDownload(m store.Message) bool {
	return m.Attachment != nil && !m.Outgoing && m.Attachment.State != store.TransferDone
}
