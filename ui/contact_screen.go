package ui

import (
	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/store"
)

// contactScreen shows a contact: their ID, the safety number, and
// actions.
type contactScreen struct {
	a       *App
	peer    string
	back    *giode.IconButtonElement
	message *giode.IconButtonElement
	voice   *giode.IconButtonElement
	video   *giode.IconButtonElement
	mute    *giode.SwitchElement
	remove  *giode.ButtonElement
	confirm bool
	dialog  *giode.DialogElement
	yes, no *giode.ButtonElement
}

func newContactScreen(a *App) *contactScreen {
	s := &contactScreen{a: a}
	s.back = iconButton("arrow-left", 22, "Back", a.back)
	tile := func(ic, label string, fn func()) *giode.IconButtonElement {
		return giode.IconButton(icon(ic, 22, "theme.primary"), giode.Styles{Width: 56, Height: 56, BorderRadius: 18, Background: "theme.surface"}).
			Label(label).OnClick(fn)
	}
	s.message = tile("message-circle", "Message", func() { a.openChat(s.peer) })
	s.voice = tile("phone", "Voice call", func() { a.calls.start(s.peer, false) })
	s.video = tile("video", "Video call", func() { a.calls.start(s.peer, true) })
	s.mute = giode.Input.Switch("").OnChange(func(on bool) {
		peer := s.peer
		go func() {
			_, err := a.svc.Store().UpdateContact(peer, func(c *store.Contact) { c.Muted = on })
			a.g.Post(func() {
				a.toastError(err)
				a.reloadContacts()
			})
		}()
	})
	s.remove = giode.Button("Delete contact", giode.Styles{Color: "theme.danger", BorderColor: "theme.danger", BorderRadius: 14, Width: -1, Padding: giode.SymmetricInset(20, 13)}).
		Variant(giode.VariantOutlined).Icon(icon("trash-2", 18, "theme.danger")).OnClick(func() { s.confirm = true })
	s.yes = giode.Button("Delete", giode.Styles{Background: "theme.danger", Color: "#FFFFFF", BorderRadius: 12}).OnClick(s.delete)
	s.no = giode.Button("Cancel", giode.Styles{Color: "theme.text", BorderRadius: 12}).Variant(giode.VariantGhost).OnClick(func() { s.confirm = false })
	s.dialog = giode.Dialog(&s.confirm, func() giode.Element {
		c, _ := a.contactByID(s.peer)
		return giode.Stack(giode.Styles{Gap: 14, MaxWidth: 340},
			text("Delete "+c.Name+"?", fsTitle, "theme.text", giode.FontWeightSemiBold),
			text("The conversation and its files are removed from this device. To talk again you'll need a new invite code.", fsSmall+1, "theme.muted"),
			giode.Row(giode.Styles{Gap: 8, Justify: giode.JustifyEnd, Width: -1}, s.no, s.yes))
	}, giode.Styles{Background: "theme.surface", BorderRadius: 18})
	return s
}

func (s *contactScreen) open(peer string) {
	s.peer = peer
	c, _ := s.a.contactByID(peer)
	s.mute.SetChecked(c.Muted)
}

func (s *contactScreen) delete() {
	s.confirm = false
	peer := s.peer
	go func() {
		err := s.a.svc.DeleteContact(peer)
		s.a.g.Post(func() {
			if err != nil {
				s.a.toastError(err)
				return
			}
			s.a.home()
		})
	}()
}

func (s *contactScreen) view() giode.Element {
	c, _ := s.a.contactByID(s.peer)
	status := lastSeen(c.LastSeen)
	if s.a.online[c.PeerID] {
		status = "online"
	}
	safety := giode.Stack(giode.Styles{Width: -1, Gap: 10, Padding: giode.UniformInset(16), Background: "theme.surface", BorderRadius: 16},
		giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter}, icon("shield-check", 18, "theme.success"),
			text("Safety number", fsBody, "theme.text", giode.FontWeightSemiBold)),
		giode.Text(safetyNumber(s.a.svc.ID(), c.PeerID), giode.Styles{FontSize: fsBody + 1, FontFamily: "Go Mono", Color: "theme.text", LineHeight: 26, TextAlign: giode.TextAlignCenter, Width: -1}),
		text("Compare these numbers with the ones on "+c.Name+"'s screen. If they match, your conversation is end-to-end encrypted with no one in between.", fsSmall, "theme.muted"),
	)
	body := giode.Stack(giode.Styles{Width: -1, MaxWidth: 560, Gap: 22, Align: giode.AlignCenter, Padding: giode.Inset{Left: 16, Right: 16, Bottom: 40}},
		s.a.avatarView(c.Name, c.Avatar, 112, s.a.online[c.PeerID]),
		giode.Stack(giode.Styles{Gap: 4, Align: giode.AlignCenter},
			text(c.Name, fsHuge-2, "theme.text", giode.FontWeightBold),
			text(status, fsSmall, "theme.muted")),
		giode.Row(giode.Styles{Gap: 18, Justify: giode.JustifyCenter},
			labeledTile(s.message, "Message"), labeledTile(s.voice, "Voice"), labeledTile(s.video, "Video")),
		safety,
		section("CONTACT",
			settingRow("Mute notifications", "", s.mute),
			settingRow("ID", groupID(c.PeerID), nil)),
		s.remove,
	)
	return giode.Stack(giode.Styles{Width: -1, Height: -1},
		screenHeader(s.back, "Contact"),
		giode.Scroll(func() giode.Element {
			return giode.Row(giode.Styles{Width: -1, Justify: giode.JustifyCenter}, body)
		}, giode.Styles{Width: -1, FlexGrow: 1}),
		s.dialog,
	)
}

func labeledTile(el giode.Element, label string) giode.Element {
	return giode.Stack(giode.Styles{Gap: 6, Align: giode.AlignCenter}, el, text(label, fsSmall, "theme.muted", giode.FontWeightMedium))
}
