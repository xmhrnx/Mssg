package ui

import (
	"bytes"
	"crypto/sha256"
	"image"
	"os"
	"sync"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/mediakit"
)

// imageCache decodes pictures off the UI goroutine and keeps the most
// recent ones, so scrolling a chat doesn't decode on every frame.
type imageCache struct {
	app *giode.App

	mu      sync.Mutex
	entries map[string]*imageEntry
	order   []string
	// gen counts finished decodes, for row versions: a row showing a
	// picture rebuilds once it's decoded.
	gen int
}

type imageEntry struct {
	img    image.Image
	frames *mediakit.Frames
	err    error
	done   bool
}

const imageCacheSize = 120

func newImageCache(app *giode.App) *imageCache {
	return &imageCache{app: app, entries: map[string]*imageEntry{}}
}

// bytesKey names in-memory images (thumbnails, avatars) by content.
func bytesKey(b []byte) string {
	sum := sha256.Sum256(b)
	return string(sum[:12])
}

// FromBytes returns a decoded image, or nil while it's decoding.
func (c *imageCache) FromBytes(b []byte) image.Image {
	if len(b) == 0 {
		return nil
	}
	e := c.lookup("b:"+bytesKey(b), func() *imageEntry {
		img, _, err := image.Decode(bytes.NewReader(b))
		return &imageEntry{img: img, err: err}
	})
	return e.img
}

// FromFile returns a file's image scaled to fit side, or nil while it's
// decoding or when it can't be read.
func (c *imageCache) FromFile(path string, side int) image.Image {
	if path == "" {
		return nil
	}
	e := c.lookup("f:"+path, func() *imageEntry {
		f, err := os.Open(path)
		if err != nil {
			return &imageEntry{err: err}
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		if err != nil {
			return &imageEntry{err: err}
		}
		return &imageEntry{img: mediakit.Fit(img, side)}
	})
	return e.img
}

// GIF returns a GIF's decoded frames, or nil while decoding.
func (c *imageCache) GIF(path string, side int) *mediakit.Frames {
	if path == "" {
		return nil
	}
	e := c.lookup("g:"+path, func() *imageEntry {
		f, err := os.Open(path)
		if err != nil {
			return &imageEntry{err: err}
		}
		defer f.Close()
		fr, err := mediakit.DecodeGIF(f, side)
		return &imageEntry{frames: fr, err: err}
	})
	return e.frames
}

// Gen changes whenever an image finished decoding.
func (c *imageCache) Gen() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

func (c *imageCache) lookup(key string, decode func() *imageEntry) *imageEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok {
		return e
	}
	e := &imageEntry{}
	c.entries[key] = e
	c.order = append(c.order, key)
	if len(c.order) > imageCacheSize {
		delete(c.entries, c.order[0])
		c.order = c.order[1:]
	}
	go func() {
		res := decode()
		c.app.Post(func() {
			c.mu.Lock()
			*e = *res
			e.done = true
			c.gen++
			c.mu.Unlock()
			giode.InvalidateCaches()
		})
	}()
	return e
}
