package main

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestSpectatorCookie_BindsBattleAddressAndEpoch(t *testing.T) {
	r := newTestSpectatorRegistry()
	const epoch = int64(12345)
	now := time.Unix(epoch*30, 0)
	addr := testAddr(40000)
	cookie := r.subscribeCookie("battle", addr, epoch)
	assertEq(t, spectatorCookieBytes, len(cookie))
	assertEq(t, true, r.validSubscribeCookie("battle", addr, cookie, now))
	assertEq(t, false, r.validSubscribeCookie("other", addr, cookie, now))
	assertEq(t, false, r.validSubscribeCookie("battle", testAddr(40001), cookie, now))
	otherIP := &net.UDPAddr{IP: net.ParseIP("127.0.0.2"), Port: addr.Port}
	assertEq(t, false, r.validSubscribeCookie("battle", otherIP, cookie, now))
	assertEq(t, false, newTestSpectatorRegistry().validSubscribeCookie("battle", addr, cookie, now))

	for _, size := range []int{0, 1, spectatorCookieBytes - 1, spectatorCookieBytes + 1, 1024} {
		assertEq(t, false, r.validSubscribeCookie("battle", addr, make([]byte, size), now))
	}
	cookie[0] ^= 1
	assertEq(t, false, r.validSubscribeCookie("battle", addr, cookie, now))
}

func TestSpectatorCookie_ExpiresAfterPreviousWindow(t *testing.T) {
	r := newTestSpectatorRegistry()
	const epoch = int64(12345)
	addr := testAddr(40000)
	cookie := r.subscribeCookie("battle", addr, epoch)
	boundary := time.Unix((epoch+1)*30, 0)
	assertEq(t, true, r.validSubscribeCookie("battle", addr, cookie, boundary.Add(-time.Nanosecond)))
	assertEq(t, true, r.validSubscribeCookie("battle", addr, cookie, boundary))
	assertEq(t, true, r.validSubscribeCookie("battle", addr, cookie, boundary.Add(spectatorCookieWindow-time.Nanosecond)))
	assertEq(t, false, r.validSubscribeCookie("battle", addr, cookie, boundary.Add(spectatorCookieWindow)))
	assertEq(t, false, r.validSubscribeCookie("battle", addr, cookie, time.Unix((epoch-1)*30, 0)))
}

func TestSpectatorCookie_IPv4AndIPv6(t *testing.T) {
	r := newTestSpectatorRegistry()
	now := time.Unix(12345*30, 0)
	for _, ip := range []string{"192.0.2.1", "2001:db8::1", "fe80::1"} {
		t.Run(ip, func(t *testing.T) {
			addr := &net.UDPAddr{IP: net.ParseIP(ip), Port: 1234, Zone: "en0"}
			cookie := r.subscribeCookie("battle\x00with\x00separators", addr, 12345)
			assertEq(t, true, r.validSubscribeCookie("battle\x00with\x00separators", addr, cookie, now))
			assertEq(t, false, r.validSubscribeCookie("battle\x00with", addr, cookie, now))
		})
	}
	addr4 := &net.UDPAddr{IP: net.IPv4(192, 0, 2, 1).To4(), Port: 1234}
	addr16 := &net.UDPAddr{IP: net.ParseIP("::ffff:192.0.2.1"), Port: 1234}
	assertEq(t, true, bytes.Equal(r.subscribeCookie("battle", addr4, 12345), r.subscribeCookie("battle", addr16, 12345)))
	addr6 := &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: 1234, Zone: "en0"}
	cookie := r.subscribeCookie("battle", addr6, 12345)
	addr6.Zone = "en1"
	assertEq(t, false, r.validSubscribeCookie("battle", addr6, cookie, now))
}
