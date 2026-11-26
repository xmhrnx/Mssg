package ui

import (
	"context"
	"io"
	"strings"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/mediakit"
)

// onboardingScreen asks for a name (and optionally a photo) on first run.
type onboardingScreen struct {
	a      *App
	name   *giode.InputElement
	photo  *giode.PressableElement
	cont   *giode.ButtonElement
	avatar []byte
	err    string
}

func newOnboardingScreen(a *App) *onboardingScreen {
	s := &onboardingScreen{a: a}
	s.name = textField("Your name").Submit(func(string) { s.finish() })
	s.cont = primaryButton("Get started", s.finish)
	s.photo = giode.Pressable(func(st giode.PressState) giode.Element { return s.photoView(st) }).
		Label("Choose a profile photo").OnClick(func(giode.ClickEvent) { go s.pickPhoto() })
	return s
}

func (s *onboardingScreen) pickPhoto() {
	f, err := s.a.g.PickPhoto(context.Background(), giode.PhotoOptions{Source: giode.PhotoLibrary, MaxSide: 512})
	if err != nil {
		if !isCanceled(err) {
			s.a.g.Post(func() { s.a.toastError(err) })
		}
		return
	}
	defer f.Close()
	jpeg, err := avatarJPEG(f)
	s.a.g.Post(func() {
		if err != nil {
			s.a.toastError(err)
			return
		}
		s.avatar = jpeg
	})
}

// avatarJPEG turns a picked photo into a small square-ish JPEG.
func avatarJPEG(r io.Reader) ([]byte, error) {
	_, _, thumb, err := mediakit.Preview(r)
	return thumb, err
}

func (s *onboardingScreen) finish() {
	name := strings.TrimSpace(s.name.Text())
	if name == "" {
		s.err = "Pick a name your contacts will see."
		s.name.Focus()
		return
	}
	s.cont.Loading(true)
	avatar := s.avatar
	go func() {
		err := s.a.svc.SetProfile(name, avatar)
		s.a.g.Post(func() {
			s.cont.Loading(false)
			if err != nil {
				s.err = err.Error()
				return
			}
			s.a.phase = phaseReady
			s.a.applyStayOnline()
		})
	}()
}

func (s *onboardingScreen) photoView(st giode.PressState) giode.Element {
	var inner giode.Element
	if img := s.a.images.FromBytes(s.avatar); img != nil {
		inner = giode.Image(img, giode.Styles{Width: 96, Height: 96, Fit: giode.FitCover, BorderRadius: 999})
	} else {
		inner = icon("camera", 30, "theme.muted")
	}
	bg := "theme.raised"
	if st.Pressed {
		bg = pressTint
	}
	return giode.Box(giode.Styles{
		Width: 96, Height: 96, BorderRadius: 999, Background: bg,
		Align: giode.AlignCenter, Justify: giode.JustifyCenter, Overflow: giode.OverflowHidden,
	}, inner)
}

func (s *onboardingScreen) view() giode.Element {
	features := giode.Stack(giode.Styles{Gap: 14, Width: -1},
		feature("lock", "End-to-end encrypted", "Only you and the person you talk to can read it."),
		feature("wifi", "Peer to peer", "Messages, files and calls go straight between devices."),
		feature("key-round", "No accounts", "Your identity is a key that never leaves this device."),
	)
	form := []giode.Element{
		giode.Row(giode.Styles{Justify: giode.JustifyCenter, Width: -1}, s.photo),
		s.name,
	}
	if s.err != "" {
		form = append(form, text(s.err, fsSmall, "theme.danger"))
	}
	form = append(form, s.cont)
	return giode.Scroll(func() giode.Element {
		return giode.Stack(giode.Styles{Width: -1, Align: giode.AlignCenter, Padding: giode.Inset{Top: 48, Bottom: 32, Left: 24, Right: 24}},
			giode.Stack(giode.Styles{Width: -1, MaxWidth: 440, Gap: 28, Align: giode.AlignCenter},
				logoMark(84),
				giode.Stack(giode.Styles{Gap: 8, Align: giode.AlignCenter},
					text("Mssg", fsHuge+6, "theme.text", giode.FontWeightBold),
					giode.Text("Private messaging without servers.", giode.Styles{FontSize: fsBody, Color: "theme.muted", TextAlign: giode.TextAlignCenter}),
				),
				features,
				giode.Stack(giode.Styles{Gap: 14, Width: -1}, form...),
			),
		)
	}, giode.Styles{Width: -1, Height: -1})
}

func feature(ic, title, body string) giode.Element {
	return giode.Row(giode.Styles{Gap: 14, Align: giode.AlignCenter, Width: -1, Wrap: giode.WrapNoWrap},
		giode.Box(giode.Styles{Width: 42, Height: 42, BorderRadius: 12, Background: "theme.raised", Align: giode.AlignCenter, Justify: giode.JustifyCenter},
			icon(ic, 20, "theme.primary")),
		giode.Stack(giode.Styles{Gap: 2, FlexGrow: 1},
			text(title, fsBody, "theme.text", giode.FontWeightSemiBold),
			text(body, fsSmall, "theme.muted"),
		),
	)
}
