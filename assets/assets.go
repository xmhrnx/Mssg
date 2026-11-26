// Package assets embeds the app's fonts and icons.
package assets

import "embed"

// Icons are Lucide icons (ISC license), one SVG per name.
//
//go:embed icons/*.svg
var Icons embed.FS

// Fonts are Inter (SIL Open Font License).
//
//go:embed fonts/*.ttf
var Fonts embed.FS
