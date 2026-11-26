package ui

import (
	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/store"
)

// mediaBox sizes a picture to its aspect ratio within bubble limits.
func mediaBox(w, h int) (int, int) {
	const maxW, maxH, minH = 280, 360, 120
	if w <= 0 || h <= 0 {
		return 240, 180
	}
	bw := maxW
	bh := bw * h / w
	if bh > maxH {
		bh = maxH
		bw = max(140, bh*w/h)
	}
	return bw, max(bh, minH)
}

// imageBubble shows a photo or an animated GIF.
func (a *App) imageBubble(m store.Message, corners giode.Corners) giode.Element {
	att := m.Attachment
	bw, bh := mediaBox(att.Width, att.Height)
	var pic giode.Element
	switch {
	case m.Kind == store.KindGIF && att.State == store.TransferDone:
		if fr := a.images.GIF(att.Path, 480); fr != nil {
			pic = gifView(fr, bw, bh)
		}
	case att.State == store.TransferDone:
		if img := a.images.FromFile(att.Path, 900); img != nil {
			pic = giode.Image(img, giode.Styles{Width: wpx(bw), Height: hpx(bh), Fit: giode.FitCover})
		}
	}
	if pic == nil {
		if img := a.images.FromBytes(att.Thumb); img != nil {
			pic = giode.Image(img, giode.Styles{Width: wpx(bw), Height: hpx(bh), Fit: giode.FitCover, Opacity: 0.85})
		} else {
			pic = giode.Box(giode.Styles{Width: wpx(bw), Height: hpx(bh), Background: "theme.raised"})
		}
	}
	layers := []giode.Element{pic}
	if needsDownload(m) {
		layers = append(layers, centered(bw, bh, a.transferBadge(m, 52)))
	}
	if m.Kind == store.KindGIF {
		layers = append(layers, giode.Box(giode.Styles{Position: giode.PositionAbsolute, Top: 8, Left: 8,
			Padding: giode.SymmetricInset(7, 3), BorderRadius: 6, Background: "#00000099"},
			text("GIF", fsTiny, "#FFFFFF", giode.FontWeightBold)))
	}
	layers = append(layers, giode.Box(giode.Styles{Position: giode.PositionAbsolute, Right: 8, Bottom: 8,
		Padding: giode.SymmetricInset(7, 3), BorderRadius: 999, Background: "#00000080"}, metaLine(m, true)))
	return giode.Box(giode.Styles{BorderRadii: corners, Overflow: giode.OverflowHidden, Width: wpx(bw), Height: hpx(bh)}, layers...)
}

// centered places el in the middle of a w×h area, over what's there.
func centered(w, h int, el giode.Element) giode.Element {
	return giode.Box(giode.Styles{Position: giode.PositionAbsolute, Top: giode.Flush, Left: giode.Flush,
		Width: wpx(w), Height: hpx(h), Align: giode.AlignCenter, Justify: giode.JustifyCenter}, el)
}
