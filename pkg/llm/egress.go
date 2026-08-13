package llm

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type EgressPolicy struct {
	AllowLoopback bool
}

func ValidateProviderURL(raw string, policy EgressPolicy) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("invalid provider URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("provider URL scheme must be http or https")
	}
	if u.User != nil {
		return fmt.Errorf("provider URL must not contain credentials")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" {
		if policy.AllowLoopback {
			return nil
		}
		return fmt.Errorf("provider URL resolves to a denied local destination")
	}
	if ip := net.ParseIP(host); ip != nil && deniedProviderIP(ip, policy.AllowLoopback) {
		return fmt.Errorf("provider URL resolves to a denied local destination")
	}
	return nil
}

func newEgressClient(timeout time.Duration, policy EgressPolicy) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, candidate := range ips {
				if deniedProviderIP(candidate.IP, policy.AllowLoopback) {
					return nil, fmt.Errorf("provider destination %s is denied", candidate.IP)
				}
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("provider host has no addresses")
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
	return &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many provider redirects")
		}
		return ValidateProviderURL(req.URL.String(), policy)
	}}
}

func deniedProviderIP(ip net.IP, allowLoopback bool) bool {
	if ip.IsLoopback() {
		return !allowLoopback
	}
	return ip.IsUnspecified() || ip.IsMulticast() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}
