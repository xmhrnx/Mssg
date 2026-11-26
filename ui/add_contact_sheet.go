package ui

import (
	"context"
	"strings"
	"time"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/store"
)

type addMode int

const (
	addModeShare addMode = iota
	addModeEnter
)

type inviteState int

const (
	inviteIdle inviteState = iota
	inviteOpening
	inviteWaiting
	inviteReceived
	inviteDone
	inviteFailed
)

// addContactSheet introduces two devices through a magic wormhole code.
type addContactSheet struct {
	a     *App
	open  bool
	sheet *giode.SheetElement
	mode  *giode.SegmentedElement

	// Sharing this device's code.
	state   inviteState
	code    string
	err     string
	cancel  context.CancelFunc
	friend  store.Contact
	copy    *giode.ButtonElement
	share   *giode.ButtonElement
	retry   *giode.ButtonElement
	openNew *giode.ButtonElement

	// Entering someone's code.
	input    *giode.InputElement
	connect  *giode.ButtonElement
	enterErr string
}

func newAddContactSheet(a *App) *addContactSheet {
	s := &addContactSheet{a: a}
	s.mode = giode.Input.Segmented("Share my code", "Enter a code").OnChange(func(i int) {
		if i == 0 {
			s.startInvite()
		} else {
			s.input.Focus()
		}
	})
	s.mode.Styles(giode.Styles{Width: -1})
	s.copy = secondaryButton("Copy", func() {
		a.g.SetClipboard(s.code)
		a.toasts.Show(giode.ToastSuccess, "Code copied")
	}).Icon(icon("copy", 18, "theme.text"))
	s.share = primaryButton("Share", func() {
		msg := "Add me on Mssg, the private peer-to-peer messenger. My one-time code: " + s.code
		go func() {
			if err := a.g.Share(context.Background(), giode.ShareText{Text: msg, Subject: "My Mssg code"}); err != nil && !isCanceled(err) {
				a.g.Post(func() { a.toastError(err) })
			}
		}()
	}).Icon(icon("share-2", 18, "#FFFFFF"))
	s.retry = secondaryButton("Get a new code", s.startInvite)
	s.openNew = primaryButton("Open chat", func() {
		s.hide()
		a.openChat(s.friend.PeerID)
	})
	s.input = textField("e.g. 7-guitarist-revenge").Submit(func(string) { s.redeem() })
	s.input.Autocorrect(false)
	s.connect = primaryButton("Connect", s.redeem)
	s.sheet = giode.Sheet(&s.open, s.body).Title("Add a contact").Adaptive(true)
	return s
}

func (s *addContactSheet) show(m addMode) {
	s.open = true
	s.enterErr = ""
	s.mode.SetSelected(int(m))
	if m == addModeShare {
		s.startInvite()
	} else {
		s.input.SetText("")
		s.input.Focus()
	}
}

func (s *addContactSheet) hide() {
	s.open = false
}

// startInvite opens a wormhole with this device's card, unless one is open.
func (s *addContactSheet) startInvite() {
	if s.state == inviteOpening || s.state == inviteWaiting || s.state == inviteReceived {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	s.cancel = cancel
	s.state, s.code, s.err = inviteOpening, "", ""
	go func() {
		offer, err := s.a.svc.CreateInvite(ctx)
		if err != nil {
			s.a.g.Post(func() { s.state, s.err = inviteFailed, friendly(err) })
			return
		}
		s.a.g.Post(func() { s.state, s.code = inviteWaiting, offer.Code })
		err = <-offer.Done
		s.a.g.Post(func() {
			if s.state == inviteDone {
				return
			}
			if err != nil {
				s.state, s.err = inviteFailed, friendly(err)
				return
			}
			s.state = inviteReceived
		})
	}()
}

// redeemed is told when a contact used this device's code.
func (s *addContactSheet) redeemed(c store.Contact) {
	s.friend = c
	s.state = inviteDone
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

func (s *addContactSheet) redeem() {
	code := strings.TrimSpace(s.input.Text())
	if code == "" {
		s.enterErr = "Type the code your friend shared."
		return
	}
	s.enterErr = ""
	s.connect.Loading(true)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		c, err := s.a.svc.RedeemInvite(ctx, code)
		s.a.g.Post(func() {
			s.connect.Loading(false)
			if err != nil {
				s.enterErr = friendly(err)
				return
			}
			s.hide()
			s.a.toasts.Show(giode.ToastSuccess, "Added "+c.Name)
			s.a.reloadContacts()
			s.a.openChat(c.PeerID)
		})
	}()
}

// friendly rewords network errors for people.
func friendly(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "dial") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host"):
		return "Couldn't reach the code exchange. Check your internet connection and try again."
	case strings.Contains(msg, "bad code") || strings.Contains(msg, "crowded") || strings.Contains(msg, "nameplate"):
		return "That code didn't work. Codes work once; ask for a new one."
	case strings.Contains(msg, "deadline"):
		return "The code expired. Ask for a new one."
	}
	return msg
}

func (s *addContactSheet) view() giode.Element {
	if !s.open && s.cancel != nil && s.state != inviteDone {
		// Closing the sheet withdraws the code.
		s.cancel()
		s.cancel = nil
		s.state = inviteIdle
	}
	return s.sheet
}

func (s *addContactSheet) body() giode.Element {
	var content giode.Element
	if s.mode.Selected() == 0 {
		content = s.shareView()
	} else {
		content = s.enterView()
	}
	return giode.Stack(giode.Styles{Width: -1, Gap: 20, Padding: giode.Inset{Bottom: 20}}, s.mode, content)
}

func (s *addContactSheet) shareView() giode.Element {
	switch s.state {
	case inviteOpening, inviteIdle:
		return centerNote(giode.Spinner(giode.Styles{Width: 28, Color: "theme.primary"}), "Creating a one-time code…")
	case inviteFailed:
		return giode.Stack(giode.Styles{Width: -1, Gap: 14},
			centerNote(icon("wifi-off", 30, "theme.danger"), s.err), s.retry)
	case inviteDone:
		name := s.friend.Name
		return giode.Stack(giode.Styles{Width: -1, Gap: 16, Align: giode.AlignCenter},
			s.a.avatarView(name, s.friend.Avatar, 72, true),
			text("You and "+name+" are connected", fsTitle, "theme.text", giode.FontWeightSemiBold),
			s.openNew)
	}
	status := "Waiting for your friend to enter it…"
	if s.state == inviteReceived {
		status = "Code received. Connecting your devices…"
	}
	return giode.Stack(giode.Styles{Width: -1, Gap: 18, Align: giode.AlignCenter},
		giode.Text("Read this code to your friend or send it to them. It works once and only introduces your devices.",
			giode.Styles{FontSize: fsSmall + 1, Color: "theme.muted", TextAlign: giode.TextAlignCenter, LineHeight: 20}),
		codeView(s.code),
		giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter},
			giode.Spinner(giode.Styles{Width: 14, Color: "theme.muted"}),
			text(status, fsSmall, "theme.muted")),
		giode.Row(giode.Styles{Gap: 10, Width: -1, Wrap: giode.WrapNoWrap},
			giode.Box(giode.Styles{FlexGrow: 1}, s.copy), giode.Box(giode.Styles{FlexGrow: 1}, s.share)),
	)
}

// codeView shows a wormhole code large, one word per chip.
func codeView(code string) giode.Element {
	parts := strings.Split(code, "-")
	chips := make([]giode.Element, 0, len(parts))
	for i, p := range parts {
		bg, fg := "theme.raised", "theme.text"
		if i == 0 {
			bg, fg = "theme.primary", "theme.onprimary"
		}
		chips = append(chips, giode.Box(giode.Styles{
			Padding: giode.SymmetricInset(14, 10), BorderRadius: 12, Background: bg,
		}, text(p, fsTitle+4, fg, giode.FontWeightBold)))
	}
	return giode.Row(giode.Styles{Gap: 8, Justify: giode.JustifyCenter, Align: giode.AlignCenter, Width: -1}, chips...)
}

func (s *addContactSheet) enterView() giode.Element {
	items := []giode.Element{
		giode.Text("Type the code your friend is showing. It's a number and two words.",
			giode.Styles{FontSize: fsSmall + 1, Color: "theme.muted", LineHeight: 20}),
		s.input,
	}
	if s.enterErr != "" {
		items = append(items, giode.Row(giode.Styles{Gap: 8, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
			icon("circle-alert", 16, "theme.danger"),
			giode.Box(giode.Styles{FlexGrow: 1}, text(s.enterErr, fsSmall, "theme.danger"))))
	}
	items = append(items, s.connect)
	return giode.Stack(giode.Styles{Width: -1, Gap: 14}, items...)
}

func centerNote(top giode.Element, note string) giode.Element {
	return giode.Stack(giode.Styles{Width: -1, Gap: 14, Align: giode.AlignCenter, Padding: giode.SymmetricInset(0, 16)},
		top, giode.Text(note, giode.Styles{FontSize: fsSmall + 1, Color: "theme.muted", TextAlign: giode.TextAlignCenter}))
}
