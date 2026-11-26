// Command mssg is a decentralized, end-to-end encrypted messenger:
// contacts meet once through a magic wormhole code, then message, share
// files and call each other peer to peer.
package main

import (
	"image"
	"log"

	_ "gioui.org/app/permission/camera"
	_ "gioui.org/app/permission/microphone"
	_ "gioui.org/app/permission/networkstate"

	"github.com/xmhrnx/giode"

	"github.com/xmhrnx/mssg/ui"
)

func main() {
	if err := ui.Setup(); err != nil {
		log.Fatal(err)
	}
	root := ui.New()
	app := giode.New(giode.Config{
		Title:             "Mssg",
		Size:              image.Pt(420, 860),
		MinSize:           image.Pt(340, 520),
		Resizable:         true,
		Background:        "theme.background",
		FollowSystemTheme: true,
		OnBackground:      root.OnBackground,
		OnForeground:      root.OnForeground,
		SingleInstanceLock: &giode.SingleInstanceLock{
			UniqueID: "6d3f0b7e-2c1a-4f7e-9a51-mssg-desktop",
		},
	})
	root.Attach(app)
	if err := app.Run(root.View); err != nil {
		log.Fatal(err)
	}
}
