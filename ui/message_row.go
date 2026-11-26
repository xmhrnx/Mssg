package ui

import (
	"time"
	"unicode"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/styles/properties"

	"github.com/xmhrnx/mssg/internal/store"
)

const (
	bubbleRadius = 18
	tailRadius   = 6
	bubbleMax    = 440
	groupWindow  = 3 * time.Minute
)

// groupsWith reports whether two messages sit together as one block.
func groupsWith(a store.Message, b *store.Message) bool {
	return b != nil && b.Outgoing == a.Outgoing && b.Kind != store.KindCall && a.Kind != store.KindCall &&
		sameDay(a.Time, b.Time) && absDur(a.Time.Sub(b.Time)) < groupWindow
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// bubbleCorners rounds a bubble, tightening the corners it shares with
// its neighbors on the sender's side.
func bubbleCorners(out, groupedAbove, groupedBelow bool) giode.Corners {
	c := giode.Corners{TopLeft: bubbleRadius, TopRight: bubbleRadius, BottomLeft: bubbleRadius, BottomRight: bubbleRadius}
	if out {
		if groupedAbove {
			c.TopRight = tailRadius
		}
		if groupedBelow {
			c.BottomRight = tailRadius
		}
		return c
	}
	if groupedAbove {
		c.TopLeft = tailRadius
	}
	if groupedBelow {
		c.BottomLeft = tailRadius
	}
	return c
}

// messageRow lays one message out on its side of the conversation.
func (a *App) messageRow(m store.Message, prev, next *store.Message, actions *messageActions) giode.Element {
	if m.Kind == store.KindCall {
		return a.callLogRow(m)
	}
	above, below := groupsWith(m, prev), groupsWith(m, next)
	corners := bubbleCorners(m.Outgoing, above, below)
	content := a.bubble(m, corners)
	p := giode.Keyed("bubbles", m.ID, func() *giode.PressableElement {
		id := m.ID
		return giode.Pressable(nil).
			OnClick(func(giode.ClickEvent) { a.openMessage(id) }).
			OnLongPress(func(giode.ClickEvent) { actions.show(id) }).
			OnSecondaryClick(func(giode.ClickEvent) { actions.show(id) })
	})
	p.Label(preview(m))
	p.Content(func(giode.PressState) giode.Element { return content })
	top := 2
	if !above {
		top = 8
	}
	justify := giode.JustifyStart
	pad := giode.Inset{Top: top, Left: 12, Right: 56}
	if m.Outgoing {
		justify = giode.JustifyEnd
		pad = giode.Inset{Top: top, Left: 56, Right: 12}
	}
	return giode.Row(giode.Styles{Width: -1, Justify: justify, Padding: pad, Wrap: giode.WrapNoWrap},
		giode.Box(giode.Styles{MaxWidth: bubbleMax}, p))
}

// bubble picks the body for a message's kind.
func (a *App) bubble(m store.Message, corners giode.Corners) giode.Element {
	switch m.Kind {
	case store.KindImage, store.KindGIF:
		return a.imageBubble(m, corners)
	case store.KindVideo:
		return a.videoBubble(m, corners)
	case store.KindAudio:
		return a.audioBubble(m, corners)
	case store.KindFile:
		return a.fileBubble(m, corners)
	}
	if isJumbo(m.Text) {
		return giode.Stack(giode.Styles{Gap: 2, Align: alignFor(m.Outgoing)},
			giode.Text(m.Text, giode.Styles{FontSize: 44}),
			metaLine(m, false))
	}
	return textBubble(m, corners)
}

func alignFor(out bool) properties.Align {
	if out {
		return giode.AlignEnd
	}
	return giode.AlignStart
}

// isJumbo reports a short message of only emoji, shown large.
func isJumbo(s string) bool {
	if s == "" {
		return false
	}
	count := 0
	for _, r := range s {
		switch {
		case r == 0x200d || (r >= 0xfe00 && r <= 0xfe0f) || (r >= 0x1f3fb && r <= 0x1f3ff) || unicode.IsSpace(r):
			continue
		case r < 0x2000 || unicode.IsLetter(r) || unicode.IsDigit(r):
			return false
		}
		count++
	}
	return count > 0 && count <= 3
}

// bubbleStyle is the shell of a bubble: the gradient for yours.
func bubbleStyle(out bool, corners giode.Corners) giode.Styles {
	st := giode.Styles{BorderRadii: corners, Overflow: giode.OverflowHidden}
	if out {
		st.BackgroundGradient = gradientMine
		st.Color = "#FFFFFF"
	} else {
		st.Background = "theme.bubbleIn"
		st.Color = "theme.text"
		st.BoxShadow = giode.BoxShadow{Y: 1, Blur: 2, Color: "#00000018"}
	}
	return st
}

// metaLine is a bubble's time and, for yours, the delivery ticks.
func metaLine(m store.Message, onColor bool) giode.Element {
	color := "theme.muted"
	if onColor {
		color = "#FFFFFFB8"
	}
	items := []giode.Element{text(m.Time.Format("15:04"), fsTiny, color)}
	if m.Outgoing {
		tick := statusTicks(m.Status, 15)
		if onColor && m.Status != store.StatusFailed {
			tickColor := "#FFFFFFB8"
			if m.Status == store.StatusRead {
				tickColor = "#9FF3FF"
			}
			name := "check-check"
			switch m.Status {
			case store.StatusPending:
				name = "clock"
			case store.StatusSent:
				name = "check"
			}
			tick = icon(name, 15, tickColor)
		}
		items = append(items, tick)
	}
	return giode.Row(giode.Styles{Gap: 4, Align: giode.AlignCenter, Justify: giode.JustifyEnd, Wrap: giode.WrapNoWrap}, items...)
}
