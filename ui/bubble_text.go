package ui

import (
	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/store"
)

// textBubble shows a text message with its time tucked under the text.
func textBubble(m store.Message, corners giode.Corners) giode.Element {
	st := bubbleStyle(m.Outgoing, corners)
	st.Padding = giode.Inset{Top: 8, Bottom: 6, Left: 13, Right: 13}
	st.FlexDirection = giode.FlexDirectionColumn
	st.Gap = 2
	body := m.Text
	color := st.Color
	if body == "" {
		body, color = "Message deleted", "theme.muted"
	}
	return giode.Box(st,
		giode.Text(body, giode.Styles{FontSize: fsBody + 1, LineHeight: 22, Color: color}),
		metaLine(m, m.Outgoing),
	)
}
