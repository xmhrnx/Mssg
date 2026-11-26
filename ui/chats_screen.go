package ui

import (
	"strings"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/store"
)

// chatsScreen is the home screen: every conversation, newest first.
type chatsScreen struct {
	a        *App
	search   *giode.InputElement
	list     *giode.LazyListElement
	add      *giode.IconButtonElement
	settings *giode.IconButtonElement
	fab      *giode.IconButtonElement
	share    *giode.ButtonElement
	enter    *giode.ButtonElement
	shown    []store.Contact
}

func newChatsScreen(a *App) *chatsScreen {
	s := &chatsScreen{a: a}
	s.search = giode.Input.Text("Search", giode.Styles{
		Background: "theme.raised", BorderColor: "theme.raised", BorderRadius: 12,
		Padding: giode.SymmetricInset(14, 10), FontSize: fsBody, Color: "theme.text",
	})
	s.list = giode.LazyList(
		func() int { return len(s.shown) },
		func(i int) any { return s.shown[i].PeerID },
		func(i int) giode.Element { return s.row(s.shown[i]) },
	)
	s.add = iconButton("user-plus", 22, "Add contact", func() { a.addContact.show(addModeShare) })
	s.settings = iconButton("settings", 22, "Settings", func() { a.push(route{screen: screenSettings}) })
	s.fab = roundAction("plus", 58, "New chat", gradientMine, func() { a.addContact.show(addModeShare) })
	s.share = primaryButton("Share my invite code", func() { a.addContact.show(addModeShare) })
	s.enter = secondaryButton("Enter a friend's code", func() { a.addContact.show(addModeEnter) })
	return s
}

func (s *chatsScreen) row(c store.Contact) giode.Element {
	p := giode.Keyed("chat-rows", c.PeerID, func() *giode.PressableElement {
		peer := c.PeerID
		return giode.Pressable(nil).OnClick(func(giode.ClickEvent) { s.a.openChat(peer) })
	})
	p.Label(c.Name)
	p.Content(func(st giode.PressState) giode.Element { return s.a.chatRow(c, st) })
	return p
}

func (s *chatsScreen) filter() {
	q := strings.ToLower(strings.TrimSpace(s.search.Text()))
	s.shown = s.shown[:0]
	for _, c := range s.a.contacts {
		if q == "" || strings.Contains(strings.ToLower(c.Name), q) {
			s.shown = append(s.shown, c)
		}
	}
}

func (s *chatsScreen) view() giode.Element {
	s.filter()
	header := giode.Row(giode.Styles{Width: -1, Align: giode.AlignCenter, Padding: giode.Inset{Top: 14, Left: 20, Right: 8, Bottom: 6}, Wrap: giode.WrapNoWrap},
		giode.Box(giode.Styles{FlexGrow: 1}, text("Chats", fsHuge, "theme.text", giode.FontWeightBold)),
		s.add, s.settings,
	)
	var body giode.Element
	switch {
	case len(s.a.contacts) == 0:
		body = s.empty()
	case len(s.shown) == 0:
		body = giode.Stack(giode.Styles{Width: -1, Padding: giode.UniformInset(40), Align: giode.AlignCenter},
			text("No chats match", fsBody, "theme.muted"))
	default:
		body = giode.Box(giode.Styles{Width: -1, FlexGrow: 1}, s.list)
	}
	items := []giode.Element{header}
	if len(s.a.contacts) > 0 {
		items = append(items, giode.Box(giode.Styles{Width: -1, Padding: giode.Inset{Left: 16, Right: 16, Bottom: 8}}, s.search))
	}
	items = append(items, body)
	page := giode.Stack(giode.Styles{Width: -1, Height: -1}, items...)
	if len(s.a.contacts) == 0 {
		return page
	}
	return giode.Box(giode.Styles{Width: -1, Height: -1}, page,
		giode.Box(giode.Styles{Position: giode.PositionAbsolute, Right: 20, Bottom: 24, ZIndex: 2}, s.fab))
}

func (s *chatsScreen) empty() giode.Element {
	return giode.Stack(giode.Styles{Width: -1, FlexGrow: 1, Align: giode.AlignCenter, Justify: giode.JustifyCenter, Padding: giode.UniformInset(28)},
		giode.Stack(giode.Styles{Width: -1, MaxWidth: 380, Gap: 22, Align: giode.AlignCenter},
			giode.Box(giode.Styles{Width: 120, Height: 120, BorderRadius: 999, Background: "theme.raised", Align: giode.AlignCenter, Justify: giode.JustifyCenter},
				giode.Box(giode.Styles{Width: 72, Height: 72, BorderRadius: 22, BackgroundGradient: gradientHero, Align: giode.AlignCenter, Justify: giode.JustifyCenter},
					icon("sparkles", 34, "#FFFFFF"))),
			giode.Stack(giode.Styles{Gap: 8, Align: giode.AlignCenter},
				text("Start your first chat", fsTitle+2, "theme.text", giode.FontWeightBold),
				giode.Text("Share your invite code with a friend, or type theirs. The code is used once to introduce your devices; after that you talk directly.",
					giode.Styles{FontSize: fsSmall + 1, Color: "theme.muted", TextAlign: giode.TextAlignCenter, LineHeight: 20}),
			),
			giode.Stack(giode.Styles{Gap: 10, Width: -1}, s.share, s.enter),
		),
	)
}
