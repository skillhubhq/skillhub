package hubhttp

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

func (s *Server) trusted(ip netip.Addr) bool {
	for _, prefix := range s.proxies {
		if prefix.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}

// 只在直接对端可信时解析代理链，从右向左取第一个不可信地址；畸形链回退到对端。
func (s *Server) clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || !s.trusted(peer) {
		return host
	}
	raw := r.Header.Get("X-Forwarded-For")
	if raw == "" || len(raw) > 4096 {
		return host
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 32 {
		return host
	}
	addresses := make([]netip.Addr, len(parts))
	for i, part := range parts {
		ip, e := netip.ParseAddr(strings.TrimSpace(part))
		if e != nil {
			return host
		}
		addresses[i] = ip.Unmap()
	}
	for i := len(addresses) - 1; i >= 0; i-- {
		if !s.trusted(addresses[i]) {
			return addresses[i].String()
		}
	}
	return addresses[0].String()
}
