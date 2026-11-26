package ui

import (
	"path/filepath"
	"strings"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/store"
)

// fileBubble shows any other file: its type, name, size and state.
func (a *App) fileBubble(m store.Message, corners giode.Corners) giode.Element {
	att := m.Attachment
	st := bubbleStyle(m.Outgoing, corners)
	st.Padding = giode.Inset{Top: 10, Bottom: 8, Left: 10, Right: 14}
	st.FlexDirection = giode.FlexDirectionColumn
	st.Gap = 4
	ext := strings.ToUpper(strings.TrimPrefix(filepath.Ext(att.Name), "."))
	if len(ext) > 4 || ext == "" {
		ext = ""
	}
	tileBg, tileFg := "theme.raised", "theme.primary"
	if m.Outgoing {
		tileBg, tileFg = "#FFFFFF33", "#FFFFFF"
	}
	var tile giode.Element
	switch {
	case needsDownload(m) && a.svc.Downloading(m.ID):
		frac := float32(0)
		if att.Size > 0 {
			frac = float32(att.Received) / float32(att.Size)
		}
		tile = progressRing(44, frac, tileFg)
	case needsDownload(m):
		tile = icon("download", 22, tileFg)
	default:
		tile = giode.Stack(giode.Styles{Align: giode.AlignCenter, Gap: 1}, icon("file-text", 20, tileFg),
			text(ext, 9, tileFg, giode.FontWeightBold))
	}
	status := fileSize(att.Size)
	switch {
	case needsDownload(m) && a.svc.Downloading(m.ID):
		status = fileSize(att.Received) + " of " + fileSize(att.Size)
	case att.State == store.TransferFailed && !m.Outgoing:
		status += " · tap to retry"
	case needsDownload(m):
		status += " · tap to download"
	}
	return giode.Box(st,
		giode.Row(giode.Styles{Gap: 12, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
			giode.Box(giode.Styles{Width: 44, Height: 44, BorderRadius: 12, Background: tileBg, Align: giode.AlignCenter, Justify: giode.JustifyCenter}, tile),
			giode.Stack(giode.Styles{Gap: 3, MaxWidth: 220},
				line(att.Name, fsBody, st.Color, giode.FontWeightMedium),
				text(status, fsTiny+1, mutedOn(m.Outgoing)))),
		metaLine(m, m.Outgoing),
	)
}
