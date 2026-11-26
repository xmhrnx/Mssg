package ui

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
)

// safetyNumber derives the same 60 digits on both devices from the two
// peer IDs; if they match on both screens, no one is in between.
func safetyNumber(a, b string) string {
	if a > b {
		a, b = b, a
	}
	sum := sha256.Sum256([]byte("mssg-safety-v1\x00" + a + "\x00" + b))
	more := sha256.Sum256(sum[:])
	digits := append(sum[:], more[:]...)
	var groups []string
	for i := 0; i < 12; i++ {
		v := binary.BigEndian.Uint32(digits[i*4:]) % 100000
		groups = append(groups, fmt.Sprintf("%05d", v))
	}
	var rows []string
	for i := 0; i < 12; i += 4 {
		rows = append(rows, strings.Join(groups[i:i+4], "  "))
	}
	return strings.Join(rows, "\n")
}
