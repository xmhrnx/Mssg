package ui

import (
	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/styles/properties"
)

// avatarView draws a contact's picture, or initials, with an online dot.
func (a *App) avatarView(name string, picture []byte, size int, online bool) giode.Element {
	av := giode.Avatar(name, giode.Styles{Width: properties.Width(size)})
	if img := a.images.FromBytes(picture); img != nil {
		av.Image(img)
	}
	if !online {
		return av
	}
	dot := max(10, size/4)
	return giode.Box(giode.Styles{Width: properties.Width(size), Height: properties.Height(size)},
		av,
		giode.Box(giode.Styles{
			Position: giode.PositionAbsolute, Right: giode.Flush, Bottom: giode.Flush,
			Width: properties.Width(dot), Height: properties.Height(dot), BorderRadius: 999,
			Background: "theme.online", BorderWidth: 2, BorderColor: "theme.background",
		}),
	)
}
