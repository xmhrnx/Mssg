package ui

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xmhrnx/giode/mcp"

	"github.com/xmhrnx/mssg/internal/mediakit"
	"github.com/xmhrnx/mssg/internal/messenger"
	"github.com/xmhrnx/mssg/internal/store"
)

// registerAutomation adds MCP tools for driving the app without the
// system's file pickers: they only exist while the MCP server runs
// (GIODE_MCP=1), for tests and agents.
func (a *App) registerAutomation() {
	type sendIn struct {
		Contact string `json:"contact" description:"The contact's name"`
		Path    string `json:"path" description:"A file on this computer"`
		Voice   bool   `json:"voice,omitempty" description:"Send a 16-bit mono WAV as a voice message"`
	}
	a.g.MCPTool(mcp.Func("send_file", "Sends a file to a contact, as the attach button would.",
		func(ctx context.Context, in sendIn) (any, error) {
			done := make(chan string, 1)
			a.g.Post(func() {
				for _, c := range a.contacts {
					if c.Name == in.Contact {
						done <- c.PeerID
						return
					}
				}
				done <- ""
			})
			peer := <-done
			if peer == "" {
				return nil, fmt.Errorf("no contact named %q", in.Contact)
			}
			f, err := os.Open(in.Path)
			if err != nil {
				return nil, err
			}
			defer f.Close()
			meta := messenger.FileMeta{Name: filepath.Base(in.Path)}
			if in.Voice {
				samples, rate, err := readWAV(in.Path)
				if err != nil {
					return nil, err
				}
				meta.Kind, meta.MIME = store.KindAudio, "audio/wav"
				meta.DurationMs = int64(len(samples)) * 1000 / int64(rate)
				meta.Waveform = mediakit.Waveform(samples, 40)
			}
			m, err := a.svc.SendFile(peer, f, meta)
			if err != nil {
				return nil, err
			}
			return "sent " + m.ID, nil
		}))
}

// readWAV reads a canonical 16-bit mono WAV.
func readWAV(path string) ([]int16, int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	if len(b) < 44 || string(b[:4]) != "RIFF" {
		return nil, 0, fmt.Errorf("not a WAV file")
	}
	rate := int(binary.LittleEndian.Uint32(b[24:28]))
	data := b[44:]
	samples := make([]int16, len(data)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return samples, rate, nil
}
