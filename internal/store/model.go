// Package store keeps the app's data - profile, contacts, messages and
// the outbox - in a bbolt database whose values are sealed with
// XChaCha20-Poly1305 under a key held by the platform's secure storage.
package store

import "time"

// Kind is what a message carries.
type Kind string

// Message kinds.
const (
	KindText  Kind = "text"
	KindImage Kind = "image"
	KindGIF   Kind = "gif"
	KindVideo Kind = "video"
	KindAudio Kind = "audio" // voice notes and audio files
	KindFile  Kind = "file"
	KindCall  Kind = "call" // a call log entry
)

// Status is how far an outgoing message got.
type Status int

// Message statuses, in order.
const (
	StatusPending Status = iota
	StatusSent
	StatusDelivered
	StatusRead
	StatusFailed
)

// TransferState is where an attachment's bytes are.
type TransferState int

// Attachment transfer states.
const (
	TransferNone TransferState = iota // offered, not downloaded
	TransferActive
	TransferDone
	TransferFailed
)

// Profile is this device's identity as shown to contacts.
type Profile struct {
	Name   string `json:"name"`
	Avatar []byte `json:"avatar,omitempty"` // small JPEG
	// Rev increases on every change so contacts can tell a newer one.
	Rev int64 `json:"rev"`
}

// Contact is someone this device exchanged invites with.
type Contact struct {
	PeerID   string    `json:"peer"`
	Name     string    `json:"name"`
	Avatar   []byte    `json:"avatar,omitempty"`
	Rev      int64     `json:"rev"`
	Addrs    []string  `json:"addrs,omitempty"` // last known multiaddrs
	Added    time.Time `json:"added"`
	LastSeen time.Time `json:"lastSeen"`
	// Confirmed is set once both sides completed the handshake.
	Confirmed bool `json:"confirmed"`
	// InviteToken proves to the inviter that this device redeemed its
	// code; kept until confirmed.
	InviteToken string `json:"token,omitempty"`
	Unread      int    `json:"unread"`
	Muted       bool   `json:"muted"`
}

// Attachment describes a file sent with a message.
type Attachment struct {
	Name       string        `json:"name"`
	MIME       string        `json:"mime"`
	Size       int64         `json:"size"`
	SHA256     string        `json:"sha256"`
	Width      int           `json:"w,omitempty"`
	Height     int           `json:"h,omitempty"`
	DurationMs int64         `json:"dur,omitempty"`
	Thumb      []byte        `json:"thumb,omitempty"` // small JPEG preview
	Waveform   []byte        `json:"wave,omitempty"`  // voice note loudness, 0-255
	Path       string        `json:"path,omitempty"`  // local file, once present
	Received   int64         `json:"recv,omitempty"`
	State      TransferState `json:"state"`
}

// CallInfo is the content of a call log message.
type CallInfo struct {
	Video    bool   `json:"video"`
	Missed   bool   `json:"missed"`
	Declined bool   `json:"declined"`
	Seconds  int    `json:"secs"`
	Reason   string `json:"reason,omitempty"`
}

// Message is one entry of a conversation.
type Message struct {
	ID         string      `json:"id"`
	Peer       string      `json:"peer"`
	Seq        uint64      `json:"seq"`
	Outgoing   bool        `json:"out"`
	Time       time.Time   `json:"time"`
	Kind       Kind        `json:"kind"`
	Text       string      `json:"text,omitempty"`
	Attachment *Attachment `json:"att,omitempty"`
	Call       *CallInfo   `json:"call,omitempty"`
	Status     Status      `json:"status"`
	// Rev increases on every local change, for the UI's row cache.
	Rev int `json:"-"`
}

// Settings are the user's preferences.
type Settings struct {
	Theme            string   `json:"theme"` // "system", "light", "dark"
	StayOnline       bool     `json:"stayOnline"`
	AutoDownloadMB   int      `json:"autoDownloadMB"`
	ReadReceipts     bool     `json:"readReceipts"`
	TURN             []string `json:"turn,omitempty"` // "turn:host:port|user|pass"
	STUN             []string `json:"stun,omitempty"`
	RelayOnlyCalls   bool     `json:"relayOnlyCalls"`
	NotifyOnMessages bool     `json:"notify"`
}

// DefaultSettings are used until the user changes something.
func DefaultSettings() Settings {
	return Settings{
		Theme:            "system",
		AutoDownloadMB:   25,
		ReadReceipts:     true,
		NotifyOnMessages: true,
		STUN:             []string{"stun:stun.l.google.com:19302", "stun:stun.cloudflare.com:3478"},
	}
}
