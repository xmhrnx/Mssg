package ui

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/mediakit"
	"github.com/xmhrnx/mssg/internal/messenger"
)

func (a *App) ctx() context.Context { return context.Background() }

// sendPicked sends files the user picked, closing each. Call it off the
// UI goroutine.
func (a *App) sendPicked(peer string, files []giode.PickedFile) {
	for _, f := range files {
		_, err := a.svc.SendFile(peer, f, messenger.FileMeta{Name: f.Name, MIME: f.MIME})
		f.Close()
		if err != nil {
			a.g.Post(func() { a.toastError(err) })
		}
	}
}

// sendImage sends an in-memory picture (a pasted screenshot) as JPEG.
func (a *App) sendImage(peer string, img image.Image) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, mediakit.Fit(img, 2560), &jpeg.Options{Quality: 88}); err != nil {
		a.g.Post(func() { a.toastError(err) })
		return
	}
	if _, err := a.svc.SendFile(peer, &buf, messenger.FileMeta{Name: "Image.jpg", MIME: "image/jpeg"}); err != nil {
		a.g.Post(func() { a.toastError(err) })
	}
}
