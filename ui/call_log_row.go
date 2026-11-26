package ui

import (
	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/store"
)

// callLogRow is a call in the conversation, centered; tapping calls back.
func (a *App) callLogRow(m store.Message) giode.Element {
	c := m.Call
	ic, color := "phone-incoming", "theme.success"
	switch {
	case c == nil:
	case (c.Missed || c.Seconds == 0) && !m.Outgoing:
		ic, color = "phone-missed", "theme.danger"
	case m.Outgoing:
		ic, color = "phone-outgoing", "theme.primary"
	}
	if c != nil && c.Video {
		ic = "video"
	}
	p := giode.Keyed("calllog", m.ID, func() *giode.PressableElement {
		peer, video := m.Peer, c != nil && c.Video
		return giode.Pressable(nil).Label("Call back").OnClick(func(giode.ClickEvent) { a.calls.start(peer, video) })
	})
	p.Content(func(st giode.PressState) giode.Element {
		bg := "theme.surface"
		if st.Pressed {
			bg = pressTint
		}
		return giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter, Padding: giode.SymmetricInset(14, 8), BorderRadius: 999, Background: bg, Wrap: giode.WrapNoWrap},
			icon(ic, 16, color),
			text(callSummary(m), fsSmall, "theme.text", giode.FontWeightMedium),
			text(m.Time.Format("15:04"), fsTiny, "theme.muted"))
	})
	return giode.Row(giode.Styles{Width: -1, Justify: giode.JustifyCenter, Padding: giode.SymmetricInset(0, 8)}, p)
}
