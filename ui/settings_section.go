package ui

import "github.com/xmhrnx/giode"

// section is a titled card of settings rows.
func section(title string, rows ...giode.Element) giode.Element {
	items := make([]giode.Element, 0, len(rows)*2)
	for i, r := range rows {
		if i > 0 {
			items = append(items, giode.Divider(giode.Styles{Color: hairline}))
		}
		items = append(items, r)
	}
	return giode.Stack(giode.Styles{Width: -1, Gap: 8},
		giode.Box(giode.Styles{Padding: giode.Inset{Left: 6}}, text(title, fsSmall, "theme.muted", giode.FontWeightSemiBold)),
		giode.Stack(giode.Styles{Width: -1, Background: "theme.surface", BorderRadius: 16, Padding: giode.SymmetricInset(16, 6)}, items...),
	)
}

// settingRow is a label, a hint below it and a control at the end.
func settingRow(title, hint string, control giode.Element) giode.Element {
	left := []giode.Element{text(title, fsBody, "theme.text", giode.FontWeightMedium)}
	if hint != "" {
		left = append(left, text(hint, fsSmall, "theme.muted"))
	}
	row := []giode.Element{giode.Stack(giode.Styles{Gap: 3, FlexGrow: 1}, left...)}
	if control != nil {
		row = append(row, control)
	}
	return giode.Row(giode.Styles{Width: -1, Gap: 12, Align: giode.AlignCenter, Padding: giode.SymmetricInset(0, 12), Wrap: giode.WrapNoWrap}, row...)
}

// screenHeader is a back button and a title.
func screenHeader(back *giode.IconButtonElement, title string) giode.Element {
	return giode.Row(giode.Styles{Width: -1, Align: giode.AlignCenter, Gap: 6, Padding: giode.Inset{Top: 8, Bottom: 8, Left: 4, Right: 12}, Wrap: giode.WrapNoWrap},
		back, text(title, fsTitle+1, "theme.text", giode.FontWeightBold))
}
