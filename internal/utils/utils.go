// Package utils provides shared utility types used across internal packages.
package utils

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	neturl "net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sqlc-dev/pqtype"
	"github.com/vicky/url-shortner/internal/apperror"
)

// UnixMilliTime is a time.Time that accepts both RFC3339 strings and Unix
// milliseconds (as JSON numbers) in requests. An omitted, null, or zero value
// means "not provided".
type UnixMilliTime struct {
	Time  time.Time
	Valid bool
}

// UnmarshalJSON parses either an RFC3339 string or a Unix millisecond number.
func (t *UnixMilliTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		t.Valid = false
		return nil
	}

	// Try parsing as Unix milliseconds (number)
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		if ms == 0 {
			t.Valid = false
			return nil
		}
		t.Time = time.UnixMilli(ms)
		t.Valid = true
		return nil
	}

	// Fall back to RFC3339
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}
	t.Time = parsed
	t.Valid = true
	return nil
}

// MarshalJSON returns null if invalid, otherwise RFC3339 string.
func (t UnixMilliTime) MarshalJSON() ([]byte, error) {
	if !t.Valid {
		return []byte("null"), nil
	}
	return []byte(`"` + t.Time.Format(time.RFC3339) + `"`), nil
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
func ValidateExpiresAt(e UnixMilliTime) error {
	if !e.Valid {
		return nil
	}
	if e.Time.Before(time.Now()) {
		return fmt.Errorf("expiresAt must be greater than the current time")
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

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// ValidateEmail validates email format using regex.
func ValidateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("email is required")
	}
	if !emailRegex.MatchString(email) {
		return fmt.Errorf("invalid email format")
	}
	return nil
}

// ValidatePassword validates password strength:
// - At least 1 lowercase letter
// - At least 1 uppercase letter
// - At least 1 number
// - Maximum 8 characters
func ValidatePassword(password string) error {

	if len(password) < 8 {
		return fmt.Errorf("%w: password must be at most 8 characters", apperror.ErrInvalidPayload)
	}

	hasLower := false
	hasUpper := false
	hasNumber := false

	for _, r := range password {
		switch {
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			hasNumber = true
		}
	}

	var missing []string
	if !hasLower {
		missing = append(missing, "lowercase letter")
	}
	if !hasUpper {
		missing = append(missing, "uppercase letter")
	}
	if !hasNumber {
		missing = append(missing, "number")
	}

	if len(missing) > 0 {
		return fmt.Errorf("%w: password must contain at least one %s", apperror.ErrInvalidPayload, strings.Join(missing, ", "))
	}

	return nil
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

	if slices.ContainsFunc(ips, IsBlockedIP) {
		return fmt.Errorf("host '%s' resolves to a private or loopback IP", host)
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

// NullString returns a sql.NullString with the given value if non-empty,
// otherwise returns an invalid NullString.
func NullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// NullIP returns a pqtype.Inet for the given IP address string.
// An empty or unparseable string returns an invalid pqtype.Inet.
func NullIP(ipString string) pqtype.Inet {
	if ipString == "" {
		return pqtype.Inet{Valid: false}
	}
	ip := net.ParseIP(ipString)
	if ip == nil {
		return pqtype.Inet{Valid: false}
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	return pqtype.Inet{IPNet: net.IPNet{IP: ip, Mask: net.CIDRMask(len(ip)*8, len(ip)*8)}, Valid: true}
}

// ParseBrowser extracts the primary browser name from a User-Agent string.
func ParseBrowser(ua string) string {
	lower := strings.ToLower(ua)
	switch {
	case strings.Contains(lower, "edg/") || strings.Contains(lower, "edge/"):
		return "Edge"
	case strings.Contains(lower, "opr/") || strings.Contains(lower, "opera"):
		return "Opera"
	case strings.Contains(lower, "chrome") && !strings.Contains(lower, "edg"):
		return "Chrome"
	case strings.Contains(lower, "firefox"):
		return "Firefox"
	case strings.Contains(lower, "safari") && !strings.Contains(lower, "chrome"):
		return "Safari"
	case strings.Contains(lower, "msie") || strings.Contains(lower, "trident"):
		return "IE"
	default:
		return "Other"
	}
}

// ParseDeviceType extracts the device type from a User-Agent string.
func ParseDeviceType(ua string) string {
	lower := strings.ToLower(ua)
	switch {
	case strings.Contains(lower, "mobile") || strings.Contains(lower, "android") && !strings.Contains(lower, "tablet"):
		return "Mobile"
	case strings.Contains(lower, "tablet") || strings.Contains(lower, "ipad"):
		return "Tablet"
	default:
		return "Desktop"
	}
}
