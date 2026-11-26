package ui

import (
	"time"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/store"
)

// videoBubble shows a video's poster with a play button; tapping plays
// it full screen.
func (a *App) videoBubble(m store.Message, corners giode.Corners) giode.Element {
	att := m.Attachment
	bw, bh := mediaBox(att.Width, att.Height)
	if att.Width == 0 {
		bw, bh = 260, 170
	}
	var poster giode.Element
	if img := a.images.FromBytes(att.Thumb); img != nil {
		poster = giode.Image(img, giode.Styles{Width: wpx(bw), Height: hpx(bh), Fit: giode.FitCover})
	} else {
		poster = giode.Box(giode.Styles{Width: wpx(bw), Height: hpx(bh), BackgroundGradient: giode.Linear(160, "#1F2433", "#0B0D12")},
			giode.Box(giode.Styles{Position: giode.PositionAbsolute, Left: 14, Top: 14}, icon("film", 22, "#FFFFFF88")))
	}
	center := giode.Element(giode.Box(giode.Styles{Width: 56, Height: 56, BorderRadius: 999, Background: "#000000AA",
		Align: giode.AlignCenter, Justify: giode.JustifyCenter}, icon("play", 26, "#FFFFFF")))
	if needsDownload(m) {
		center = a.transferBadge(m, 56)
	}
	label := fileSize(att.Size)
	if att.DurationMs > 0 {
		label = duration(time.Duration(att.DurationMs)*time.Millisecond) + " · " + label
	}
	return giode.Box(giode.Styles{BorderRadii: corners, Overflow: giode.OverflowHidden, Width: wpx(bw), Height: hpx(bh)},
		poster,
		centered(bw, bh, center),
		giode.Box(giode.Styles{Position: giode.PositionAbsolute, Left: 8, Bottom: 8, Padding: giode.SymmetricInset(7, 3), BorderRadius: 999, Background: "#00000080"},
			text(label, fsTiny, "#FFFFFF")),
		giode.Box(giode.Styles{Position: giode.PositionAbsolute, Right: 8, Bottom: 8, Padding: giode.SymmetricInset(7, 3), BorderRadius: 999, Background: "#00000080"},
			metaLine(m, true)),
	)
}
