package subnet

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"google.golang.org/grpc/metadata"
)

type Validator struct {
	ipNet *net.IPNet
}

func NewValidator(trustedSubnet string) (*Validator, error) {
	if trustedSubnet == "" {
		return nil, nil
	}

	_, ipNet, err := net.ParseCIDR(trustedSubnet)
	if err != nil {
		return nil, fmt.Errorf("invalid trusted subnet format: %w", err)
	}

	return &Validator{ipNet: ipNet}, nil
}

func (v *Validator) IsTrusted(ipStr string) bool {
	if v == nil || v.ipNet == nil {
		return true
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	return v.ipNet.Contains(ip)
}

func ExtractIPFromRequest(r *http.Request) string {
	// Check X-Forwarded-For header
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}

	// Fallback to RemoteAddr
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

func ExtractIPFromGRPCContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	if values := md.Get("x-real-ip"); len(values) > 0 {
		return values[0]
	}
	return ""
}
