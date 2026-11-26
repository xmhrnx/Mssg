package messenger

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/xmhrnx/mssg/internal/mediakit"
	"github.com/xmhrnx/mssg/internal/store"
	"github.com/xmhrnx/mssg/internal/wire"
)

// MaxFileSize bounds attachments.
const MaxFileSize = 2 << 30

// FileMeta is what the caller knows about a file it sends.
type FileMeta struct {
	Name       string
	MIME       string
	DurationMs int64
	Waveform   []byte
	// Kind overrides the kind guessed from the MIME type.
	Kind store.Kind
}

// SendFile copies a file into the app's storage and offers it to a
// contact. Images and GIFs get a thumbnail; the contact downloads the
// bytes from this device when it's online.
func (s *Service) SendFile(peerID string, r io.Reader, meta FileMeta) (store.Message, error) {
	id := newID()
	dir := filepath.Join(s.attDir, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return store.Message{}, err
	}
	name := safeName(meta.Name)
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return store.Message{}, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, MaxFileSize+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > MaxFileSize {
		err = fmt.Errorf("files are limited to %d GB", MaxFileSize>>30)
	}
	if err != nil {
		os.RemoveAll(dir)
		return store.Message{}, err
	}
	mt := mediakit.MIMEOf(name, meta.MIME)
	kind := meta.Kind
	if kind == "" {
		kind = mediakit.KindOf(mt)
	}
	a := &store.Attachment{
		Name: name, MIME: mt, Size: n, SHA256: hex.EncodeToString(h.Sum(nil)),
		DurationMs: meta.DurationMs, Waveform: meta.Waveform,
		Path: path, Received: n, State: store.TransferDone,
	}
	if kind == store.KindImage || kind == store.KindGIF {
		if pf, err := os.Open(path); err == nil {
			a.Width, a.Height, a.Thumb, _ = mediakit.Preview(pf)
			pf.Close()
		}
	}
	m := store.Message{Peer: peerID, Kind: kind, Attachment: a}
	m.ID = id
	m.Outgoing = true
	m.Time = time.Now()
	if err := s.st.AddMessage(&m); err != nil {
		return m, err
	}
	s.emitMessage(m)
	s.emitContacts()
	s.queue("msg:"+m.ID, m.Peer, wire.Frame{Type: wire.TMessage, Message: toWire(m)})
	return m, nil
}

func safeName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		name = "file"
	}
	if len(name) > 120 {
		ext := filepath.Ext(name)
		if len(ext) > 16 {
			ext = ""
		}
		name = name[:120-len(ext)] + ext
	}
	return name
}

// Downloading reports whether an attachment is being fetched now.
func (s *Service) Downloading(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.downloads[id]
}

// Download fetches an incoming attachment from its sender, resuming a
// partial file, and verifies its hash.
func (s *Service) Download(id string) {
	s.mu.Lock()
	if s.downloads[id] {
		s.mu.Unlock()
		return
	}
	s.downloads[id] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.downloads, id)
		s.mu.Unlock()
	}()
	m, err := s.st.Message(id)
	if err != nil || m.Outgoing || m.Attachment == nil || m.Attachment.State == store.TransferDone {
		return
	}
	s.setTransfer(id, store.TransferActive, "", -1)
	var lastErr error
	for attempt := range 4 {
		if attempt > 0 {
			select {
			case <-s.ctx.Done():
				return
			case <-time.After(time.Duration(attempt*attempt) * 3 * time.Second):
			}
		}
		lastErr = s.fetch(m)
		if lastErr == nil {
			return
		}
		s.ev.Logf("download %s: %v", id, lastErr)
		if errors.Is(lastErr, errHashMismatch) {
			break
		}
	}
	s.setTransfer(id, store.TransferFailed, "", -1)
}

var errHashMismatch = errors.New("file doesn't match what was sent")

func (s *Service) fetch(m store.Message) error {
	p, err := peer.Decode(m.Peer)
	if err != nil {
		return err
	}
	dir := filepath.Join(s.attDir, m.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, safeName(m.Attachment.Name))
	part := path + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	have, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	if err := s.node.Reach(ctx, p); err != nil {
		return err
	}
	// Relays carry only small amounts; give hole punching a moment.
	s.node.WaitDirect(ctx, p, 8*time.Second)
	st, err := s.node.NewStream(ctx, p, wire.FileProtocol)
	if err != nil {
		return err
	}
	defer st.Close()
	st.SetDeadline(time.Now().Add(30 * time.Second))
	if err := wire.Write(st, wire.FileRequest{ID: m.ID, Offset: have}); err != nil {
		return err
	}
	br := bufio.NewReader(st)
	var resp wire.FileResponse
	if err := wire.Read(br, &resp); err != nil {
		return err
	}
	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	if resp.Size != m.Attachment.Size {
		return fmt.Errorf("size changed")
	}
	buf := make([]byte, 64<<10)
	lastReport := time.Now()
	for have < resp.Size {
		st.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, rerr := br.Read(buf[:min(int64(len(buf)), resp.Size-have)])
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			have += int64(n)
			if time.Since(lastReport) > 250*time.Millisecond {
				lastReport = time.Now()
				s.setTransfer(m.ID, store.TransferActive, "", have)
			}
		}
		if rerr != nil {
			if rerr == io.EOF && have == resp.Size {
				break
			}
			return rerr
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != m.Attachment.SHA256 {
		f.Close()
		os.Remove(part)
		return errHashMismatch
	}
	f.Close()
	if err := os.Rename(part, path); err != nil {
		return err
	}
	s.setTransfer(m.ID, store.TransferDone, path, have)
	return nil
}

func (s *Service) setTransfer(id string, state store.TransferState, path string, received int64) {
	m, err := s.st.UpdateMessage(id, func(m *store.Message) {
		if m.Attachment == nil {
			return
		}
		m.Attachment.State = state
		if path != "" {
			m.Attachment.Path = path
		}
		if received >= 0 {
			m.Attachment.Received = received
		}
	})
	if err == nil {
		s.emitMessage(m)
	}
}

// handleFileStream serves an attachment this device sent to the peer
// asking for it.
func (s *Service) handleFileStream(st network.Stream) {
	defer st.Close()
	st.SetDeadline(time.Now().Add(30 * time.Second))
	from := st.Conn().RemotePeer().String()
	var req wire.FileRequest
	if err := wire.Read(bufio.NewReader(st), &req); err != nil {
		st.Reset()
		return
	}
	fail := func(reason string) { wire.Write(st, wire.FileResponse{Error: reason}) }
	m, err := s.st.Message(req.ID)
	if err != nil || m.Peer != from || m.Attachment == nil || m.Attachment.Path == "" {
		fail("no such file")
		return
	}
	f, err := os.Open(m.Attachment.Path)
	if err != nil {
		fail("the file is no longer on the sender's device")
		return
	}
	defer f.Close()
	if req.Offset < 0 || req.Offset > m.Attachment.Size {
		fail("bad offset")
		return
	}
	if _, err := f.Seek(req.Offset, io.SeekStart); err != nil {
		fail("read failed")
		return
	}
	if err := wire.Write(st, wire.FileResponse{Size: m.Attachment.Size}); err != nil {
		return
	}
	buf := make([]byte, 64<<10)
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			st.SetWriteDeadline(time.Now().Add(30 * time.Second))
			if _, err := st.Write(buf[:n]); err != nil {
				st.Reset()
				return
			}
		}
		if rerr != nil {
			return
		}
	}
}
