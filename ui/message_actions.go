package ui

import (
	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/gallery"

	"github.com/xmhrnx/mssg/internal/store"
)

// messageActions is the sheet a long press on a message opens.
type messageActions struct {
	a     *App
	open  bool
	msg   store.Message
	sheet *giode.SheetElement

	copy, save, share, del, retry *giode.PressableElement
}

func newMessageActions(a *App) *messageActions {
	s := &messageActions{a: a}
	s.copy = actionRow("copy", "Copy text", "theme.text", func() {
		a.g.SetClipboard(s.msg.Text)
		a.toasts.Show(giode.ToastSuccess, "Copied")
	}, &s.open)
	s.save = actionRow("download", "Save to device", "theme.text", s.saveToDevice, &s.open)
	s.share = actionRow("share-2", "Share", "theme.text", s.shareFile, &s.open)
	s.retry = actionRow("refresh-cw", "Download again", "theme.text", func() {
		id := s.msg.ID
		go a.svc.Download(id)
	}, &s.open)
	s.del = actionRow("trash-2", "Delete for me", "theme.danger", func() {
		id := s.msg.ID
		go func() {
			if err := a.svc.DeleteMessage(id); err != nil {
				a.g.Post(func() { a.toastError(err) })
			}
		}()
	}, &s.open)
	s.sheet = giode.Sheet(&s.open, s.body).Adaptive(true)
	return s
}

// actionRow is one tappable line of an action sheet; it closes the sheet.
func actionRow(ic, label, color string, run func(), open *bool) *giode.PressableElement {
	return giode.Pressable(func(st giode.PressState) giode.Element {
		bg := "theme.surface"
		if st.Pressed || st.Hovered {
			bg = pressTint
		}
		return giode.Row(giode.Styles{Width: -1, Gap: 14, Align: giode.AlignCenter, Padding: giode.SymmetricInset(12, 14), BorderRadius: 12, Background: bg},
			icon(ic, 20, color), text(label, fsBody, color, giode.FontWeightMedium))
	}).Label(label).OnClick(func(giode.ClickEvent) {
		*open = false
		run()
	})
}

func (s *messageActions) show(id string) {
	i, ok := s.a.chat.index[id]
	if !ok {
		return
	}
	s.msg = s.a.chat.msgs[i]
	s.open = true
	giode.Haptic(giode.HapticLight)
}

func (s *messageActions) body() giode.Element {
	m := s.msg
	var rows []giode.Element
	if m.Kind == store.KindText && m.Text != "" {
		rows = append(rows, s.copy)
	}
	if a := m.Attachment; a != nil {
		switch {
		case a.State == store.TransferDone:
			rows = append(rows, s.save, s.share)
		case !m.Outgoing:
			rows = append(rows, s.retry)
		}
	}
	rows = append(rows, s.del)
	return giode.Stack(giode.Styles{Width: -1, Gap: 2, Padding: giode.Inset{Bottom: 16}}, rows...)
}

func (s *messageActions) saveToDevice() {
	m := s.msg
	a := s.a
	go func() {
		var err error
		switch m.Kind {
		case store.KindImage, store.KindGIF, store.KindVideo:
			err = gallery.Save(a.ctx(), a.g, m.Attachment.Path, "Mssg")
		default:
			_, err = a.g.SaveToDownloads(a.ctx(), m.Attachment.Path, "Mssg")
		}
		a.g.Post(func() {
			if err != nil {
				a.toastError(err)
				return
			}
			a.toasts.Show(giode.ToastSuccess, "Saved")
		})
	}()
}

func (s *messageActions) shareFile() {
	m := s.msg
	go func() {
		err := s.a.g.Share(s.a.ctx(), giode.ShareFile{Path: m.Attachment.Path, MIME: m.Attachment.MIME})
		if err != nil && !isCanceled(err) {
			s.a.g.Post(func() { s.a.toastError(err) })
		}
	}()
}

func (s *messageActions) view() giode.Element { return s.sheet }
