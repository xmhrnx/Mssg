package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/store"
)

// preview is a one-line summary of a message for the chat list and
// notifications.
func preview(m store.Message) string {
	switch m.Kind {
	case store.KindText:
		if m.Text == "" {
			return "Message deleted"
		}
		return strings.Join(strings.Fields(m.Text), " ")
	case store.KindImage:
		return "📷 Photo"
	case store.KindGIF:
		return "GIF"
	case store.KindVideo:
		return "🎬 Video"
	case store.KindAudio:
		if m.Attachment != nil && m.Attachment.DurationMs > 0 {
			return "🎤 Voice message " + duration(time.Duration(m.Attachment.DurationMs)*time.Millisecond)
		}
		return "🎵 Audio"
	case store.KindFile:
		if m.Attachment != nil {
			return "📄 " + m.Attachment.Name
		}
		return "📄 File"
	case store.KindCall:
		return callSummary(m)
	}
	return ""
}

func callSummary(m store.Message) string {
	c := m.Call
	if c == nil {
		return "Call"
	}
	kind := "voice call"
	if c.Video {
		kind = "video call"
	}
	switch {
	case c.Missed && !m.Outgoing:
		return "Missed " + kind
	case c.Declined:
		return "Declined " + kind
	case c.Seconds == 0:
		if m.Outgoing {
			return "Cancelled " + kind
		}
		return "Missed " + kind
	}
	dir := "Incoming"
	if m.Outgoing {
		dir = "Outgoing"
	}
	return fmt.Sprintf("%s %s · %s", dir, kind, duration(time.Duration(c.Seconds)*time.Second))
}

// chatRow is one conversation in the list.
func (a *App) chatRow(c store.Contact, state giode.PressState) giode.Element {
	m, hasLast := a.last[c.PeerID]
	sub, subColor := "Say hello 👋", "theme.muted"
	switch {
	case !c.Confirmed && c.InviteToken != "":
		sub = "Connecting… they'll appear once you're both online"
	case a.isTyping(c.PeerID):
		sub, subColor = "typing…", "theme.primary"
	case hasLast:
		sub = preview(m)
	}
	right := []giode.Element{}
	if hasLast {
		tc := "theme.muted"
		if c.Unread > 0 {
			tc = "theme.primary"
		}
		right = append(right, text(listTime(m.Time), fsTiny, tc, giode.FontWeightMedium))
	}
	switch {
	case c.Unread > 0:
		right = append(right, giode.Badge(fmt.Sprint(min(c.Unread, 999)), giode.Styles{Background: "theme.primary", FontSize: fsTiny}))
	case hasLast && m.Outgoing && m.Kind != store.KindCall:
		right = append(right, statusTicks(m.Status, 16))
	}
	bg := "theme.background"
	if state.Pressed || state.Hovered {
		bg = pressTint
	}
	name := c.Name
	if name == "" {
		name = shortID(c.PeerID)
	}
	nameWeight := giode.FontWeightSemiBold
	return giode.Row(giode.Styles{
		Width: -1, Gap: 14, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap,
		Padding: giode.SymmetricInset(16, 10), Background: bg,
	},
		a.avatarView(name, c.Avatar, 54, a.online[c.PeerID]),
		giode.Stack(giode.Styles{FlexGrow: 1, Gap: 4},
			line(name, fsBody+1, "theme.text", nameWeight),
			line(sub, fsSmall+1, subColor),
		),
		giode.Stack(giode.Styles{Gap: 6, Align: giode.AlignEnd}, right...),
	)
}

// statusTicks shows how far an outgoing message got.
func statusTicks(s store.Status, size int) giode.Element {
	switch s {
	case store.StatusPending:
		return icon("clock", size-3, "theme.muted")
	case store.StatusSent:
		return icon("check", size, "theme.muted")
	case store.StatusDelivered:
		return icon("check-check", size, "theme.muted")
	case store.StatusRead:
		return icon("check-check", size, "theme.accent")
	}
	return icon("circle-alert", size-2, "theme.danger")
}
