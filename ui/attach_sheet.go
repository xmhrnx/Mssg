package ui

import (
	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/takephoto"
)

// attachSheet offers what can be sent: photos and videos, the camera,
// GIFs and any file.
type attachSheet struct {
	a     *App
	open  bool
	peer  string
	sheet *giode.SheetElement
	rows  []*giode.PressableElement
}

type attachOption struct {
	icon, title, sub string
	gradient         *giode.LinearGradient
	pick             func(peer string)
}

func newAttachSheet(a *App) *attachSheet {
	s := &attachSheet{a: a}
	opts := []attachOption{
		{"image", "Photos & videos", "From your gallery, at full quality", giode.Linear(135, "#7C5CFF", "#B15CFF"), s.pickMedia},
		{"camera", "Camera", "Take a photo now", giode.Linear(135, "#22D3EE", "#4F7BFF"), s.takePhoto},
		{"sparkles", "GIF", "Send an animated GIF", giode.Linear(135, "#FF7A59", "#FF5470"), s.pickGIF},
		{"file", "File", "Documents, archives, anything", giode.Linear(135, "#2DD4A4", "#22C3EE"), s.pickFiles},
	}
	for _, o := range opts {
		o := o
		row := giode.Pressable(func(st giode.PressState) giode.Element { return attachRow(o, st) }).
			Label(o.title).
			OnClick(func(giode.ClickEvent) {
				s.open = false
				peer := s.peer
				go o.pick(peer)
			})
		s.rows = append(s.rows, row)
	}
	s.sheet = giode.Sheet(&s.open, func() giode.Element {
		items := make([]giode.Element, len(s.rows))
		for i, r := range s.rows {
			items[i] = r
		}
		return giode.Stack(giode.Styles{Width: -1, Gap: 4, Padding: giode.Inset{Bottom: 16}}, items...)
	}).Title("Send").Adaptive(true)
	return s
}

func attachRow(o attachOption, st giode.PressState) giode.Element {
	bg := "theme.surface"
	if st.Pressed || st.Hovered {
		bg = pressTint
	}
	return giode.Row(giode.Styles{Width: -1, Gap: 14, Align: giode.AlignCenter, Padding: giode.SymmetricInset(8, 10), BorderRadius: 14, Background: bg, Wrap: giode.WrapNoWrap},
		giode.Box(giode.Styles{Width: 46, Height: 46, BorderRadius: 14, BackgroundGradient: o.gradient, Align: giode.AlignCenter, Justify: giode.JustifyCenter},
			icon(o.icon, 22, "#FFFFFF")),
		giode.Stack(giode.Styles{Gap: 2, FlexGrow: 1},
			text(o.title, fsBody, "theme.text", giode.FontWeightSemiBold),
			text(o.sub, fsSmall, "theme.muted")),
	)
}

func (s *attachSheet) show(peer string) {
	s.peer = peer
	s.open = true
}

func (s *attachSheet) view() giode.Element { return s.sheet }

func (s *attachSheet) report(err error) {
	if err != nil && !isCanceled(err) {
		s.a.g.Post(func() { s.a.toastError(err) })
	}
}

func (s *attachSheet) pickMedia(peer string) {
	files, err := s.a.g.PickMedia(s.a.ctx(), giode.MediaOptions{})
	if err != nil {
		s.report(err)
		return
	}
	s.a.sendPicked(peer, files)
}

func (s *attachSheet) takePhoto(peer string) {
	f, err := takephoto.Take(s.a.ctx(), s.a.g, 2560)
	if err != nil {
		s.report(err)
		return
	}
	s.a.sendPicked(peer, []giode.PickedFile{f})
}

func (s *attachSheet) pickGIF(peer string) {
	files, err := s.a.g.PickFiles(s.a.ctx(), giode.PickOptions{Types: []string{"image/gif"}, Multiple: true})
	if err != nil {
		s.report(err)
		return
	}
	s.a.sendPicked(peer, files)
}

func (s *attachSheet) pickFiles(peer string) {
	files, err := s.a.g.PickFiles(s.a.ctx(), giode.PickOptions{Multiple: true})
	if err != nil {
		s.report(err)
		return
	}
	s.a.sendPicked(peer, files)
}
