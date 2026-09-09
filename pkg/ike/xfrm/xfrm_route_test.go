package xfrm

import (
	"net"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestIPsecRouteRejectsInvalidArguments(t *testing.T) {
	link := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Index: 7, Name: "ipsec-1"}}

	tests := []struct {
		name      string
		link      netlink.Link
		localAddr net.IP
		peerAddr  net.IP
	}{
		{name: "nil interface", localAddr: net.ParseIP("10.0.0.2"), peerAddr: net.ParseIP("10.0.0.1")},
		{name: "invalid local address", link: link, peerAddr: net.ParseIP("10.0.0.1")},
		{name: "invalid peer address", link: link, localAddr: net.ParseIP("10.0.0.2")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ipsecRoute(tt.link, tt.localAddr, tt.peerAddr); err == nil {
				t.Fatal("ipsecRoute returned nil error")
			}
		})
	}
}

func TestIPsecRoute(t *testing.T) {
	link := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Index: 7, Name: "ipsec-1"}}
	route, err := ipsecRoute(link, net.ParseIP("10.0.0.2"), net.ParseIP("10.0.0.1"))
	if err != nil {
		t.Fatalf("ipsecRoute returned error: %v", err)
	}

	if route.LinkIndex != 7 {
		t.Errorf("LinkIndex = %d, want 7", route.LinkIndex)
	}
	if route.Scope != netlink.SCOPE_LINK {
		t.Errorf("Scope = %d, want %d", route.Scope, netlink.SCOPE_LINK)
	}
	if got := route.Src.String(); got != "10.0.0.2" {
		t.Errorf("Src = %s, want 10.0.0.2", got)
	}
	if got := route.Dst.String(); got != "10.0.0.1/32" {
		t.Errorf("Dst = %s, want 10.0.0.1/32", got)
	}
}
