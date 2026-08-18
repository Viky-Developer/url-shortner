// Package utils provides shared utility types used across internal packages.
package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/vicky/url-shortner/internal/apperror"
)

// OptionalTime is a time that also accepts null or empty-string JSON values,
// treating them as "not provided". It is used for optional request fields so an
// omitted or empty expiresAt never triggers a type conversion error.
type OptionalTime struct {
	Time  time.Time
	Valid bool
}

// UnmarshalJSON parses an RFC3339 timestamp. Null and empty strings set the
// value as invalid instead of raising a conversion error.
func (t *OptionalTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		t.Valid = false
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}
	t.Time = parsed
	t.Valid = true
	return nil
}

// SanitizeLog strips control characters (0x00-0x1F and 0x7F) from a string
// to prevent log injection via newlines, carriage returns, tabs, etc.
func SanitizeLog(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return '_'
		}
		return r
	}, s)
}

// DerefInt16 safely dereferences an int16 pointer, returning 0 if nil.
func DerefInt16(p *int16) int16 {
	if p == nil {
		return 0
	}
	return *p
}

// DerefInt32 safely dereferences an int32 pointer, returning 0 if nil.
func DerefInt32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

// IsBlockedIP reports whether ip is loopback, private, link-local, multicast,
// or unspecified — any of which make an address unsafe as a redirect target.
func IsBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

// ParsePositiveInt parses value as a positive int32, falling back to the
// provided default when the value is empty or invalid.
func ParsePositiveInt(value string, fallback int32) int32 {
	if value == "" {
		return fallback
	}
	n, err := strconv.ParseInt(value, 10, 32)
	if err != nil || n < 1 {
		return fallback
	}
	return int32(n)
}

// ValidateExpiresAt ensures that when an expiration time is provided it is not
// in the past (i.e. it must be the current moment or a future time).
func ValidateExpiresAt(e OptionalTime) error {
	if !e.Valid {
		return nil
	}
	if e.Time.Before(time.Now()) {
		return fmt.Errorf("expiresAt must not be in the past")
	}
	return nil
}

// ParseID extracts an integer id from a string path segment.
func ParseID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id: %s", value)
	}
	return id, nil
}

// ParsePagination computes page, perPage, and offset from query parameters,
// clamping values to safe ranges: page >= 1, 1 <= perPage <= 100.
func ParsePagination(pageStr, perPageStr string) (page, perPage, offset int32) {
	page = max(ParsePositiveInt(pageStr, 1), 1)
	perPage = min(max(ParsePositiveInt(perPageStr, 10), 1), 100)

	o := int64(page-1) * int64(perPage)
	offset = int32(min(max(o, 0), math.MaxInt32))

	return page, perPage, offset
}

// LookupFunc is the function signature for DNS resolution.
type LookupFunc func(host string) ([]net.IP, error)

// ValidateURL checks that the raw URL is a well-formed https URL and does not
// point at a localhost, internal, private, or loopback destination. DNS
// resolution is performed using the provided lookup function.
func ValidateURL(rawURL string, lookupDNS LookupFunc) error {
	if rawURL == "" {
		return fmt.Errorf("originalURL is required")
	}

	parsed, err := neturl.ParseRequestURI(rawURL)
	if err != nil || parsed.Scheme != "https" {
		return fmt.Errorf("URL must start with https://")
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("invalid host in URL")
	}

	if host == "localhost" {
		return fmt.Errorf("localhost is not allowed as destination")
	}

	// Internal TLD suffixes are never allowed.
	blockedSuffixes := []string{".local", ".internal", ".lan", ".corp", ".home"}
	for _, suffix := range blockedSuffixes {
		if strings.HasSuffix(host, suffix) {
			return fmt.Errorf("internal domain '%s' is not allowed", host)
		}
	}

	// If the host is an IP literal, validate it directly.
	if ip := net.ParseIP(host); ip != nil {
		if IsBlockedIP(ip) {
			return fmt.Errorf("private or loopback IP is not allowed")
		}
		return nil
	}

	// Otherwise resolve the host and reject any private/loopback addresses.
	ips, err := lookupDNS(host)
	if err != nil {
		return fmt.Errorf("unable to resolve host '%s'", host)
	}
	for _, ip := range ips {
		if IsBlockedIP(ip) {
			return fmt.Errorf("host '%s' resolves to a private or loopback IP", host)
		}
	}

	return nil
}

// decodeBody decodes the request body into v.
func DecodeBody(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%w: request body is required", apperror.ErrInvalidPayload)
		}
		return fmt.Errorf("%w: %v", apperror.ErrInvalidPayload, err)
	}
	return nil
}
