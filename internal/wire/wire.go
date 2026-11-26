// Package wire defines the frames peers exchange over their encrypted
// libp2p streams, and their length-prefixed JSON encoding.
package wire

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/xmhrnx/mssg/internal/store"
)

// Protocol IDs.
const (
	MsgProtocol  = "/mssg/msg/1.0.0"
	FileProtocol = "/mssg/file/1.0.0"
)

// MaxFrame bounds a frame so a peer can't make us allocate without limit.
const MaxFrame = 1 << 20

// Type names a frame.
type Type string

// Frame types.
const (
	TContactRequest Type = "contact_req" // redeem an invite
	TContactOK      Type = "contact_ok"
	TProfile        Type = "profile"
	TMessage        Type = "msg"
	TAck            Type = "ack"
	TRead           Type = "read"
	TTyping         Type = "typing"
	TCall           Type = "call"
	TError          Type = "error"
)

// Card is what an invite carries through the wormhole.
type Card struct {
	Version int      `json:"v"`
	PeerID  string   `json:"peer"`
	Name    string   `json:"name"`
	Addrs   []string `json:"addrs,omitempty"`
	Token   string   `json:"token,omitempty"`
}

// Profile is a peer's display name and picture.
type Profile struct {
	Name   string `json:"name"`
	Avatar []byte `json:"avatar,omitempty"`
	Rev    int64  `json:"rev"`
}

// Message is a chat message as sent; attachments carry no local path.
type Message struct {
	ID         string            `json:"id"`
	Time       int64             `json:"time"` // unix ms
	Kind       store.Kind        `json:"kind"`
	Text       string            `json:"text,omitempty"`
	Attachment *store.Attachment `json:"att,omitempty"`
	Call       *store.CallInfo   `json:"call,omitempty"`
}

// CallAction is a step of call signaling.
type CallAction string

// Call signaling actions.
const (
	CallOffer  CallAction = "offer"
	CallAnswer CallAction = "answer"
	CallEnd    CallAction = "end"
	CallReject CallAction = "reject"
	CallBusy   CallAction = "busy"
	CallRing   CallAction = "ringing"
	CallMedia  CallAction = "media" // camera turned on or off
)

// Call is a call signaling frame.
type Call struct {
	ID     string     `json:"id"`
	Action CallAction `json:"action"`
	Video  bool       `json:"video,omitempty"`
	SDP    string     `json:"sdp,omitempty"`
}

// Frame is one unit on a message stream.
type Frame struct {
	Type    Type     `json:"t"`
	Card    *Card    `json:"card,omitempty"`
	Profile *Profile `json:"profile,omitempty"`
	Message *Message `json:"msg,omitempty"`
	IDs     []string `json:"ids,omitempty"`
	Call    *Call    `json:"call,omitempty"`
	Error   string   `json:"err,omitempty"`
}

// FileRequest asks a peer for an attachment's bytes from Offset.
type FileRequest struct {
	ID     string `json:"id"`
	Offset int64  `json:"off"`
}

// FileResponse precedes the bytes of a file stream.
type FileResponse struct {
	Size  int64  `json:"size"`
	Error string `json:"err,omitempty"`
}

// Write writes v as one length-prefixed JSON frame.
func Write(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > MaxFrame {
		return fmt.Errorf("wire: frame of %d bytes is too large", len(b))
	}
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(b)))
	if _, err := w.Write(n[:]); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// ErrTooLarge is returned for frames over MaxFrame.
var ErrTooLarge = errors.New("wire: frame too large")

// Read reads one frame written by Write into v.
func Read(r *bufio.Reader, v any) error {
	var n [4]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(n[:])
	if size > MaxFrame {
		return ErrTooLarge
	}
	b := make([]byte, size)
	if _, err := io.ReadFull(r, b); err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
