package ui

import (
	"fmt"
	"strings"
	"time"
)

// listTime formats a message time: the time today, the weekday this week,
// else the date.
func listTime(t time.Time) string {
	now := time.Now()
	switch {
	case sameDay(t, now):
		return t.Format("15:04")
	case sameDay(t, now.AddDate(0, 0, -1)):
		return "Yesterday"
	case now.Sub(t) < 6*24*time.Hour:
		return t.Format("Mon")
	case t.Year() == now.Year():
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2, 2006")
}

func dayLabel(t time.Time) string {
	now := time.Now()
	switch {
	case sameDay(t, now):
		return "Today"
	case sameDay(t, now.AddDate(0, 0, -1)):
		return "Yesterday"
	case t.Year() == now.Year():
		return t.Format("Monday, January 2")
	}
	return t.Format("January 2, 2006")
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func lastSeen(t time.Time) string {
	if t.IsZero() {
		return "offline"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "last seen just now"
	case d < time.Hour:
		return fmt.Sprintf("last seen %d min ago", int(d.Minutes()))
	case sameDay(t, time.Now()):
		return "last seen today at " + t.Format("15:04")
	}
	return "last seen " + listTime(t)
}

func fileSize(n int64) string {
	switch {
	case n < 1<<10:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	case n < 1<<30:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
}

func duration(d time.Duration) string {
	s := int(d.Round(time.Second).Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// shortID shows a peer ID's tail, the part that differs between people.
func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:6] + "…" + id[len(id)-6:]
}

// groupID splits an ID into groups of four for reading aloud.
func groupID(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && i%4 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
