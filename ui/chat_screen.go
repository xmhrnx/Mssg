package ui

import (
	"time"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/store"
)

const pageSize = 60

// chatItem is one row of the conversation: a message or a day heading.
type chatItem struct {
	key string
	day time.Time
	msg int // index into msgs; -1 for a day heading
}

// chatScreen is one conversation.
type chatScreen struct {
	a        *App
	peer     string
	msgs     []store.Message
	index    map[string]int
	items    []chatItem
	dirty    bool
	hasOlder bool
	loading  bool
	gen      int // bumps when the peer changes, to drop stale loads

	list     *giode.LazyListElement
	back     *giode.IconButtonElement
	voice    *giode.IconButtonElement
	video    *giode.IconButtonElement
	title    *giode.PressableElement
	composer *composer
	actions  *messageActions
	jump     *giode.IconButtonElement
}

func newChatScreen(a *App) *chatScreen {
	s := &chatScreen{a: a, index: map[string]int{}}
	s.list = giode.LazyList(
		func() int { return len(s.items) },
		func(i int) any { return s.items[i].key },
		func(i int) giode.Element { return s.row(i) },
		giode.Styles{Padding: giode.Inset{Top: 8, Bottom: 8}},
	).Version(func(i int) any { return s.version(i) }).
		Anchor(giode.LazyAnchorBottom).
		OnReachTop(s.loadOlder).
		StickyHeaders(func(i int) bool { return s.items[i].msg < 0 })
	s.back = iconButton("arrow-left", 22, "Back", a.back)
	s.voice = iconButton("phone", 21, "Voice call", func() { a.calls.start(s.peer, false) })
	s.video = iconButton("video", 22, "Video call", func() { a.calls.start(s.peer, true) })
	s.title = giode.Pressable(nil).Label("Contact info").OnClick(func(giode.ClickEvent) {
		a.push(route{screen: screenContact, peer: s.peer})
	})
	s.composer = newComposer(a, s)
	s.actions = newMessageActions(a)
	s.jump = giode.IconButton(icon("chevron-right", 20, "theme.text"), giode.Styles{
		Width: 40, Height: 40, BorderRadius: 999, Background: "theme.surface",
		BoxShadow: giode.BoxShadow{Y: 4, Blur: 14, Color: "#00000040"},
		Transform: giode.Transform{Rotate: 90},
	}).Label("Jump to latest").OnClick(func() { s.list.ScrollToBottom() })
	return s
}

type rowVersion struct {
	rev, gen, prev, next int
	playing              bool
	progress             int
	frame                int // of an animated GIF
	downloading          bool
}

// version changes whenever a row must be rebuilt: the message changed,
// a neighbor changed how it groups, or a picture finished decoding.
func (s *chatScreen) version(i int) any {
	it := s.items[i]
	if it.msg < 0 {
		return it.day
	}
	m := s.msgs[it.msg]
	v := rowVersion{rev: m.Rev, gen: s.a.images.Gen()}
	if it.msg > 0 {
		v.prev = s.msgs[it.msg-1].Rev + int(s.msgs[it.msg-1].Seq)
	}
	if it.msg+1 < len(s.msgs) {
		v.next = s.msgs[it.msg+1].Rev + int(s.msgs[it.msg+1].Seq)
	}
	if m.Kind == store.KindGIF && m.Attachment != nil && m.Attachment.State == store.TransferDone {
		if fr := s.a.images.GIF(m.Attachment.Path, 480); fr != nil && len(fr.Images) > 1 {
			v.frame, _ = gifFrame(fr, time.Now())
		}
	}
	v.downloading = s.a.svc.Downloading(m.ID)
	if s.a.voice.current == m.ID {
		v.playing = s.a.voice.playing()
		v.progress = int(s.a.voice.progress() * 200)
	}
	return v
}

// open shows a peer's conversation, loading its latest page.
func (s *chatScreen) open(peer string) {
	if s.peer != peer {
		s.peer = peer
		s.gen++
		s.msgs, s.items = nil, nil
		s.index = map[string]int{}
		s.hasOlder = true
		s.loading = false
		s.composer.reset(peer)
		s.loadOlder()
	}
	s.a.svc.SetActiveChat(peer)
}

func (s *chatScreen) close() {
	if s.peer != "" {
		s.composer.saveDraft()
		s.a.svc.SetActiveChat("")
	}
	s.peer = ""
}

func (s *chatScreen) loadOlder() {
	if s.loading || !s.hasOlder || s.peer == "" {
		return
	}
	s.loading = true
	peer, gen := s.peer, s.gen
	var before uint64
	if len(s.msgs) > 0 {
		before = s.msgs[0].Seq
	}
	go func() {
		page, err := s.a.svc.Messages(peer, before, pageSize)
		s.a.g.Post(func() {
			if gen != s.gen {
				return
			}
			s.loading = false
			if err != nil {
				s.a.toastError(err)
				return
			}
			s.hasOlder = len(page) == pageSize
			fresh := page[:0]
			for _, m := range page {
				if _, ok := s.index[m.ID]; !ok {
					fresh = append(fresh, m)
				}
			}
			s.msgs = append(append([]store.Message(nil), fresh...), s.msgs...)
			s.reindex()
		})
	}()
}

func (s *chatScreen) reindex() {
	s.index = make(map[string]int, len(s.msgs))
	for i, m := range s.msgs {
		s.index[m.ID] = i
	}
	s.dirty = true
}

// onMessage merges a new or changed message into the open conversation.
func (s *chatScreen) onMessage(m store.Message) {
	if m.Peer != s.peer {
		return
	}
	if i, ok := s.index[m.ID]; ok {
		m.Rev = s.msgs[i].Rev + 1
		s.msgs[i] = m
		return
	}
	m.Rev = 1
	s.msgs = append(s.msgs, m)
	s.index[m.ID] = len(s.msgs) - 1
	s.dirty = true
	if m.Outgoing {
		s.list.ScrollToBottom()
	}
}

func (s *chatScreen) rebuildItems() {
	if !s.dirty {
		return
	}
	s.dirty = false
	s.items = s.items[:0]
	var lastDay time.Time
	for i, m := range s.msgs {
		if !sameDay(m.Time, lastDay) {
			lastDay = m.Time
			y, mo, d := m.Time.Date()
			day := time.Date(y, mo, d, 0, 0, 0, 0, time.Local)
			s.items = append(s.items, chatItem{key: "day:" + day.Format("2006-01-02"), day: day, msg: -1})
		}
		s.items = append(s.items, chatItem{key: m.ID, msg: i})
	}
}

func (s *chatScreen) row(i int) giode.Element {
	it := s.items[i]
	if it.msg < 0 {
		return dayHeading(it.day)
	}
	m := s.msgs[it.msg]
	var prev, next *store.Message
	if it.msg > 0 {
		prev = &s.msgs[it.msg-1]
	}
	if it.msg+1 < len(s.msgs) {
		next = &s.msgs[it.msg+1]
	}
	return s.a.messageRow(m, prev, next, s.actions)
}

func dayHeading(day time.Time) giode.Element {
	return giode.Row(giode.Styles{Width: -1, Justify: giode.JustifyCenter, Padding: giode.SymmetricInset(0, 10)},
		giode.Box(giode.Styles{Padding: giode.SymmetricInset(12, 5), BorderRadius: 999, Background: "theme.surface",
			BoxShadow: giode.BoxShadow{Y: 1, Blur: 4, Color: "#00000022"}},
			text(dayLabel(day), fsTiny+1, "theme.muted", giode.FontWeightMedium)))
}

func (s *chatScreen) view() giode.Element {
	s.rebuildItems()
	c, _ := s.a.contactByID(s.peer)
	list := giode.Box(giode.Styles{Width: -1, FlexGrow: 1, Background: "theme.chatBg"}, s.list)
	if len(s.msgs) == 0 && !s.loading {
		list = giode.Box(giode.Styles{Width: -1, FlexGrow: 1, Background: "theme.chatBg", Align: giode.AlignCenter, Justify: giode.JustifyCenter},
			s.emptyChat(c))
	}
	overlay := []giode.Element{list}
	if !s.list.Following() && len(s.msgs) > 0 {
		overlay = append(overlay, giode.Box(giode.Styles{Position: giode.PositionAbsolute, Right: 16, Bottom: 16, ZIndex: 3}, s.jump))
	}
	return giode.Stack(giode.Styles{Width: -1, Height: -1},
		s.header(c),
		giode.Box(giode.Styles{Width: -1, FlexGrow: 1}, overlay...),
		s.composer.view(),
		s.actions.view(),
	)
}

func (s *chatScreen) emptyChat(c store.Contact) giode.Element {
	note := "Messages are end-to-end encrypted and go straight to " + c.Name + "'s device."
	if !c.Confirmed {
		note = "Waiting to reach " + c.Name + ". You can write now; messages are delivered once you're both online."
	}
	return giode.Stack(giode.Styles{Gap: 12, Align: giode.AlignCenter, MaxWidth: 300, Padding: giode.UniformInset(24)},
		s.a.avatarView(c.Name, c.Avatar, 84, false),
		text(c.Name, fsTitle, "theme.text", giode.FontWeightSemiBold),
		giode.Row(giode.Styles{Gap: 6, Align: giode.AlignCenter},
			icon("lock", 14, "theme.muted"),
			giode.Text(note, giode.Styles{FontSize: fsSmall, Color: "theme.muted", TextAlign: giode.TextAlignCenter, MaxWidth: 260})),
	)
}

func (s *chatScreen) header(c store.Contact) giode.Element {
	status, color := lastSeen(c.LastSeen), "theme.muted"
	switch {
	case s.a.isTyping(c.PeerID):
		status, color = "typing…", "theme.primary"
	case s.a.online[c.PeerID]:
		status, color = "online", "theme.online"
	case !c.Confirmed:
		status = "connecting…"
	}
	s.title.Content(func(giode.PressState) giode.Element {
		return giode.Row(giode.Styles{Gap: 10, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
			s.a.avatarView(c.Name, c.Avatar, 40, false),
			giode.Stack(giode.Styles{Gap: 1},
				line(c.Name, fsBody+1, "theme.text", giode.FontWeightSemiBold),
				line(status, fsSmall-1, color)),
		)
	})
	return giode.Row(giode.Styles{
		Width: -1, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap, Gap: 4,
		Padding:    giode.Inset{Top: 8, Bottom: 8, Left: 4, Right: 6},
		Background: "theme.background", BorderColor: hairline,
	},
		s.back,
		giode.Box(giode.Styles{FlexGrow: 1}, s.title),
		s.voice, s.video,
	)
}
