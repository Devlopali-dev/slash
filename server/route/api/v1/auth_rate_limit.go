package v1

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const (
	authRateWindow          = time.Minute
	authRateMaxBurst        = 10
	authRateCleanupInterval = 5 * time.Minute
)

var (
	authRateMu      sync.Mutex
	authRateClients = make(map[string]*rateBucket)
)

type rateBucket struct {
	tokens   int
	lastSeen time.Time
}

func init() {
	go func() {
		ticker := time.NewTicker(authRateCleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			pruneAuthRateClients()
		}
	}()
}

func pruneAuthRateClients() {
	cutoff := time.Now().Add(-authRateWindow * 2)
	authRateMu.Lock()
	defer authRateMu.Unlock()
	for ip, c := range authRateClients {
		if c.lastSeen.Before(cutoff) {
			delete(authRateClients, ip)
		}
	}
}

// extractClientIP returns the effective client IP.
//
// It reads the real TCP peer address via gRPC peer context. X-Forwarded-For is
// only trusted when the immediate peer is a loopback address (the local
// gRPC-gateway proxy). This prevents remote clients from forging their IP by
// injecting arbitrary XFF headers.
func extractClientIP(ctx context.Context) string {
	var peerHost string
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		host, _, err := net.SplitHostPort(p.Addr.String())
		if err == nil {
			peerHost = host
		}
	}

	// Only trust XFF when the request came through the local gateway (loopback peer).
	if isLoopbackIP(peerHost) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			for _, xff := range md["x-forwarded-for"] {
				if xff != "" {
					return strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
				}
			}
		}
	}

	return peerHost
}

func isLoopbackIP(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.IsLoopback()
}

// checkAuthRateLimit returns ResourceExhausted if the client IP has exceeded
// authRateMaxBurst authentication attempts within authRateWindow.
func checkAuthRateLimit(ctx context.Context) error {
	ip := extractClientIP(ctx)
	if ip == "" {
		return nil
	}

	authRateMu.Lock()
	defer authRateMu.Unlock()

	now := time.Now()
	c, ok := authRateClients[ip]
	if !ok || now.Sub(c.lastSeen) > authRateWindow {
		authRateClients[ip] = &rateBucket{tokens: authRateMaxBurst - 1, lastSeen: now}
		return nil
	}

	if c.tokens <= 0 {
		return status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}
	c.tokens--
	c.lastSeen = now
	return nil
}