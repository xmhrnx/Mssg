package ui

import (
	"fmt"

	"gioui.org/unit"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/giode/styles/properties"

	"github.com/xmhrnx/mssg/assets"
)

// The app's palettes: a deep, quiet dark mode and a soft light one,
// with a violet-to-blue accent for what's yours.
var (
	darkPalette = giode.Palette{
		Primary: "#7C5CFF", Secondary: "#5B7CFF",
		Background: "#0B0D12", Surface: "#151922", Text: "#F3F5F9", Border: "#252B37",
		OnPrimary: "#FFFFFF", Muted: "#8A92A6", Accent: "#22D3EE",
		Success: "#2DD4A4", Warning: "#F5B544", Danger: "#FF5470", Info: "#38BDF8",
		Custom: map[string]string{
			"raised":   "#1C212C",
			"bubbleIn": "#1C212C",
			"chatBg":   "#0E1117",
			"scrim":    "#000000",
			"online":   "#2DD4A4",
			"subtle":   "#12151C",
		},
	}
	lightPalette = giode.Palette{
		Primary: "#6A4DFF", Secondary: "#4F6FFF",
		Background: "#F5F6FA", Surface: "#FFFFFF", Text: "#0E1220", Border: "#E3E6EE",
		OnPrimary: "#FFFFFF", Muted: "#6B7385", Accent: "#0EA5C6",
		Success: "#12A37C", Warning: "#C98A12", Danger: "#E5395A", Info: "#0284C7",
		Custom: map[string]string{
			"raised":   "#EEF0F6",
			"bubbleIn": "#FFFFFF",
			"chatBg":   "#EEF0F6",
			"scrim":    "#0E1220",
			"online":   "#12A37C",
			"subtle":   "#F9FAFC",
		},
	}
)

// Shared colors and gradients.
var (
	gradientMine   = giode.Linear(135, "#7C5CFF", "#4F7BFF")
	gradientHero   = giode.Linear(160, "#7C5CFF", "#22D3EE")
	gradientDanger = giode.Linear(135, "#FF5470", "#FF7A59")
	gradientAccept = giode.Linear(135, "#2DD4A4", "#22C3EE")
	hairline       = giode.Mix("theme.text", "theme.background", 0.07)
	pressTint      = giode.Mix("theme.text", "theme.surface", 0.06)
)

// Font sizes.
const (
	fsTiny  = 11
	fsSmall = 13
	fsBody  = 15
	fsTitle = 18
	fsHuge  = 28
)

// setupTheme registers fonts, icons and palettes.
func setupTheme() error {
	giode.SetPalette(giode.ModeDark, darkPalette)
	giode.SetPalette(giode.ModeLight, lightPalette)
	var files []giode.FontFile
	for _, f := range []struct {
		name   string
		weight giode.FontWeight
	}{
		{"Inter-Regular.ttf", giode.FontWeightNormal},
		{"Inter-Medium.ttf", giode.FontWeightMedium},
		{"Inter-SemiBold.ttf", giode.FontWeightSemiBold},
		{"Inter-Bold.ttf", giode.FontWeightBold},
	} {
		data, err := assets.Fonts.ReadFile("fonts/" + f.name)
		if err != nil {
			return fmt.Errorf("font %s: %w", f.name, err)
		}
		files = append(files, giode.FontFile{Family: "Inter", Weight: f.weight, Data: data})
	}
	if err := giode.RegisterFont(files...); err != nil {
		return err
	}
	giode.SetDefaultFont("Inter")
	return giode.RegisterIconsFS(assets.Icons, "icons")
}

// applyTheme switches light and dark from the settings.
func applyTheme(theme string) {
	switch theme {
	case "light":
		giode.SetMode(giode.ModeLight)
	case "dark":
		giode.SetMode(giode.ModeDark)
	default:
		if dark, err := giode.SystemDarkMode(); err != nil || dark {
			giode.SetMode(giode.ModeDark)
		} else {
			giode.SetMode(giode.ModeLight)
		}
	}
}

// icon draws a named Lucide icon.
func icon(name string, size int, color string) giode.Element {
	return giode.Icon(name, giode.Styles{Width: properties.Width(size), Height: properties.Height(size), Color: color})
}

// text is a Text in the app's body style.
func text(s string, size int, color string, weight ...giode.FontWeight) giode.Element {
	st := giode.Styles{FontSize: unit.Sp(size), Color: color}
	if len(weight) > 0 {
		st.FontWeight = weight[0]
	}
	return giode.Text(s, st)
}

// line is a one-line text cut off with an ellipsis.
func line(s string, size int, color string, weight ...giode.FontWeight) giode.Element {
	st := giode.Styles{FontSize: unit.Sp(size), Color: color, MaxLines: 1, TextOverflow: giode.TextOverflowEllipsis}
	if len(weight) > 0 {
		st.FontWeight = weight[0]
	}
	return giode.Text(s, st)
}

// iconButton makes a round ghost button around an icon.
func iconButton(name string, size int, label string, onClick func()) *giode.IconButtonElement {
	return giode.IconButton(icon(name, size, "theme.text"), giode.Styles{Background: "theme.text", BorderRadius: 999, Padding: giode.UniformInset(8)}).
		Variant(giode.IconButtonGhost).Label(label).OnClick(onClick)
}

// roundAction is a filled circular call-style button.
func roundAction(name string, size int, label string, gradient *giode.LinearGradient, onClick func()) *giode.IconButtonElement {
	return giode.IconButton(icon(name, size*45/100, "#FFFFFF"), giode.Styles{
		Width: properties.Width(size), Height: properties.Height(size), BorderRadius: 999, BackgroundGradient: gradient,
		BoxShadow: giode.BoxShadow{Y: 6, Blur: 18, Color: "#00000055"},
	}).Label(label).OnClick(onClick)
}

// primaryButton is the main call to action of a screen.
func primaryButton(label string, onClick func()) *giode.ButtonElement {
	return giode.Button(label, giode.Styles{
		BackgroundGradient: gradientMine, Color: "#FFFFFF", BorderRadius: 14,
		FontSize: fsBody, FontWeight: giode.FontWeightSemiBold, Padding: giode.SymmetricInset(20, 14), Width: -1,
	}).OnClick(onClick)
}

// secondaryButton is a quiet outlined button.
func secondaryButton(label string, onClick func()) *giode.ButtonElement {
	return giode.Button(label, giode.Styles{
		BorderColor: "theme.border", Color: "theme.text", BorderRadius: 14, BorderWidth: 1,
		FontSize: fsBody, FontWeight: giode.FontWeightMedium, Padding: giode.SymmetricInset(20, 13), Width: -1,
	}).Variant(giode.VariantOutlined).OnClick(onClick)
}

// textField is the app's rounded input.
func textField(placeholder string) *giode.InputElement {
	return giode.Input.Text(placeholder, giode.Styles{
		Background: "theme.raised", BorderColor: "theme.raised", BorderRadius: 14,
		Padding: giode.SymmetricInset(16, 13), FontSize: fsBody, Color: "theme.text",
	})
}

// wpx and hpx turn dp counts into Width and Height values.
func wpx(n int) properties.Width  { return properties.Width(n) }
func hpx(n int) properties.Height { return properties.Height(n) }
