package ui

import (
	"strings"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/components/emojipicker"
)

// composer is the message field with emoji, attachments, and send or
// record-voice buttons.
type composer struct {
	a      *App
	chat   *chatScreen
	peer   string
	drafts map[string]string

	field    *giode.InputElement
	emoji    *giode.IconButtonElement
	attach   *giode.IconButtonElement
	send     *giode.IconButtonElement
	mic      *giode.IconButtonElement
	picker   *emojipicker.Picker
	emojiOn  bool
	emojiBox *giode.SheetElement

	attachSheet *attachSheet
	recorder    *voiceRecorder
}

func newComposer(a *App, chat *chatScreen) *composer {
	c := &composer{a: a, chat: chat, drafts: map[string]string{}}
	c.field = giode.Input.Text("Message", giode.Styles{
		Background: "theme.raised", BorderColor: "theme.raised", BorderRadius: 22,
		Padding: giode.SymmetricInset(16, 11), FontSize: fsBody + 1, Color: "theme.text",
	}).Lines(1, 6).SubmitOn(giode.SubmitEnterUnlessShift).
		Submit(func(string) { c.sendText() }).
		OnChange(func(string) {
			if c.peer != "" {
				a.svc.SendTyping(c.peer)
			}
		}).
		OnPaste(c.paste)
	c.field.SpellCheck(true)
	c.emoji = iconButton("smile", 22, "Emoji", func() { c.emojiOn = true })
	c.attach = iconButton("paperclip", 22, "Attach", func() { c.attachSheet.show(c.peer) })
	c.send = giode.IconButton(icon("send-horizontal", 20, "#FFFFFF"), giode.Styles{
		Width: 44, Height: 44, BorderRadius: 999, BackgroundGradient: gradientMine,
	}).Label("Send").OnClick(c.sendText)
	c.recorder = newVoiceRecorder(a)
	c.mic = giode.IconButton(icon("mic", 21, "#FFFFFF"), giode.Styles{
		Width: 44, Height: 44, BorderRadius: 999, BackgroundGradient: gradientMine,
	}).Label("Record a voice message").OnClick(func() { c.recorder.start(c.peer) })
	c.picker = giode.EmojiPicker().OnPick(func(e string) { c.field.InsertText(e) })
	c.emojiBox = giode.Sheet(&c.emojiOn, func() giode.Element { return c.picker }).Detents(giode.DetentMedium, giode.DetentLarge)
	c.attachSheet = newAttachSheet(a)
	return c
}

// reset switches to another conversation, keeping each one's draft.
func (c *composer) reset(peer string) {
	c.peer = peer
	c.field.SetText(c.drafts[peer])
	c.recorder.cancel()
}

func (c *composer) saveDraft() {
	if c.peer != "" {
		c.drafts[c.peer] = c.field.Text()
	}
}

func (c *composer) sendText() {
	body := strings.TrimSpace(c.field.Text())
	if body == "" || c.peer == "" {
		return
	}
	c.field.SetText("")
	delete(c.drafts, c.peer)
	peer := c.peer
	go func() {
		if _, err := c.a.svc.SendText(peer, body); err != nil {
			c.a.g.Post(func() { c.a.toastError(err) })
		}
	}()
}

// paste attaches a copied image, or pastes text.
func (c *composer) paste(pasteText func()) {
	peer := c.peer
	go func() {
		img, err := c.a.g.ReadClipboardImage(c.a.ctx())
		if err != nil || img == nil {
			c.a.g.Post(pasteText)
			return
		}
		c.a.sendImage(peer, img)
	}()
}

func (c *composer) view() giode.Element {
	if c.recorder.active() {
		return giode.Stack(giode.Styles{Width: -1}, c.recorder.view(), c.attachSheet.view())
	}
	action := c.mic
	if strings.TrimSpace(c.field.Text()) != "" {
		action = c.send
	}
	bar := giode.Row(giode.Styles{
		Width: -1, Gap: 4, Align: giode.AlignEnd, Wrap: giode.WrapNoWrap,
		Padding: giode.Inset{Top: 8, Bottom: 10, Left: 6, Right: 10}, Background: "theme.background",
	},
		c.attach,
		giode.Box(giode.Styles{FlexGrow: 1}, c.field),
		c.emoji,
		action,
	)
	return giode.Stack(giode.Styles{Width: -1}, bar, c.emojiBox, c.attachSheet.view())
}
