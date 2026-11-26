package ui

import (
	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/media"
	"github.com/xmhrnx/giode/video"

	"github.com/xmhrnx/mssg/internal/store"
)

// mediaViewer shows a photo (pinch to zoom), a GIF or a video over the
// whole window.
type mediaViewer struct {
	a      *App
	msg    *store.Message
	player *media.Player
	zoom   *giode.ZoomElement
	close  *giode.IconButtonElement
	share  *giode.IconButtonElement
}

func newMediaViewer(a *App) *mediaViewer {
	v := &mediaViewer{a: a}
	v.zoom = giode.Zoomable(v.picture)
	v.close = giode.IconButton(icon("x", 24, "#FFFFFF"), giode.Styles{Background: "#FFFFFF", BorderRadius: 999, Padding: giode.UniformInset(8)}).
		Variant(giode.IconButtonGhost).Label("Close").OnClick(v.hide)
	v.share = giode.IconButton(icon("share-2", 22, "#FFFFFF"), giode.Styles{Background: "#FFFFFF", BorderRadius: 999, Padding: giode.UniformInset(8)}).
		Variant(giode.IconButtonGhost).Label("Share").OnClick(func() {
		m := *v.msg
		go func() {
			if err := a.g.Share(a.ctx(), giode.ShareFile{Path: m.Attachment.Path, MIME: m.Attachment.MIME}); err != nil && !isCanceled(err) {
				a.g.Post(func() { a.toastError(err) })
			}
		}()
	})
	return v
}

func (v *mediaViewer) showImage(m store.Message) {
	v.hide()
	v.msg = &m
}

func (v *mediaViewer) showVideo(m store.Message) {
	v.hide()
	p, err := media.Open(v.a.ctx(), v.a.g, m.Attachment.Path, media.Options{
		Autoplay: true,
		OnError:  func(err error) { v.a.g.Post(func() { v.a.toastError(err) }) },
	})
	if err != nil {
		v.a.toastError(err)
		return
	}
	v.a.voice.stop()
	v.msg, v.player = &m, p
}

func (v *mediaViewer) hide() {
	if v.player != nil {
		v.player.Close()
		v.player = nil
	}
	v.msg = nil
}

func (v *mediaViewer) picture() giode.Element {
	if v.msg == nil {
		return giode.Box(giode.Styles{})
	}
	att := v.msg.Attachment
	if v.msg.Kind == store.KindGIF {
		if fr := v.a.images.GIF(att.Path, 480); fr != nil {
			w, h := fitInto(att.Width, att.Height, 900, 900)
			return gifView(fr, w, h)
		}
	}
	if img := v.a.images.FromFile(att.Path, 2560); img != nil {
		return giode.Image(img, giode.Styles{Width: -1, Height: -1, Fit: giode.FitContain})
	}
	return giode.Spinner(giode.Styles{Width: 32, Color: "#FFFFFF"})
}

// fitInto scales w×h to fit a box, keeping its shape.
func fitInto(w, h, bw, bh int) (int, int) {
	if w <= 0 || h <= 0 {
		return bw, bh
	}
	if w*bh > h*bw {
		return bw, h * bw / w
	}
	return w * bh / h, bh
}

func (v *mediaViewer) view() giode.Element {
	if v.msg == nil {
		return giode.Box(giode.Styles{Display: giode.DisplayNone})
	}
	giode.HandleBack(v.hide)
	var content giode.Element
	if v.player != nil {
		content = v.player.View(media.ViewOptions{Fit: video.Contain, Label: v.msg.Attachment.Name})
	} else {
		content = v.zoom
	}
	c, _ := v.a.contactByID(v.msg.Peer)
	who := c.Name
	if v.msg.Outgoing {
		who = "You"
	}
	top := giode.Row(giode.Styles{Width: -1, Align: giode.AlignCenter, Gap: 8, Padding: giode.SymmetricInset(8, 8), Wrap: giode.WrapNoWrap,
		BackgroundGradient: giode.Linear(180, "#000000CC", "#00000000")},
		v.close,
		giode.Stack(giode.Styles{FlexGrow: 1, Gap: 1},
			line(who, fsBody, "#FFFFFF", giode.FontWeightSemiBold),
			line(dayLabel(v.msg.Time)+" · "+v.msg.Time.Format("15:04"), fsSmall-1, "#FFFFFFAA")),
		v.share,
	)
	return giode.Box(giode.Styles{Position: giode.PositionAbsolute, Top: giode.Flush, Left: giode.Flush, Right: giode.Flush, Bottom: giode.Flush,
		ZIndex: 50, Background: "#000000"},
		giode.Box(giode.Styles{Width: -1, Height: -1}, content),
		giode.Box(giode.Styles{Position: giode.PositionAbsolute, Top: giode.Flush, Left: giode.Flush, Right: giode.Flush, ZIndex: 2}, top),
	)
}
