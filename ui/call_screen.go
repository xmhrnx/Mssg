package ui

import (
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/camera"
	"github.com/xmhrnx/giode/video"
)

// callScreen is the full-window call view: the other side's video or
// picture, your camera in a corner, and the controls.
type callScreen struct {
	a *App
	m *callManager

	accept, decline, hangUp *giode.IconButtonElement
	mute, camera, flip      *giode.IconButtonElement
	muteOn, cameraOff       *giode.IconButtonElement
	pulse                   *giode.KeyframeAnimation
}

func newCallScreen(a *App, m *callManager) *callScreen {
	s := &callScreen{a: a, m: m}
	s.accept = roundAction("phone", 72, "Answer", gradientAccept, m.accept)
	s.decline = roundAction("phone-off", 72, "Decline", gradientDanger, m.decline)
	s.hangUp = roundAction("phone-off", 68, "Hang up", gradientDanger, m.hangUp)
	s.mute = glassButton("mic", "Mute", func() { m.setMuted(true) })
	s.muteOn = glassButtonOn("mic-off", "Unmute", func() { m.setMuted(false) })
	s.camera = glassButton("video", "Turn camera off", m.toggleCamera)
	s.cameraOff = glassButtonOn("video-off", "Turn camera on", m.toggleCamera)
	s.flip = glassButton("switch-camera", "Switch camera", m.flipCamera)
	s.pulse = giode.Keyframes(1800*time.Millisecond,
		giode.Keyframe{At: 0, Styles: giode.Styles{Transform: giode.Transform{Scale: 1}, Opacity: 0.55}},
		giode.Keyframe{At: 1, Styles: giode.Styles{Transform: giode.Transform{Scale: 1.6}, Opacity: 0.01}},
	).Repeat(0)
	return s
}

// glassButton is a translucent round control over video.
func glassButton(ic, label string, onClick func()) *giode.IconButtonElement {
	return giode.IconButton(icon(ic, 24, "#FFFFFF"), giode.Styles{
		Width: 58, Height: 58, BorderRadius: 999, Background: "#FFFFFF2E",
	}).Label(label).OnClick(onClick)
}

// glassButtonOn is a glassButton in its engaged state: solid white.
func glassButtonOn(ic, label string, onClick func()) *giode.IconButtonElement {
	return giode.IconButton(icon(ic, 24, "#0B0D12"), giode.Styles{
		Width: 58, Height: 58, BorderRadius: 999, Background: "#FFFFFF",
	}).Label(label).OnClick(onClick)
}

func (s *callScreen) view() giode.Element {
	m := s.m
	if m.phase == callIdle {
		return giode.Box(giode.Styles{Display: giode.DisplayNone})
	}
	// Back doesn't drop a call by accident.
	giode.HandleBack(func() {})
	c, _ := s.a.contactByID(m.peer)
	layers := []giode.Element{s.backdrop(c.Name, c.Avatar)}
	if m.cam != nil {
		layers = append(layers, giode.Box(giode.Styles{
			Position: giode.PositionAbsolute, Top: 56, Right: 16, ZIndex: 2,
			Width: 112, Height: 160, BorderRadius: 18, Overflow: giode.OverflowHidden,
			BorderWidth: 2, BorderColor: "#FFFFFF40", BoxShadow: giode.BoxShadow{Y: 8, Blur: 24, Color: "#00000080"},
		}, m.cam.Preview(camera.PreviewOptions{Fill: true, NoTapFocus: true})))
	}
	layers = append(layers, giode.Box(giode.Styles{Position: giode.PositionAbsolute, Left: giode.Flush, Right: giode.Flush, Bottom: giode.Flush, ZIndex: 3},
		s.controls()))
	return giode.Box(giode.Styles{Position: giode.PositionAbsolute, Top: giode.Flush, Left: giode.Flush, Right: giode.Flush, Bottom: giode.Flush,
		ZIndex: 60, Background: "#07080C"}, layers...)
}

func (s *callScreen) showsRemoteVideo() bool {
	c := s.m.call.Load()
	return c != nil && s.m.phase == callActive && s.m.remote && c.RemoteVideo().Size().X > 0
}

// backdrop is the remote video when there is some, else their picture
// on a gradient with the call's state.
func (s *callScreen) backdrop(name string, avatar []byte) giode.Element {
	m := s.m
	status := m.status
	if m.phase == callActive && status == "" {
		status = duration(time.Since(m.started))
	}
	header := giode.Stack(giode.Styles{Align: giode.AlignCenter, Gap: 6},
		text(name, fsHuge, "#FFFFFF", giode.FontWeightSemiBold),
		giode.Row(giode.Styles{Gap: 6, Align: giode.AlignCenter},
			icon("lock", 13, "#FFFFFFAA"), text(status, fsBody, "#FFFFFFCC")),
	)
	if s.showsRemoteVideo() {
		return giode.Box(giode.Styles{Width: -1, Height: -1},
			m.call.Load().RemoteVideo().View(video.ViewOptions{Fit: video.Cover, Label: name + "'s camera"}),
			giode.Box(giode.Styles{Position: giode.PositionAbsolute, Top: giode.Flush, Left: giode.Flush, Right: giode.Flush,
				Padding: giode.Inset{Top: 24, Bottom: 40}, BackgroundGradient: giode.Linear(180, "#000000AA", "#00000000"), Align: giode.AlignCenter, FlexDirection: giode.FlexDirectionColumn},
				header))
	}
	ringing := m.phase == callIncoming || m.phase == callOutgoing
	av := []giode.Element{}
	if ringing {
		av = append(av, giode.Box(giode.Styles{Position: giode.PositionAbsolute, Top: giode.Flush, Left: giode.Flush,
			Width: 148, Height: 148, BorderRadius: 999, Background: "#7C5CFF"}).Animation(s.pulse))
	}
	av = append(av, giode.Box(giode.Styles{Position: giode.PositionAbsolute, Top: 14, Left: 14}, s.a.avatarView(name, avatar, 120, false)))
	return giode.Stack(giode.Styles{Width: -1, Height: -1, Align: giode.AlignCenter, Justify: giode.JustifyCenter, Gap: 28,
		BackgroundGradient: giode.Linear(170, "#1A1340", "#07080C")},
		giode.Box(giode.Styles{Width: 148, Height: 148}, av...),
		header,
		giode.Box(giode.Styles{Height: 160}),
	)
}

func (s *callScreen) controls() giode.Element {
	m := s.m
	pad := giode.Inset{Top: 32, Bottom: 40, Left: 24, Right: 24}
	bg := giode.Linear(0, "#000000CC", "#00000000")
	if m.phase == callIncoming {
		return giode.Row(giode.Styles{Width: -1, Justify: giode.JustifySpaceEvenly, Align: giode.AlignCenter, Padding: pad, BackgroundGradient: bg},
			labeled(s.decline, "Decline"), labeled(s.accept, "Answer"))
	}
	mute := s.mute
	if m.muted {
		mute = s.muteOn
	}
	cam := s.cameraOff
	if m.camOn {
		cam = s.camera
	}
	row := []giode.Element{mute, cam}
	if m.camOn {
		row = append(row, s.flip)
	}
	return giode.Stack(giode.Styles{Width: -1, Align: giode.AlignCenter, Gap: 26, Padding: pad, BackgroundGradient: bg},
		giode.Row(giode.Styles{Gap: 22, Justify: giode.JustifyCenter, Align: giode.AlignCenter}, row...),
		s.hangUp,
	)
}

func labeled(el giode.Element, label string) giode.Element {
	return giode.Stack(giode.Styles{Gap: 10, Align: giode.AlignCenter}, el, text(label, fsSmall, "#FFFFFFCC"))
}
