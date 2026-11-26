package ui

import (
	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/styles/properties"
)

// splash is shown while the engine starts, or with the reason it couldn't.
func splash(failure string) giode.Element {
	body := []giode.Element{logoMark(88)}
	if failure == "" {
		body = append(body, giode.Spinner(giode.Styles{Width: 28, Color: "theme.primary"}))
	} else {
		body = append(body,
			text("Couldn't start", fsTitle, "theme.text", giode.FontWeightSemiBold),
			giode.Text(failure, giode.Styles{FontSize: fsSmall, Color: "theme.muted", TextAlign: giode.TextAlignCenter, MaxWidth: 320}),
		)
	}
	return giode.Stack(giode.Styles{Width: -1, Height: -1, Align: giode.AlignCenter, Justify: giode.JustifyCenter, Gap: 24, Padding: giode.UniformInset(24)}, body...)
}

// logoMark is the app's mark: a speech bubble on the brand gradient.
func logoMark(size int) giode.Element {
	return giode.Box(giode.Styles{
		Width: wpx(size), Height: hpx(size), BorderRadius: properties.BorderRadius(size * 3 / 10),
		BackgroundGradient: gradientHero, Align: giode.AlignCenter, Justify: giode.JustifyCenter,
		BoxShadow: giode.BoxShadow{Y: 12, Blur: 32, Color: "#7C5CFF55"},
	}, icon("message-circle", size/2, "#FFFFFF"))
}
