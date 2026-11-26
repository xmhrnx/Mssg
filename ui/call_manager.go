package ui

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/camera"
	"github.com/xmhrnx/giode/rtc"
	"github.com/xmhrnx/giode/telecom"

	"github.com/xmhrnx/mssg/internal/store"
	"github.com/xmhrnx/mssg/internal/wire"
)

type callPhase int

const (
	callIdle callPhase = iota
	callOutgoing
	callIncoming
	callConnecting
	callActive
)

const ringTimeout = 45 * time.Second

// callManager runs voice and video calls: WebRTC media (DTLS-SRTP
// encrypted end to end), signaled over the contacts' libp2p channel,
// with the phone's system call screen where there is one.
type callManager struct {
	a        *App
	phase    callPhase
	id       string
	peer     string
	video    bool
	outgoing bool
	offer    string
	status   string
	started  time.Time
	gen      int

	call    atomic.Pointer[rtc.Call]
	cam     *camera.Camera
	camOn   bool
	facing  camera.Facing
	muted   bool
	remote  bool // the other side's camera is on
	ringing chan struct{}
	ticking chan struct{}

	// ICE candidates from the other side that arrived before its
	// session description was applied, by call ID.
	early     map[string][]string
	remoteSet bool

	screen *callScreen
}

func newCallManager(a *App) *callManager {
	m := &callManager{a: a, facing: camera.Front, early: map[string][]string{}}
	m.screen = newCallScreen(a, m)
	telecom.Setup(a.g, telecom.Handler{
		OnAnswer: func(id string) {
			if id == m.id && m.phase == callIncoming {
				m.accept()
			}
		},
		OnEnd: func(id string) {
			if id != m.id {
				return
			}
			if m.phase == callIncoming {
				m.decline()
			} else {
				m.hangUp()
			}
		},
		OnMute: func(id string, muted bool) {
			if id == m.id {
				m.setMuted(muted)
			}
		},
	})
	return m
}

func (m *callManager) iceServers() ([]rtc.ICEServer, bool) {
	set := m.a.svc.Settings()
	var out []rtc.ICEServer
	if len(set.STUN) > 0 {
		out = append(out, rtc.ICEServer{URLs: set.STUN})
	}
	for _, t := range set.TURN {
		parts := strings.SplitN(t, "|", 3)
		s := rtc.ICEServer{URLs: []string{parts[0]}}
		if len(parts) == 3 {
			s.Username, s.Credential = parts[1], parts[2]
		}
		out = append(out, s)
	}
	return out, set.RelayOnlyCalls && len(set.TURN) > 0
}

func (m *callManager) newRTC(ctx context.Context, gen int) (*rtc.Call, error) {
	ice, relay := m.iceServers()
	cfg := rtc.Config{
		ICEServers: ice, RelayOnly: relay,
		Microphone: true, Speaker: true, SpeakerPhone: m.video,
		VideoCodecs: []rtc.VideoCodec{rtc.VP8}, SendVideo: true, VideoFPS: 15,
		OnState: func(s rtc.State) { m.a.g.Post(func() { m.onState(gen, s) }) },
		// Trickle ICE: candidates go out as they're found, so the call
		// rings at once instead of after gathering.
		OnCandidate: func(cand string) {
			m.a.g.Post(func() {
				if gen == m.gen {
					m.sendCandidate(cand)
				}
			})
		},
	}
	c, err := rtc.New(ctx, m.a.g, cfg)
	if err == nil {
		return c, nil
	}
	// A computer without a working microphone or speaker can still
	// see and be seen.
	cfg.Microphone, cfg.Speaker = false, false
	c, err2 := rtc.New(ctx, m.a.g, cfg)
	if err2 != nil {
		return nil, err
	}
	m.a.g.Post(func() { m.a.toasts.Show(giode.ToastWarning, "No audio device: "+err.Error()) })
	return c, nil
}

// start calls a contact.
func (m *callManager) start(peer string, video bool) {
	if m.phase != callIdle {
		m.a.toasts.Show(giode.ToastWarning, "You're already in a call")
		return
	}
	if !m.a.online[peer] {
		m.a.toasts.Show(giode.ToastInfo, "Trying to reach them…")
	}
	m.reset(peer, video, true)
	m.id = telecom.NewID()
	m.phase, m.status = callOutgoing, "Calling…"
	if video {
		m.openCamera()
	}
	m.startRinging(false)
	gen, id := m.gen, m.id
	go func() {
		ctx, cancel := context.WithTimeout(m.a.ctx(), 40*time.Second)
		defer cancel()
		c, err := m.newRTC(ctx, gen)
		if err != nil {
			m.a.g.Post(func() { m.failed(gen, err) })
			return
		}
		m.call.Store(c)
		offer, err := c.Offer(ctx)
		if err == nil {
			err = m.a.svc.SendCall(ctx, peer, wire.Call{ID: id, Action: wire.CallOffer, Video: video, SDP: offer})
		}
		if err != nil {
			m.a.g.Post(func() { m.failed(gen, err) })
		}
	}()
	go m.timeout(gen)
}

// timeout ends an unanswered call.
func (m *callManager) timeout(gen int) {
	time.Sleep(ringTimeout)
	m.a.g.Post(func() {
		if gen != m.gen {
			return
		}
		switch m.phase {
		case callOutgoing:
			m.signal(wire.CallEnd)
			m.log(store.CallInfo{Video: m.video, Reason: "unanswered"})
			m.finish("No answer")
		case callIncoming:
			m.log(store.CallInfo{Video: m.video, Missed: true})
			m.finish("")
		}
	})
}

func (m *callManager) failed(gen int, err error) {
	if gen != m.gen || m.phase == callIdle {
		return
	}
	reason := "Couldn't connect"
	if m.phase == callOutgoing {
		reason = "They're unreachable right now"
	}
	m.log(store.CallInfo{Video: m.video, Reason: err.Error()})
	m.finish(reason)
}

func (m *callManager) sendCandidate(cand string) {
	peer, id := m.peer, m.id
	go func() {
		ctx, cancel := context.WithTimeout(m.a.ctx(), 15*time.Second)
		defer cancel()
		m.a.svc.SendCall(ctx, peer, wire.Call{ID: id, Action: wire.CallCandidate, SDP: cand})
	}()
}

// addCandidate applies a remote candidate, or keeps it until the remote
// description is set.
func (m *callManager) addCandidate(id, cand string) {
	if id == m.id && m.remoteSet {
		if c := m.call.Load(); c != nil {
			c.AddCandidate(cand)
		}
		return
	}
	if len(m.early) > 16 {
		clear(m.early)
	}
	m.early[id] = append(m.early[id], cand)
}

// remoteReady marks the remote description applied and replays the
// candidates that came early.
func (m *callManager) remoteReady() {
	m.remoteSet = true
	c := m.call.Load()
	for _, cand := range m.early[m.id] {
		if c != nil {
			c.AddCandidate(cand)
		}
	}
	delete(m.early, m.id)
}

// onSignal handles a call frame from a contact.
func (m *callManager) onSignal(peer string, c wire.Call) {
	switch c.Action {
	case wire.CallOffer:
		m.incoming(peer, c)
		return
	case wire.CallCandidate:
		if peer == m.peer || m.phase == callIdle {
			m.addCandidate(c.ID, c.SDP)
		}
		return
	}
	if peer != m.peer || c.ID != m.id {
		return
	}
	switch c.Action {
	case wire.CallRing:
		if m.phase == callOutgoing {
			m.status = "Ringing…"
		}
	case wire.CallAnswer:
		if m.phase != callOutgoing {
			return
		}
		m.stopRinging()
		m.phase, m.status = callConnecting, "Connecting…"
		if call := m.call.Load(); call != nil {
			if err := call.Accept(c.SDP); err != nil {
				m.failed(m.gen, err)
				return
			}
			m.remoteReady()
		}
	case wire.CallReject:
		m.log(store.CallInfo{Video: m.video, Declined: true})
		m.finish("Declined")
	case wire.CallBusy:
		m.log(store.CallInfo{Video: m.video, Reason: "busy"})
		m.finish("Busy on another call")
	case wire.CallEnd:
		switch m.phase {
		case callIncoming:
			m.log(store.CallInfo{Video: m.video, Missed: true})
			telecom.ReportEnded(m.id, telecom.RemoteEnded)
			m.finish("")
		default:
			m.log(store.CallInfo{Video: m.video, Seconds: m.seconds()})
			telecom.ReportEnded(m.id, telecom.RemoteEnded)
			m.finish("Call ended")
		}
	case wire.CallMedia:
		m.remote = c.Video
	}
}

func (m *callManager) incoming(peer string, c wire.Call) {
	if m.phase != callIdle {
		go m.a.svc.SendCall(m.a.ctx(), peer, wire.Call{ID: c.ID, Action: wire.CallBusy})
		return
	}
	contact, ok := m.a.contactByID(peer)
	if !ok {
		return
	}
	m.reset(peer, c.Video, false)
	m.id, m.offer = c.ID, c.SDP
	m.phase, m.status = callIncoming, "Incoming call"
	if c.Video {
		m.status = "Incoming video call"
	}
	m.remote = c.Video
	go m.a.svc.SendCall(m.a.ctx(), peer, wire.Call{ID: c.ID, Action: wire.CallRing})
	err := telecom.ReportIncoming(m.a.ctx(), telecom.Call{ID: c.ID, Handle: peer, Name: contact.Name, Video: c.Video})
	if err != nil {
		// No system call screen: ring in the app.
		m.startRinging(true)
		if !m.a.foreground.Load() {
			_ = m.a.g.Notify(contact.Name, m.status)
		}
	}
	go m.timeout(m.gen)
}

// accept answers the ringing call.
func (m *callManager) accept() {
	if m.phase != callIncoming {
		return
	}
	m.stopRinging()
	m.phase, m.status = callConnecting, "Connecting…"
	if m.video {
		m.openCamera()
	}
	gen, peer, id, offer := m.gen, m.peer, m.id, m.offer
	go func() {
		ctx, cancel := context.WithTimeout(m.a.ctx(), 40*time.Second)
		defer cancel()
		c, err := m.newRTC(ctx, gen)
		if err != nil {
			m.a.g.Post(func() { m.failed(gen, err) })
			return
		}
		m.call.Store(c)
		answer, err := c.Answer(ctx, offer)
		if err != nil {
			m.a.g.Post(func() { m.failed(gen, err) })
			return
		}
		m.a.g.Post(func() {
			if gen == m.gen {
				m.remoteReady()
			}
		})
		if err := m.a.svc.SendCall(ctx, peer, wire.Call{ID: id, Action: wire.CallAnswer, SDP: answer}); err != nil {
			m.a.g.Post(func() { m.failed(gen, err) })
		}
	}()
}

func (m *callManager) decline() {
	if m.phase != callIncoming {
		return
	}
	m.signal(wire.CallReject)
	telecom.End(m.id)
	m.log(store.CallInfo{Video: m.video, Declined: true})
	m.finish("")
}

// hangUp ends the call from this side.
func (m *callManager) hangUp() {
	if m.phase == callIdle {
		return
	}
	m.signal(wire.CallEnd)
	telecom.End(m.id)
	m.log(store.CallInfo{Video: m.video, Seconds: m.seconds()})
	m.finish("Call ended")
}

func (m *callManager) onState(gen int, s rtc.State) {
	if gen != m.gen {
		return
	}
	switch s {
	case rtc.StateConnected:
		if m.phase != callActive {
			m.phase, m.started = callActive, time.Now()
			telecom.ReportConnected(m.id)
			m.ticking = make(chan struct{})
			go ticker(m.a.g, time.Second, m.ticking)
			giode.Haptic(giode.HapticSuccess)
		}
		m.status = ""
	case rtc.StateDisconnected:
		m.status = "Reconnecting…"
	case rtc.StateFailed:
		m.log(store.CallInfo{Video: m.video, Seconds: m.seconds(), Reason: "connection failed"})
		telecom.ReportEnded(m.id, telecom.Failed)
		m.signal(wire.CallEnd)
		m.finish("Connection lost")
	}
}

// signal sends a call frame to the other side without waiting.
func (m *callManager) signal(action wire.CallAction) {
	peer, id := m.peer, m.id
	go func() {
		ctx, cancel := context.WithTimeout(m.a.ctx(), 15*time.Second)
		defer cancel()
		m.a.svc.SendCall(ctx, peer, wire.Call{ID: id, Action: action, Video: m.camOn})
	}()
}

func (m *callManager) seconds() int {
	if m.started.IsZero() {
		return 0
	}
	return int(time.Since(m.started).Seconds())
}

// log records the call in the conversation.
func (m *callManager) log(info store.CallInfo) {
	peer, out := m.peer, m.outgoing
	go func() {
		if _, err := m.a.svc.AddCallLog(peer, out, info); err != nil {
			m.a.g.Post(func() { m.a.toastError(err) })
		}
	}()
}

func (m *callManager) reset(peer string, video, outgoing bool) {
	m.gen++
	m.peer, m.video, m.outgoing = peer, video, outgoing
	m.started = time.Time{}
	m.muted, m.remote, m.remoteSet = false, false, false
	m.facing = camera.Front
}

// finish tears the call down; reason shows briefly in a toast.
func (m *callManager) finish(reason string) {
	m.gen++
	m.stopRinging()
	if m.ticking != nil {
		close(m.ticking)
		m.ticking = nil
	}
	m.closeCamera()
	if c := m.call.Swap(nil); c != nil {
		go c.Close()
	}
	m.phase, m.status = callIdle, ""
	if reason != "" && reason != "Call ended" {
		m.a.toasts.Show(giode.ToastInfo, reason)
	}
}

func (m *callManager) setMuted(muted bool) {
	m.muted = muted
	if c := m.call.Load(); c != nil {
		c.SetMuted(muted)
	}
	telecom.SetMuted(m.id, muted)
}

func (m *callManager) toggleCamera() {
	if m.camOn {
		m.closeCamera()
	} else {
		m.openCamera()
	}
	m.signal(wire.CallMedia)
}

func (m *callManager) flipCamera() {
	if m.facing == camera.Front {
		m.facing = camera.Back
	} else {
		m.facing = camera.Front
	}
	if m.camOn {
		m.closeCamera()
		m.openCamera()
	}
}

func (m *callManager) openCamera() {
	m.camOn = true
	gen, facing := m.gen, m.facing
	go func() {
		cam, err := camera.Open(m.a.ctx(), m.a.g, camera.Options{
			Facing: facing, Color: true, AnalysisSize: 480, AnalysisRate: 15,
			OnFrame: func(f camera.Frame) {
				if c := m.call.Load(); c != nil && f.RGBA != nil {
					c.SendVideo(f.RGBA)
				}
			},
		})
		m.a.g.Post(func() {
			if err != nil {
				m.camOn = false
				m.a.toastError(err)
				return
			}
			if gen != m.gen || !m.camOn || m.cam != nil {
				cam.Close()
				return
			}
			m.cam = cam
		})
	}()
}

func (m *callManager) closeCamera() {
	m.camOn = false
	if m.cam != nil {
		m.cam.Close()
		m.cam = nil
	}
}

func (m *callManager) view() giode.Element { return m.screen.view() }
