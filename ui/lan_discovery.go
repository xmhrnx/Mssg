package ui

import (
	"time"

	"github.com/xmhrnx/giode/multicast"
)

// lanWindow is how long Android lets mDNS packets in after a start or a
// return to the screen; the lock costs battery, so it isn't held.
const lanWindow = time.Minute

// discoverLAN briefly lets Android receive mDNS, so contacts on the same
// Wi-Fi connect directly. Elsewhere it does nothing.
func discoverLAN() {
	release, err := multicast.Acquire("mssg-lan")
	if err != nil {
		return // without the lock, contacts are still found through the DHT
	}
	time.Sleep(lanWindow)
	release()
}
