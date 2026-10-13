package server

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	ma "github.com/multiformats/go-multiaddr"
)

const (
	// DefaultHTTPListen is where `ipfs mcp --http` binds without an address.
	// 4000 is not used: it belongs to the liter-llm proxy on the operator's
	// machine.
	DefaultHTTPListen = "127.0.0.1:5050"
	// HTTPListenEnv overrides the default listen address; a flag overrides it.
	HTTPListenEnv = "IPFS_MCP_HTTP_LISTEN"
)

// ReservedPorts are ports of other local services. The contract's config
// schema (ListenAddress) lists the same ports; a test keeps them in step.
var ReservedPorts = []int{4000, 4001, 5001, 8080, 1906, 1907, 1908, 50051, 8181, 8081, 11434, 23001, 28000}

// Listen is a parsed listen address.
type Listen struct {
	// Host is an IP literal or a DNS name; never empty.
	Host string
	// Port is 0 only when the caller asked the OS to choose (tests).
	Port int
}

// ParseListen reads "host:port", "[v6]:port" or a /ip4|/ip6|/dns*/<host>/tcp/<port>
// multiaddr, and rejects the ports reserved for other local services. An empty
// string selects DefaultHTTPListen.
func ParseListen(s string) (Listen, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		s = DefaultHTTPListen
	}
	host, portStr, err := splitListen(s)
	if err != nil {
		return Listen{}, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > 65535 {
		return Listen{}, fmt.Errorf("listen address %q: %q is not a valid port", s, portStr)
	}
	if host == "" {
		return Listen{}, fmt.Errorf("listen address %q: the host is empty; name a loopback address such as 127.0.0.1", s)
	}
	if slices.Contains(ReservedPorts, port) {
		return Listen{}, fmt.Errorf("listen address %q: port %d is reserved for another local service (%v)", s, port, ReservedPorts)
	}
	return Listen{Host: host, Port: port}, nil
}

func splitListen(s string) (host, port string, err error) {
	if strings.HasPrefix(s, "/") {
		return splitMultiaddr(s)
	}
	host, port, err = net.SplitHostPort(s)
	if err != nil {
		return "", "", fmt.Errorf("listen address %q: want host:port or a /ip4/.../tcp/<port> multiaddr: %w", s, err)
	}
	return host, port, nil
}

func splitMultiaddr(s string) (host, port string, err error) {
	addr, err := ma.NewMultiaddr(s)
	if err != nil {
		return "", "", fmt.Errorf("listen address %q: %w", s, err)
	}
	comps := []ma.Component(addr)
	if len(comps) != 2 || comps[1].Protocol().Code != ma.P_TCP {
		return "", "", fmt.Errorf("listen address %q: want /ip4|/ip6|/dns*/<host>/tcp/<port>", s)
	}
	switch comps[0].Protocol().Code {
	case ma.P_IP4, ma.P_IP6, ma.P_DNS, ma.P_DNS4, ma.P_DNS6:
	default:
		return "", "", fmt.Errorf("listen address %q: want /ip4|/ip6|/dns*/<host>/tcp/<port>", s)
	}
	return comps[0].Value(), comps[1].Value(), nil
}

// String is the host:port form.
func (l Listen) String() string { return net.JoinHostPort(l.Host, strconv.Itoa(l.Port)) }

// BindAddr is the address handed to net.Listen. "localhost" binds IPv4
// loopback so the result does not depend on how the resolver orders its answers.
func (l Listen) BindAddr() string {
	if strings.EqualFold(l.Host, "localhost") {
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(l.Port))
	}
	return l.String()
}

// IsLoopback reports whether the listener is reachable only from this machine.
// DNS names other than localhost are not loopback, and neither are the
// unspecified addresses 0.0.0.0 and ::.
func (l Listen) IsLoopback() bool { return isLoopbackHost(l.Host) }

// isLoopbackHost reports whether host is localhost or a loopback IP literal.
func isLoopbackHost(host string) bool {
	host = strings.TrimSuffix(strings.Trim(host, "[]"), ".")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return ip.Unmap().IsLoopback()
}

// errNonLoopbackWithoutAuth is returned when a non-loopback bind has no
// verifier.
var errNonLoopbackWithoutAuth = errors.New("refusing to listen on a non-loopback address without authentication")

// hostOnly returns the host part of a Host header value without the port.
func hostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return strings.Trim(hostport, "[]")
}
