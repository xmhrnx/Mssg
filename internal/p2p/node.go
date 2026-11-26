// Package p2p runs the libp2p node: QUIC and TCP transports secured with
// Noise or TLS, the public IPFS DHT to find contacts by their key, circuit
// relays and hole punching through NATs, and mDNS on the local network.
// No server of this app sits in between; relays only see ciphertext.
package p2p

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	ma "github.com/multiformats/go-multiaddr"
	mh "github.com/multiformats/go-multihash"
)

// Config configures a node.
type Config struct {
	PrivateKey crypto.PrivKey
	// DHTServer makes the node answer DHT queries when it's publicly
	// reachable; phones leave it off to save battery and data.
	DHTServer bool
	// IsContact reports whether a peer may reach this node's chat
	// protocols. Other peers can still use the DHT and relays.
	IsContact func(peer.ID) bool
	// OnConnectedness is called when a contact becomes reachable or not.
	OnConnectedness func(p peer.ID, connected bool)
	Logf            func(format string, args ...any)
	// Bootstrap overrides the DHT bootstrap peers (tests).
	Bootstrap []peer.AddrInfo
	// ListenAddrs overrides the listen addresses (tests).
	ListenAddrs []string
	// NoMDNS turns local discovery off (tests).
	NoMDNS bool
}

// Node is a running libp2p node.
type Node struct {
	cfg  Config
	Host host.Host
	DHT  *dht.IpfsDHT

	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	reaching map[peer.ID]*reachCall
	mdns     mdns.Service
}

type reachCall struct {
	done chan struct{}
	err  error
}

// Start creates and starts a node.
func Start(ctx context.Context, cfg Config) (*Node, error) {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.IsContact == nil {
		cfg.IsContact = func(peer.ID) bool { return false }
	}
	listen := cfg.ListenAddrs
	if listen == nil {
		listen = []string{
			"/ip4/0.0.0.0/udp/0/quic-v1",
			"/ip6/::/udp/0/quic-v1",
			"/ip4/0.0.0.0/tcp/0",
			"/ip6/::/tcp/0",
		}
	}
	boot := cfg.Bootstrap
	if boot == nil {
		boot = dht.GetDefaultBootstrapPeerAddrInfos()
	}
	n := &Node{cfg: cfg, reaching: map[peer.ID]*reachCall{}}
	n.ctx, n.cancel = context.WithCancel(context.Background())

	mode := dht.ModeClient
	if cfg.DHTServer {
		mode = dht.ModeAuto
	}
	var kad *dht.IpfsDHT
	opts := []libp2p.Option{
		libp2p.Identity(cfg.PrivateKey),
		libp2p.ListenAddrStrings(listen...),
		libp2p.DefaultTransports,
		libp2p.DefaultSecurity,
		libp2p.DefaultMuxers,
		libp2p.NATPortMap(),
		libp2p.EnableHolePunching(),
		libp2p.EnableRelay(),
		libp2p.EnableAutoRelayWithPeerSource(func(ctx context.Context, num int) <-chan peer.AddrInfo {
			return n.relayCandidates(ctx, num)
		}),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			var err error
			kad, err = dht.New(h, dht.Mode(mode), dht.BootstrapPeers(boot...))
			return kad, err
		}),
	}
	h, err := libp2p.New(opts...)
	if err != nil {
		n.cancel()
		return nil, fmt.Errorf("start libp2p: %w", err)
	}
	n.Host, n.DHT = h, kad

	h.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(_ network.Network, c network.Conn) {
			if p := c.RemotePeer(); cfg.IsContact(p) && cfg.OnConnectedness != nil {
				go cfg.OnConnectedness(p, true)
			}
		},
		DisconnectedF: func(net network.Network, c network.Conn) {
			p := c.RemotePeer()
			if cfg.IsContact(p) && cfg.OnConnectedness != nil && net.Connectedness(p) != network.Connected && net.Connectedness(p) != network.Limited {
				go cfg.OnConnectedness(p, false)
			}
		},
	})

	if err := kad.Bootstrap(n.ctx); err != nil {
		cfg.Logf("dht bootstrap: %v", err)
	}
	go n.connectBootstrap(boot)
	go n.announceLoop()
	if !cfg.NoMDNS {
		n.mdns = mdns.NewMdnsService(h, "mssg-lan", mdnsNotifee{n})
		if err := n.mdns.Start(); err != nil {
			cfg.Logf("mdns: %v", err)
		}
	}
	return n, nil
}

// Close stops the node.
func (n *Node) Close() error {
	n.cancel()
	if n.mdns != nil {
		n.mdns.Close()
	}
	n.DHT.Close()
	return n.Host.Close()
}

// ID is this node's peer ID: the user's permanent address.
func (n *Node) ID() peer.ID { return n.Host.ID() }

// Addrs returns this node's addresses with its peer ID, for invites.
func (n *Node) Addrs() []string {
	info := peer.AddrInfo{ID: n.Host.ID(), Addrs: n.Host.Addrs()}
	full, err := peer.AddrInfoToP2pAddrs(&info)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(full))
	for _, a := range full {
		out = append(out, a.String())
	}
	return out
}

// Connected reports whether a peer is connected, directly or relayed.
func (n *Node) Connected(p peer.ID) bool {
	c := n.Host.Network().Connectedness(p)
	return c == network.Connected || c == network.Limited
}

// Direct reports whether a peer has a connection not through a relay.
func (n *Node) Direct(p peer.ID) bool {
	for _, c := range n.Host.Network().ConnsToPeer(p) {
		if !c.Stat().Limited {
			return true
		}
	}
	return false
}

// AddAddrs remembers addresses for a peer.
func (n *Node) AddAddrs(p peer.ID, addrs []string) {
	for _, s := range addrs {
		a, err := ma.NewMultiaddr(s)
		if err != nil {
			continue
		}
		info, err := peer.AddrInfoFromP2pAddr(a)
		if err == nil && info.ID == p {
			n.Host.Peerstore().AddAddrs(p, info.Addrs, peerstore.RecentlyConnectedAddrTTL)
			continue
		}
		n.Host.Peerstore().AddAddr(p, a, peerstore.RecentlyConnectedAddrTTL)
	}
}

// KnownAddrs returns the addresses the peerstore holds for a peer.
func (n *Node) KnownAddrs(p peer.ID) []string {
	var out []string
	for _, a := range n.Host.Peerstore().Addrs(p) {
		out = append(out, a.String())
	}
	return out
}

// ErrUnreachable is returned when a peer can't be found or dialed.
var ErrUnreachable = errors.New("peer is offline or unreachable")

// Reach makes sure there is a connection to p, trying known addresses,
// then the DHT. Concurrent calls for one peer share the attempt.
func (n *Node) Reach(ctx context.Context, p peer.ID) error {
	if n.Connected(p) {
		return nil
	}
	n.mu.Lock()
	if rc, ok := n.reaching[p]; ok {
		n.mu.Unlock()
		select {
		case <-rc.done:
			return rc.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	rc := &reachCall{done: make(chan struct{})}
	n.reaching[p] = rc
	n.mu.Unlock()

	rc.err = n.reach(ctx, p)
	close(rc.done)
	n.mu.Lock()
	delete(n.reaching, p)
	n.mu.Unlock()
	return rc.err
}

func (n *Node) reach(ctx context.Context, p peer.ID) error {
	if len(n.Host.Peerstore().Addrs(p)) > 0 {
		dctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		err := n.Host.Connect(dctx, peer.AddrInfo{ID: p})
		cancel()
		if err == nil {
			return nil
		}
	}
	lctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	found := make(chan peer.AddrInfo, 8)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if info, err := n.DHT.FindPeer(lctx, p); err == nil {
			found <- info
		}
	}()
	go func() {
		defer wg.Done()
		for info := range n.DHT.FindProvidersAsync(lctx, PresenceKey(p), 4) {
			if info.ID == p {
				found <- info
			}
		}
	}()
	go func() { wg.Wait(); close(found) }()
	for info := range found {
		if len(info.Addrs) == 0 {
			continue
		}
		n.Host.Peerstore().AddAddrs(p, info.Addrs, peerstore.TempAddrTTL)
		dctx, dcancel := context.WithTimeout(lctx, 15*time.Second)
		err := n.Host.Connect(dctx, peer.AddrInfo{ID: p})
		dcancel()
		if err == nil {
			return nil
		}
		n.cfg.Logf("dial %s: %v", p.ShortString(), err)
	}
	if n.Connected(p) {
		return nil
	}
	return ErrUnreachable
}

// NewStream reaches p and opens a stream, over a relay if that's all there is.
func (n *Node) NewStream(ctx context.Context, p peer.ID, proto string) (network.Stream, error) {
	if err := n.Reach(ctx, p); err != nil {
		return nil, err
	}
	ctx = network.WithAllowLimitedConn(ctx, "mssg")
	return n.Host.NewStream(ctx, p, protocol.ID(proto))
}

// WaitDirect waits up to d for a hole-punched direct connection to p.
func (n *Node) WaitDirect(ctx context.Context, p peer.ID, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for !n.Direct(p) {
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(300 * time.Millisecond):
		}
	}
	return true
}

// SetHandler registers a stream handler that only contacts reach;
// streams from anyone else are reset.
func (n *Node) SetHandler(proto string, fn func(network.Stream)) {
	n.Host.SetStreamHandler(protocol.ID(proto), func(s network.Stream) {
		if !n.cfg.IsContact(s.Conn().RemotePeer()) {
			s.Reset()
			return
		}
		fn(s)
	})
}

// SetOpenHandler registers a handler any peer can reach (invite redemption).
func (n *Node) SetOpenHandler(proto string, fn func(network.Stream)) {
	n.Host.SetStreamHandler(protocol.ID(proto), fn)
}

// PresenceKey is the DHT key a peer announces itself under, so contacts
// find its current addresses even when it's a DHT client.
func PresenceKey(p peer.ID) cid.Cid {
	sum := sha256.Sum256([]byte("mssg-presence/v1/" + p.String()))
	h, _ := mh.Encode(sum[:], mh.SHA2_256)
	return cid.NewCidV1(cid.Raw, h)
}

func (n *Node) announceLoop() {
	sub, err := n.Host.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))
	if err != nil {
		n.cfg.Logf("subscribe addresses: %v", err)
		return
	}
	defer sub.Close()
	// Let the routing table fill before the first announcement.
	select {
	case <-n.ctx.Done():
		return
	case <-time.After(5 * time.Second):
	}
	tick := time.NewTicker(4 * time.Hour)
	defer tick.Stop()
	var last time.Time
	announce := func() {
		if time.Since(last) < 2*time.Minute {
			return
		}
		last = time.Now()
		ctx, cancel := context.WithTimeout(n.ctx, 2*time.Minute)
		defer cancel()
		if err := n.DHT.Provide(ctx, PresenceKey(n.ID()), true); err != nil {
			n.cfg.Logf("announce: %v", err)
		}
	}
	announce()
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-tick.C:
			announce()
		case <-sub.Out():
			announce()
		}
	}
}

func (n *Node) connectBootstrap(boot []peer.AddrInfo) {
	var wg sync.WaitGroup
	for _, info := range boot {
		wg.Add(1)
		go func(info peer.AddrInfo) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(n.ctx, 15*time.Second)
			defer cancel()
			if err := n.Host.Connect(ctx, info); err != nil {
				n.cfg.Logf("bootstrap %s: %v", info.ID.ShortString(), err)
			}
		}(info)
	}
	wg.Wait()
}

// relayCandidates offers DHT peers as relays to AutoRelay.
func (n *Node) relayCandidates(ctx context.Context, num int) <-chan peer.AddrInfo {
	out := make(chan peer.AddrInfo, num)
	go func() {
		defer close(out)
		if n.DHT == nil {
			return
		}
		sent := 0
		for _, p := range n.DHT.RoutingTable().ListPeers() {
			if sent >= num {
				return
			}
			addrs := n.Host.Peerstore().Addrs(p)
			if len(addrs) == 0 {
				continue
			}
			select {
			case out <- peer.AddrInfo{ID: p, Addrs: addrs}:
				sent++
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

type mdnsNotifee struct{ n *Node }

func (m mdnsNotifee) HandlePeerFound(info peer.AddrInfo) {
	if info.ID == m.n.ID() || !m.n.cfg.IsContact(info.ID) {
		return
	}
	m.n.Host.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.TempAddrTTL)
	if m.n.Connected(info.ID) {
		return
	}
	ctx, cancel := context.WithTimeout(m.n.ctx, 10*time.Second)
	defer cancel()
	if err := m.n.Host.Connect(ctx, info); err != nil {
		m.n.cfg.Logf("lan dial %s: %v", info.ID.ShortString(), err)
	}
}
