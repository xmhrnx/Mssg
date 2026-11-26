package ui

import (
	"time"

	"github.com/xmhrnx/giode"
)

// ticker redraws the window every interval until stop closes, for
// clocks and meters that change without input.
func ticker(g *giode.App, every time.Duration, stop <-chan struct{}) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			g.Invalidate()
		}
	}
}
