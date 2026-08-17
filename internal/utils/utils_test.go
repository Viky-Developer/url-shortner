package utils

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vicky/url-shortner/internal/apperror"
)

func TestOptionalTimeUnmarshalValid(t *testing.T) {
	var ot OptionalTime
	if err := ot.UnmarshalJSON([]byte(`"2099-01-01T00:00:00Z"`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !ot.Valid {
		t.Fatal("expected Valid=true")
	}
}

func TestOptionalTimeUnmarshalNull(t *testing.T) {
	var ot OptionalTime
	if err := ot.UnmarshalJSON([]byte(`null`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if ot.Valid {
		t.Fatal("expected Valid=false for null")
	}
}

func TestOptionalTimeUnmarshalEmpty(t *testing.T) {
	var ot OptionalTime
	if err := ot.UnmarshalJSON([]byte(`""`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if ot.Valid {
		t.Fatal("expected Valid=false for empty string")
	}
}

func TestOptionalTimeUnmarshalInvalid(t *testing.T) {
	var ot OptionalTime
	err := ot.UnmarshalJSON([]byte(`"not-a-time"`))
	if err == nil {
		t.Fatal("expected error for invalid timestamp")
	}
}

func TestDerefInt16(t *testing.T) {
	if got := DerefInt16(nil); got != 0 {
		t.Errorf("DerefInt16(nil) = %d, want 0", got)
	}
	v := int16(42)
	if got := DerefInt16(&v); got != 42 {
		t.Errorf("DerefInt16(&42) = %d, want 42", got)
	}
}

func TestDerefInt32(t *testing.T) {
	if got := DerefInt32(nil); got != 0 {
		t.Errorf("DerefInt32(nil) = %d, want 0", got)
	}
	v := int32(99)
	if got := DerefInt32(&v); got != 99 {
		t.Errorf("DerefInt32(&99) = %d, want 99", got)
	}
}

func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"169.254.0.1", true},
		{"224.0.0.1", true},
		{"0.0.0.0", true},
		{"93.184.216.34", false},
		{"8.8.8.8", false},
	}
	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if ip == nil {
			t.Fatalf("failed to parse IP %q", tt.ip)
		}
		if got := IsBlockedIP(ip); got != tt.want {
			t.Errorf("IsBlockedIP(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestParsePositiveInt(t *testing.T) {
	if got := ParsePositiveInt("", 5); got != 5 {
		t.Errorf("empty = %d, want 5", got)
	}
	if got := ParsePositiveInt("abc", 5); got != 5 {
		t.Errorf("invalid = %d, want 5", got)
	}
	if got := ParsePositiveInt("0", 5); got != 5 {
		t.Errorf("zero = %d, want 5", got)
	}
	if got := ParsePositiveInt("-1", 5); got != 5 {
		t.Errorf("negative = %d, want 5", got)
	}
	if got := ParsePositiveInt("10", 5); got != 10 {
		t.Errorf("valid = %d, want 10", got)
	}
}

func TestValidateExpiresAtNil(t *testing.T) {
	if err := ValidateExpiresAt(OptionalTime{}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateExpiresAtFuture(t *testing.T) {
	ot := OptionalTime{Time: time.Now().Add(time.Hour), Valid: true}
	if err := ValidateExpiresAt(ot); err != nil {
		t.Errorf("unexpected error for future time: %v", err)
	}
}

func TestValidateExpiresAtPast(t *testing.T) {
	ot := OptionalTime{Time: time.Now().Add(-time.Hour), Valid: true}
	if err := ValidateExpiresAt(ot); err == nil {
		t.Error("expected error for past time")
	}
}

func TestParseID(t *testing.T) {
	id, err := ParseID("123")
	if err != nil {
		t.Fatalf("ParseID(123): %v", err)
	}
	if id != 123 {
		t.Errorf("ParseID(123) = %d, want 123", id)
	}

	if _, err := ParseID("abc"); err == nil {
		t.Error("expected error for non-numeric input")
	}
	if _, err := ParseID(""); err == nil {
		t.Error("expected error for empty input")
	}
}

func TestParsePagination(t *testing.T) {
	page, perPage, offset := ParsePagination("", "")
	if page != 1 || perPage != 10 || offset != 0 {
		t.Errorf("defaults: page=%d perPage=%d offset=%d", page, perPage, offset)
	}

	page, perPage, offset = ParsePagination("3", "20")
	if page != 3 || perPage != 20 || offset != 40 {
		t.Errorf("explicit: page=%d perPage=%d offset=%d", page, perPage, offset)
	}

	_, perPage, _ = ParsePagination("1", "200")
	if perPage != 100 {
		t.Errorf("perPage capped: got %d, want 100", perPage)
	}

	page, _, _ = ParsePagination("0", "10")
	if page != 1 {
		t.Errorf("page floor: got %d, want 1", page)
	}

	_, perPage, _ = ParsePagination("1", "0")
	if perPage != 10 {
		t.Errorf("perPage zero falls back to default: got %d, want 10", perPage)
	}
}

func TestValidateURLEmpty(t *testing.T) {
	if err := ValidateURL("", nil); err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestValidateURLNotHTTPS(t *testing.T) {
	if err := ValidateURL("http://example.com", nil); err == nil {
		t.Error("expected error for http URL")
	}
}

func TestValidateURLNotAURL(t *testing.T) {
	if err := ValidateURL("not-a-url", nil); err == nil {
		t.Error("expected error for non-URL string")
	}
}

func TestValidateURLMissingScheme(t *testing.T) {
	if err := ValidateURL("example.com/foo", nil); err == nil {
		t.Error("expected error for missing scheme")
	}
}

func TestValidateURLLocalhost(t *testing.T) {
	if err := ValidateURL("https://localhost:8080/x", nil); err == nil {
		t.Error("expected error for localhost")
	}
}

func TestValidateURLInternalSuffix(t *testing.T) {
	suffixes := []string{".local", ".internal", ".lan", ".corp", ".home"}
	for _, s := range suffixes {
		url := "https://host" + s + "/x"
		if err := ValidateURL(url, nil); err == nil {
			t.Errorf("expected error for suffix %s", s)
		}
	}
}

func TestValidateURLPrivateIP(t *testing.T) {
	if err := ValidateURL("https://192.168.1.1/x", nil); err == nil {
		t.Error("expected error for private IP literal")
	}
}

func TestValidateURLLoopbackIP(t *testing.T) {
	if err := ValidateURL("https://127.0.0.1/x", nil); err == nil {
		t.Error("expected error for loopback IP")
	}
}

func TestValidateURLIPv6Loopback(t *testing.T) {
	if err := ValidateURL("https://[::1]/x", nil); err == nil {
		t.Error("expected error for IPv6 loopback")
	}
}

func TestValidateURLPublicIP(t *testing.T) {
	lookup := func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	if err := ValidateURL("https://example.com/foo", lookup); err != nil {
		t.Errorf("unexpected error for public URL: %v", err)
	}
}

func TestValidateURLUnresolvableHost(t *testing.T) {
	lookup := func(host string) ([]net.IP, error) {
		return nil, errors.New("no such host")
	}
	if err := ValidateURL("https://nonexistent.example.com/x", lookup); err == nil {
		t.Error("expected error for unresolvable host")
	}
}

func TestValidateURLResolvesToPrivate(t *testing.T) {
	lookup := func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	}
	if err := ValidateURL("https://somehost.example.com/x", lookup); err == nil {
		t.Error("expected error for host resolving to private IP")
	}
}

func TestDecodeBodyValid(t *testing.T) {
	body := `{"name":"test"}`
	r := createRequest(t, body)
	var out map[string]string
	if err := DecodeBody(r, &out); err != nil {
		t.Fatalf("DecodeBody: %v", err)
	}
	if out["name"] != "test" {
		t.Errorf("name = %q, want test", out["name"])
	}
}

func TestDecodeBodyEOF(t *testing.T) {
	r := createRequest(t, "")
	var out map[string]string
	err := DecodeBody(r, &out)
	if err == nil {
		t.Fatal("expected error for empty body")
	}
	if !errors.Is(err, apperror.ErrInvalidPayload) {
		t.Errorf("expected ErrInvalidPayload, got %v", err)
	}
}

func TestDecodeBodyInvalidJSON(t *testing.T) {
	r := createRequest(t, "{invalid}")
	var out map[string]string
	err := DecodeBody(r, &out)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !errors.Is(err, apperror.ErrInvalidPayload) {
		t.Errorf("expected ErrInvalidPayload, got %v", err)
	}
}

func createRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	return req
}

func TestValidateURLAllBlockedSuffixes(t *testing.T) {
	suffixes := []string{".local", ".internal", ".lan", ".corp", ".home"}
	for _, s := range suffixes {
		url := "https://host" + s + "/path"
		err := ValidateURL(url, nil)
		if err == nil {
			t.Errorf("expected error for blocked suffix %s", s)
		}
		if !strings.Contains(err.Error(), "internal domain") {
			t.Errorf("suffix %s: error should mention 'internal domain', got: %s", s, err.Error())
		}
	}
}

func TestParsePaginationLargeValues(t *testing.T) {
	page, perPage, offset := ParsePagination("999999", "999999")
	if page < 1 {
		t.Errorf("page should be >= 1, got %d", page)
	}
	if perPage > 100 {
		t.Errorf("perPage should be capped at 100, got %d", perPage)
	}
	if offset < 0 {
		t.Errorf("offset should be >= 0, got %d", offset)
	}
}
