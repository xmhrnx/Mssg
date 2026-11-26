package ui

import (
	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/internal/store"
)

// openMessage handles a tap on a message: download what isn't here,
// then show, play or open it.
func (a *App) openMessage(id string) {
	i, ok := a.chat.index[id]
	if !ok {
		return
	}
	m := a.chat.msgs[i]
	if m.Attachment == nil {
		return
	}
	if needsDownload(m) {
		if !a.svc.Downloading(id) {
			go a.svc.Download(id)
		}
		return
	}
	switch m.Kind {
	case store.KindImage, store.KindGIF:
		a.viewer.showImage(m)
	case store.KindVideo:
		a.viewer.showVideo(m)
	case store.KindAudio:
		a.voice.toggle(m)
	default:
		path := m.Attachment.Path
		go func() {
			err := a.g.OpenWith(a.ctx(), path)
			if err == nil || isCanceled(err) {
				return
			}
			// No app opens this type: offer it to other apps instead.
			if err := a.g.Share(a.ctx(), giode.ShareFile{Path: path, MIME: m.Attachment.MIME}); err != nil && !isCanceled(err) {
				a.g.Post(func() { a.toastError(err) })
			}
		}()
	}
}
