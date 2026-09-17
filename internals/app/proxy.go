package app

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// proxyTrust decides whether a request's X-Forwarded-* headers may be
// believed, and reads the real client IP and scheme back out of them.
//
// SADE is normally reached through a TLS terminator it does not control the
// address of - cloudflared, nginx, Caddy - which connects from loopback and
// forwards the original client in X-Forwarded-For. Without this, every
// request looks like it came from 127.0.0.1, which would collapse the
// per-IP auth rate limiter into one global bucket for the whole internet.
//
// Trust is only ever extended to the immediate peer. A request arriving
// straight from the internet is read from RemoteAddr however many
// X-Forwarded-For headers it carries, so the headers are never a way to
// forge an identity - the same reason the forwarded chain is walked from
// the right and stops at the first entry that isn't itself a trusted proxy.
type proxyTrust struct {
	nets []netip.Prefix
}

// newProxyTrust parses cidrs (CIDRs or bare IPs); an unparseable entry is
// skipped, config.Validate having already reported it.
func newProxyTrust(cidrs []string) proxyTrust {
	var nets []netip.Prefix
	for _, raw := range cidrs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if p, err := netip.ParsePrefix(raw); err == nil {
			nets = append(nets, p.Masked())
			continue
		}
		if a, err := netip.ParseAddr(raw); err == nil {
			nets = append(nets, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	return proxyTrust{nets: nets}
}

// trusts reports whether addr is one of the configured proxies.
func (p proxyTrust) trusts(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	for _, n := range p.nets {
		if n.Contains(addr) {
			return true
		}
	}
	return false
}

// peer is the address the connection actually came from.
func peerAddr(r *http.Request) (netip.Addr, string) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, perr := netip.ParseAddr(host)
	if perr != nil {
		return netip.Addr{}, host
	}
	return addr.Unmap(), addr.Unmap().String()
}

// clientIP returns the address to attribute the request to: the forwarded
// client when the peer is a trusted proxy, otherwise the peer itself.
func (p proxyTrust) clientIP(r *http.Request) string {
	addr, raw := peerAddr(r)
	if !p.trusts(addr) {
		return raw
	}

	// Right to left: the rightmost entry is the one our own trusted proxy
	// appended, so the first entry that is not itself a proxy we trust is
	// the furthest we can still believe.
	hops := forwardedFor(r)
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(hops[i])
		if err != nil {
			continue
		}
		if !p.trusts(hop) {
			return hop.Unmap().String()
		}
	}
	return raw
}

// scheme reports the scheme the client used, which is not the scheme we were
// reached on: behind a terminator the hop into this process is plain HTTP.
func (p proxyTrust) scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	addr, _ := peerAddr(r)
	if p.trusts(addr) {
		if fp := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); fp != "" {
			// A chain appends, so take the first (client-most) value.
			if i := strings.IndexByte(fp, ','); i >= 0 {
				fp = strings.TrimSpace(fp[:i])
			}
			if strings.EqualFold(fp, "https") {
				return "https"
			}
			return "http"
		}
	}
	return "http"
}

// forwardedFor flattens every X-Forwarded-For header into one ordered list
// of hops (clients may send several headers; each may hold several values).
func forwardedFor(r *http.Request) []string {
	var hops []string
	for _, hdr := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(hdr, ",") {
			if part = strings.TrimSpace(part); part != "" {
				hops = append(hops, part)
			}
		}
	}
	return hops
}
