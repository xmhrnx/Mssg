// Package ui is the app's interface: the chat list, conversations,
// calls and settings, on top of the messenger engine.
package ui

import (
	"context"
	"sort"
	"sync/atomic"
	"time"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/messenger"
	"github.com/xmhrnx/mssg/internal/store"
	"github.com/xmhrnx/mssg/internal/wire"
)

type phase int

const (
	phaseLoading phase = iota
	phaseOnboarding
	phaseReady
	phaseFailed
)

type screenID int

const (
	screenChats screenID = iota
	screenChat
	screenSettings
	screenContact
)

type route struct {
	screen screenID
	peer   string
}

// App is the root of the interface and the model it shows. Fields are
// owned by the UI goroutine; the engine's events reach them via Post.
type App struct {
	g      *giode.App
	svc    *messenger.Service
	images *imageCache
	toasts *giode.ToasterElement

	phase   phase
	failure string

	contacts []store.Contact
	last     map[string]store.Message
	online   map[string]bool
	typing   map[string]time.Time

	stack []route

	onboarding *onboardingScreen
	chats      *chatsScreen
	chat       *chatScreen
	settings   *settingsScreen
	contact    *contactScreen
	addContact *addContactSheet
	viewer     *mediaViewer
	calls      *callManager
	voice      *voicePlayer

	foreground atomic.Bool
	keepAlive  func()
}

// New creates the root; Attach connects it to the window.
func New() *App {
	a := &App{
		last:   map[string]store.Message{},
		online: map[string]bool{},
		typing: map[string]time.Time{},
		stack:  []route{{screen: screenChats}},
	}
	a.foreground.Store(true)
	return a
}

// Setup registers the theme; call it before creating the window.
func Setup() error { return setupTheme() }

// Attach builds the screens for the window and starts the engine.
func (a *App) Attach(g *giode.App) {
	a.g = g
	a.images = newImageCache(g)
	a.toasts = giode.Toaster().Placement(giode.ToastTopCenter).Duration(4 * time.Second).Max(2)
	a.onboarding = newOnboardingScreen(a)
	a.chats = newChatsScreen(a)
	a.chat = newChatScreen(a)
	a.settings = newSettingsScreen(a)
	a.contact = newContactScreen(a)
	a.addContact = newAddContactSheet(a)
	a.viewer = newMediaViewer(a)
	a.calls = newCallManager(a)
	a.voice = newVoicePlayer(a)
	applyTheme("system")
	a.registerAutomation()
	go a.start()
}

func (a *App) start() {
	go discoverLAN()
	dir, st, err := openData()
	if err != nil {
		a.g.Post(func() { a.fail(err) })
		return
	}
	ev := messenger.Events{
		ContactsChanged: func() { a.g.Post(a.reloadContacts) },
		Message:         func(m store.Message) { a.g.Post(func() { a.onMessage(m) }) },
		Presence: func(peer string, on bool) {
			a.g.Post(func() { a.online[peer] = on })
		},
		Typing: func(peer string) { a.g.Post(func() { a.typing[peer] = time.Now() }) },
		Call:   func(peer string, c wire.Call) { a.g.Post(func() { a.calls.onSignal(peer, c) }) },
		InviteRedeemed: func(c store.Contact) {
			a.g.Post(func() { a.onInviteRedeemed(c) })
		},
		Incoming: func(c store.Contact, m store.Message) { a.g.Post(func() { a.notifyIncoming(c, m) }) },
	}
	svc, err := messenger.New(context.Background(), messenger.Options{
		Store: st, DataDir: dir, DHTServer: giode.IsPlatform(giode.PlatformDesktop), Events: ev,
	})
	if err != nil {
		st.Close()
		a.g.Post(func() { a.fail(err) })
		return
	}
	a.g.Post(func() {
		a.svc = svc
		applyTheme(svc.Settings().Theme)
		if svc.Profile().Name == "" {
			a.phase = phaseOnboarding
		} else {
			a.phase = phaseReady
			a.applyStayOnline()
		}
		a.reloadContacts()
	})
}

func (a *App) fail(err error) {
	a.phase = phaseFailed
	a.failure = err.Error()
}

// reloadContacts reads contacts and their last messages off the UI
// goroutine. Call it on the UI goroutine.
func (a *App) reloadContacts() {
	svc := a.svc
	if svc == nil {
		return
	}
	go func() {
		cs, err := svc.Contacts()
		if err != nil {
			a.g.Post(func() { a.toastError(err) })
			return
		}
		last := map[string]store.Message{}
		for _, c := range cs {
			if m, ok, err := svc.Store().LastMessage(c.PeerID); err == nil && ok {
				last[c.PeerID] = m
			}
		}
		online := map[string]bool{}
		for _, c := range cs {
			online[c.PeerID] = svc.Online(c.PeerID)
		}
		a.g.Post(func() {
			a.contacts, a.last, a.online = cs, last, online
			a.sortContacts()
		})
	}()
}

// sortContacts orders chats by their latest activity.
func (a *App) sortContacts() {
	at := func(c store.Contact) time.Time {
		if m, ok := a.last[c.PeerID]; ok {
			return m.Time
		}
		return c.Added
	}
	sort.SliceStable(a.contacts, func(i, j int) bool { return at(a.contacts[i]).After(at(a.contacts[j])) })
}

func (a *App) onMessage(m store.Message) {
	if l, ok := a.last[m.Peer]; !ok || m.Seq >= l.Seq {
		a.last[m.Peer] = m
		a.sortContacts()
	}
	delete(a.typing, m.Peer)
	a.chat.onMessage(m)
}

func (a *App) contactByID(peer string) (store.Contact, bool) {
	for _, c := range a.contacts {
		if c.PeerID == peer {
			return c, true
		}
	}
	return store.Contact{}, false
}

// isTyping reports a typing signal from the last few seconds.
func (a *App) isTyping(peer string) bool {
	t, ok := a.typing[peer]
	return ok && time.Since(t) < 5*time.Second
}

func (a *App) onInviteRedeemed(c store.Contact) {
	a.addContact.redeemed(c)
	a.toasts.Show(giode.ToastSuccess, c.Name+" is now a contact")
	giode.Haptic(giode.HapticSuccess)
}

func (a *App) notifyIncoming(c store.Contact, m store.Message) {
	if a.svc == nil || c.Muted || !a.svc.Settings().NotifyOnMessages {
		return
	}
	if a.foreground.Load() {
		return
	}
	// With notifications off or unsupported, the unread badge is all
	// that's left; there's no one to tell while the app is hidden.
	_ = a.g.Notify(c.Name, preview(m))
}

func (a *App) toastError(err error) {
	if err != nil {
		a.toasts.Show(giode.ToastError, err.Error())
	}
}

// push opens a screen over the current one.
func (a *App) push(r route) {
	a.stack = append(a.stack, r)
	a.routeChanged()
}

// back returns to the previous screen.
func (a *App) back() {
	if len(a.stack) > 1 {
		a.stack = a.stack[:len(a.stack)-1]
		a.routeChanged()
	}
}

// home returns to the chat list.
func (a *App) home() {
	a.stack = a.stack[:1]
	a.routeChanged()
}

func (a *App) top() route { return a.stack[len(a.stack)-1] }

func (a *App) routeChanged() {
	r := a.top()
	if r.screen == screenChat {
		a.chat.open(r.peer)
	} else {
		a.chat.close()
	}
	if r.screen == screenContact {
		a.contact.open(r.peer)
	}
}

// openChat shows a conversation.
func (a *App) openChat(peer string) {
	if a.top().screen == screenChat && a.top().peer == peer {
		return
	}
	a.stack = a.stack[:1]
	a.push(route{screen: screenChat, peer: peer})
}

// OnBackground is called when the app leaves the screen.
func (a *App) OnBackground() {
	a.foreground.Store(false)
	if a.svc != nil {
		a.svc.SetActiveChat("")
	}
}

// OnForeground is called when the app returns.
func (a *App) OnForeground() {
	a.foreground.Store(true)
	a.g.Post(func() {
		if a.svc != nil && a.top().screen == screenChat {
			a.svc.SetActiveChat(a.top().peer)
		}
		if a.svc != nil {
			a.svc.Refresh()
			go discoverLAN()
		}
	})
}

// applyStayOnline holds the foreground service while the setting is on,
// so messages and calls arrive with the app in the background.
func (a *App) applyStayOnline() {
	want := a.svc.Settings().StayOnline
	switch {
	case want && a.keepAlive == nil:
		release, err := a.g.KeepAlive("Online for messages and calls")
		if err != nil {
			a.toasts.Show(giode.ToastWarning, "Can't stay online in the background on this device")
			return
		}
		a.keepAlive = release
	case !want && a.keepAlive != nil:
		a.keepAlive()
		a.keepAlive = nil
	}
}

// View is the window's content, built every frame.
func (a *App) View() giode.Element {
	var page giode.Element
	switch a.phase {
	case phaseLoading:
		page = splash("")
	case phaseFailed:
		page = splash(a.failure)
	case phaseOnboarding:
		page = a.onboarding.view()
	default:
		page = a.readyView()
	}
	return giode.Box(giode.Styles{Width: -1, Height: -1, Background: "theme.background"},
		page, a.toasts)
}

func (a *App) readyView() giode.Element {
	if len(a.stack) > 1 {
		giode.HandleBack(a.back)
	}
	var page giode.Element
	switch r := a.top(); r.screen {
	case screenChat:
		page = a.chat.view()
	case screenSettings:
		page = a.settings.view()
	case screenContact:
		page = a.contact.view()
	default:
		page = a.chats.view()
	}
	return giode.Box(giode.Styles{Width: -1, Height: -1},
		page, a.addContact.view(), a.viewer.view(), a.calls.view())
}
