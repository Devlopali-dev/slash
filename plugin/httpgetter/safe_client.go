package httpgetter

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// privateIPNets is the set of IP ranges that must never be contacted by the httpgetter.
var privateIPNets = func() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, network, _ := net.ParseCIDR(cidr)
		nets = append(nets, network)
	}
	return nets
}()

// safeHTTPClient returns an http.Client that refuses connections to private or
// reserved IP addresses, preventing SSRF attacks including DNS rebinding.
func safeHTTPClient() *http.Client {
	dialer := &net.Dialer{}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				ips, err := net.DefaultResolver.LookupHost(ctx, host)
				if err != nil {
					return nil, err
				}
				for _, ipStr := range ips {
					ip := net.ParseIP(ipStr)
					if ip == nil {
						continue
					}
					for _, network := range privateIPNets {
						if network.Contains(ip) {
							return nil, errors.Errorf("connection to private address %s is not allowed", ipStr)
						}
					}
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0], port))
			},
		},
	}
}
