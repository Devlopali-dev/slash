package v1

import (
	"context"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	authRateWindow   = time.Minute
	authRateMaxBurst = 10
)

var (
	authRateMu     sync.Mutex
	authRateClients = make(map[string]*rateBucket)
)

type rateBucket struct {
	tokens   int
	lastSeen time.Time
}

func init() {
	// Pre-fill tokens for each new client.
}

// extractClientIP extracts the client IP from gRPC metadata.
// It checks x-forwarded-for, x-real-ip, and cookie headers that may contain IP info.
func extractClientIP(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	for _, vals := range md["x-forwarded-for"] {
		if vals != "" {
			return strings.TrimSpace(strings.Split(vals, ",")[0])
		}
	}
	for _, vals := range md["x-real-ip"] {
		if vals != "" {
			return strings.TrimSpace(vals)
		}
	}
	// Fallback: use the first non-empty metadata value.
	// This is best-effort for direct gRPC without gateway.
	for _, vals := range md {
		for _, v := range vals {
			if v != "" {
				return v
			}
		}
	}
	return ""
}

// checkAuthRateLimit returns an error if the client has exceeded the rate limit.
func checkAuthRateLimit(ctx context.Context) error {
	ip := extractClientIP(ctx)
	if ip == "" {
		return nil // Cannot determine IP, skip rate limiting.
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