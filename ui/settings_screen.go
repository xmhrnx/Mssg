package ui

import (
	"strings"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/store"
)

var autoDownloadSteps = []int{0, 10, 25, 100}

// settingsScreen edits the profile and preferences.
type settingsScreen struct {
	a        *App
	back     *giode.IconButtonElement
	photo    *giode.PressableElement
	name     *giode.InputElement
	saveName *giode.ButtonElement
	copyID   *giode.ButtonElement
	theme    *giode.SegmentedElement
	receipts *giode.SwitchElement
	notify   *giode.SwitchElement
	online   *giode.SwitchElement
	auto     *giode.SegmentedElement
	turn     *giode.InputElement
	relay    *giode.SwitchElement
	loaded   bool
}

func newSettingsScreen(a *App) *settingsScreen {
	s := &settingsScreen{a: a}
	s.back = iconButton("arrow-left", 22, "Back", a.back)
	s.photo = giode.Pressable(func(st giode.PressState) giode.Element {
		p := a.svc.Profile()
		return giode.Box(giode.Styles{Width: 88, Height: 88},
			a.avatarView(p.Name, p.Avatar, 88, false),
			giode.Box(giode.Styles{Position: giode.PositionAbsolute, Right: giode.Flush, Bottom: giode.Flush, Width: 30, Height: 30, BorderRadius: 999,
				BackgroundGradient: gradientMine, Align: giode.AlignCenter, Justify: giode.JustifyCenter, BorderWidth: 2, BorderColor: "theme.background"},
				icon("camera", 15, "#FFFFFF")))
	}).Label("Change photo").OnClick(func(giode.ClickEvent) { go s.pickPhoto() })
	s.name = textField("Your name").Submit(func(string) { s.saveProfileName() })
	s.saveName = giode.Button("Save", giode.Styles{Background: "theme.primary", Color: "#FFFFFF", BorderRadius: 12}).OnClick(s.saveProfileName)
	s.copyID = secondaryButton("Copy my ID", func() {
		a.g.SetClipboard(a.svc.ID())
		a.toasts.Show(giode.ToastSuccess, "ID copied")
	}).Icon(icon("copy", 18, "theme.text"))
	s.theme = giode.Input.Segmented("System", "Light", "Dark").OnChange(func(i int) {
		s.update(func(set *store.Settings) { set.Theme = []string{"system", "light", "dark"}[i] })
		applyTheme(a.svc.Settings().Theme)
	})
	s.theme.Styles(giode.Styles{Width: -1})
	s.receipts = giode.Input.Switch("").OnChange(func(on bool) { s.update(func(set *store.Settings) { set.ReadReceipts = on }) })
	s.notify = giode.Input.Switch("").OnChange(func(on bool) { s.update(func(set *store.Settings) { set.NotifyOnMessages = on }) })
	s.online = giode.Input.Switch("").OnChange(func(on bool) {
		s.update(func(set *store.Settings) { set.StayOnline = on })
		a.applyStayOnline()
	})
	s.auto = giode.Input.Segmented("Off", "10 MB", "25 MB", "100 MB").OnChange(func(i int) {
		s.update(func(set *store.Settings) { set.AutoDownloadMB = autoDownloadSteps[i] })
	})
	s.auto.Styles(giode.Styles{Width: -1})
	s.turn = textField("turn:host:3478|user|password").Submit(func(string) { s.saveTURN() }).OnBlur(s.saveTURN)
	s.turn.Autocorrect(false)
	s.relay = giode.Input.Switch("").OnChange(func(on bool) { s.update(func(set *store.Settings) { set.RelayOnlyCalls = on }) })
	return s
}

// load copies the saved values into the controls once.
func (s *settingsScreen) load() {
	if s.loaded {
		return
	}
	s.loaded = true
	set := s.a.svc.Settings()
	s.name.SetText(s.a.svc.Profile().Name)
	s.theme.SetSelected(map[string]int{"system": 0, "light": 1, "dark": 2}[set.Theme])
	s.receipts.SetChecked(set.ReadReceipts)
	s.notify.SetChecked(set.NotifyOnMessages)
	s.online.SetChecked(set.StayOnline)
	for i, v := range autoDownloadSteps {
		if v == set.AutoDownloadMB {
			s.auto.SetSelected(i)
		}
	}
	s.turn.SetText(strings.Join(set.TURN, " "))
	s.relay.SetChecked(set.RelayOnlyCalls)
}

func (s *settingsScreen) update(fn func(*store.Settings)) {
	set := s.a.svc.Settings()
	fn(&set)
	if err := s.a.svc.SaveSettings(set); err != nil {
		s.a.toastError(err)
	}
}

func (s *settingsScreen) saveTURN() {
	fields := strings.Fields(s.turn.Text())
	for _, f := range fields {
		if !strings.HasPrefix(f, "turn:") && !strings.HasPrefix(f, "turns:") {
			s.a.toasts.Show(giode.ToastWarning, "TURN servers start with turn: or turns:")
			return
		}
	}
	s.update(func(set *store.Settings) { set.TURN = fields })
}

func (s *settingsScreen) saveProfileName() {
	name := strings.TrimSpace(s.name.Text())
	if name == "" {
		s.a.toasts.Show(giode.ToastWarning, "Your name can't be empty")
		return
	}
	avatar := s.a.svc.Profile().Avatar
	s.saveName.Loading(true)
	go func() {
		err := s.a.svc.SetProfile(name, avatar)
		s.a.g.Post(func() {
			s.saveName.Loading(false)
			if err != nil {
				s.a.toastError(err)
				return
			}
			s.a.toasts.Show(giode.ToastSuccess, "Profile updated")
		})
	}()
}

func (s *settingsScreen) pickPhoto() {
	f, err := s.a.g.PickPhoto(s.a.ctx(), giode.PhotoOptions{Source: giode.PhotoLibrary, MaxSide: 512})
	if err != nil {
		if !isCanceled(err) {
			s.a.g.Post(func() { s.a.toastError(err) })
		}
		return
	}
	defer f.Close()
	jpeg, err := avatarJPEG(f)
	if err == nil {
		err = s.a.svc.SetProfile(s.a.svc.Profile().Name, jpeg)
	}
	s.a.g.Post(func() { s.a.toastError(err) })
}

func (s *settingsScreen) view() giode.Element {
	s.load()
	id := s.a.svc.ID()
	profile := giode.Stack(giode.Styles{Width: -1, Gap: 14, Align: giode.AlignCenter, Padding: giode.SymmetricInset(0, 8)},
		s.photo,
		giode.Row(giode.Styles{Width: -1, Gap: 8, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
			giode.Box(giode.Styles{FlexGrow: 1}, s.name), s.saveName),
		giode.Stack(giode.Styles{Width: -1, Gap: 6, Padding: giode.UniformInset(14), Background: "theme.surface", BorderRadius: 16},
			giode.Row(giode.Styles{Gap: 6, Align: giode.AlignCenter}, icon("key-round", 14, "theme.muted"), text("Your permanent ID", fsSmall, "theme.muted", giode.FontWeightSemiBold)),
			giode.Text(groupID(id), giode.Styles{FontSize: fsSmall, Color: "theme.text", FontFamily: "Go Mono"}).Selectable("my-id"),
			s.copyID),
	)
	body := giode.Stack(giode.Styles{Width: -1, MaxWidth: 640, Gap: 22, Padding: giode.Inset{Left: 16, Right: 16, Bottom: 40}},
		profile,
		section("APPEARANCE", giode.Box(giode.Styles{Width: -1, Padding: giode.SymmetricInset(0, 10)}, s.theme)),
		section("PRIVACY",
			settingRow("Read receipts", "Let contacts see when you've read their messages", s.receipts),
			settingRow("Notifications", "Show new messages while the app is in the background", s.notify)),
		section("CONNECTION",
			settingRow("Stay online in the background", "Receive messages and calls with the app closed. Uses more battery.", s.online),
			giode.Stack(giode.Styles{Width: -1, Gap: 10, Padding: giode.SymmetricInset(0, 12)},
				text("Download media automatically up to", fsBody, "theme.text", giode.FontWeightMedium), s.auto)),
		section("CALLS",
			giode.Stack(giode.Styles{Width: -1, Gap: 8, Padding: giode.SymmetricInset(0, 12)},
				text("Your own TURN servers", fsBody, "theme.text", giode.FontWeightMedium),
				text("Calls connect directly. On networks that block that, a TURN server you run relays the encrypted media.", fsSmall, "theme.muted"),
				s.turn),
			settingRow("Relay every call", "Hide your IP address from contacts; needs a TURN server", s.relay)),
		section("HOW IT WORKS",
			giode.Text("Mssg has no servers or accounts. Your identity is a key made on this device. A one-time wormhole code introduces two devices; after that they find each other through the public IPFS network and talk directly, encrypted with Noise. Relays and calls' STUN servers only ever see ciphertext. Messages to someone offline wait on your device until you're both online.",
				giode.Styles{FontSize: fsSmall, Color: "theme.muted", LineHeight: 20, Padding: giode.SymmetricInset(0, 12)})),
	)
	return giode.Stack(giode.Styles{Width: -1, Height: -1},
		screenHeader(s.back, "Settings"),
		giode.Scroll(func() giode.Element {
			return giode.Row(giode.Styles{Width: -1, Justify: giode.JustifyCenter}, body)
		}, giode.Styles{Width: -1, FlexGrow: 1}),
	)
}
