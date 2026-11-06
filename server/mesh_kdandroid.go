//go:build kdandroid

package main

// Android blocks apps from listing network interfaces (the netlink socket), so the
// embedded Tailscale node failed to start with "netlinkrib: permission denied".
// androidbin gives Tailscale a fallback interface list and Android-friendly DNS
// and CA-certificate handling.

import (
	_ "tailscale.com/feature/androidbin"
	"tailscale.com/net/netns"
)

func init() {
	// Tailscale pins its sockets to a network interface (SO_MARK, else SO_BINDTODEVICE).
	// An app can't set marks, so the linux build fell back to binding to "lo" and every
	// connection to Tailscale timed out. tsnet runs on a userspace network stack, so
	// there is no routing loop to avoid: plain sockets are right here.
	netns.SetEnabled(false)
}
