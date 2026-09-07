package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"net"
	"time"
)

const spectatorCookieBytes = 16
const spectatorCookieWindow = 30 * time.Second

// subscribeCookie binds a token to one battle, observed endpoint and epoch.
// Length prefixes keep the encoding unambiguous, including embedded NULs.
func (r *SpectatorRegistry) subscribeCookie(battleCode string, remoteAddr *net.UDPAddr, epoch int64) []byte {
	mac := hmac.New(sha256.New, r.cookieKey[:])
	var encoded [8]byte
	for _, value := range []string{battleCode, remoteAddr.String()} {
		binary.BigEndian.PutUint64(encoded[:], uint64(len(value)))
		mac.Write(encoded[:])
		mac.Write([]byte(value))
	}
	binary.BigEndian.PutUint64(encoded[:], uint64(epoch))
	mac.Write(encoded[:])
	return mac.Sum(nil)[:spectatorCookieBytes]
}

func (r *SpectatorRegistry) validSubscribeCookie(battleCode string, remoteAddr *net.UDPAddr, cookie []byte, now time.Time) bool {
	if len(cookie) != spectatorCookieBytes {
		return false
	}
	epoch := now.Unix() / int64(spectatorCookieWindow/time.Second)
	// Accept the previous epoch so a challenge issued at a boundary remains
	// usable for at least one full window.
	return hmac.Equal(cookie, r.subscribeCookie(battleCode, remoteAddr, epoch)) ||
		hmac.Equal(cookie, r.subscribeCookie(battleCode, remoteAddr, epoch-1))
}
