package ui

import (
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/mediakit"
)

// gifFrame is the frame of fr showing at t and when the next one is due.
func gifFrame(fr *mediakit.Frames, t time.Time) (int, time.Time) {
	total := 0
	for _, d := range fr.Delays {
		total += d
	}
	cs := int(t.UnixMilli()/10) % max(total, 1)
	until := 0
	for i, d := range fr.Delays {
		until += d
		if cs < until {
			return i, t.Add(time.Duration(until-cs) * 10 * time.Millisecond)
		}
	}
	return 0, t.Add(100 * time.Millisecond)
}

// gifView plays an animated GIF, scheduling a redraw for the frame that
// comes next instead of redrawing continuously. Cached list rows must
// include gifFrame's index in their version to animate.
func gifView(fr *mediakit.Frames, w, h int) giode.Element {
	st := giode.Styles{Width: wpx(w), Height: hpx(h), Fit: giode.FitCover}
	if len(fr.Images) == 1 {
		return giode.Image(fr.Images[0], st)
	}
	return giode.Raw(func(gtx layout.Context) layout.Dimensions {
		i, next := gifFrame(fr, gtx.Now)
		gtx.Execute(op.InvalidateCmd{At: next})
		return giode.Image(fr.Images[i], st).Layout(gtx)
	})
}
