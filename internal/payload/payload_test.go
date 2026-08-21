package payload

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCreateURLRequestJSON(t *testing.T) {
	req := CreateURLRequest{
		OriginalURL: "https://example.com",
		CustomCode:  "my-code",
		Title:       "Test",
		Description: "A test URL",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded CreateURLRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.OriginalURL != req.OriginalURL {
		t.Errorf("OriginalURL = %q, want %q", decoded.OriginalURL, req.OriginalURL)
	}
	if decoded.CustomCode != req.CustomCode {
		t.Errorf("CustomCode = %q, want %q", decoded.CustomCode, req.CustomCode)
	}
	if decoded.Title != req.Title {
		t.Errorf("Title = %q, want %q", decoded.Title, req.Title)
	}
}

func TestUpdateURLRequestJSON(t *testing.T) {
	body := `{"originalURL":"https://updated.com","title":"Updated","status":1}`

	var decoded UpdateURLRequest
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.OriginalURL != "https://updated.com" {
		t.Errorf("OriginalURL = %q, want %q", decoded.OriginalURL, "https://updated.com")
	}
	if decoded.Status == nil || *decoded.Status != 1 {
		t.Error("expected Status to be 1")
	}
}

func TestUpdateURLRequestWithoutStatus(t *testing.T) {
	body := `{"originalURL":"https://updated.com","title":"Updated"}`

	var decoded UpdateURLRequest
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.OriginalURL != "https://updated.com" {
		t.Errorf("OriginalURL = %q, want %q", decoded.OriginalURL, "https://updated.com")
	}
	if decoded.Status != nil {
		t.Errorf("expected Status to be nil, got %v", decoded.Status)
	}
}

func TestURLResponseJSON(t *testing.T) {
	now := time.Now()
	resp := URLResponse{
		ID:          1,
		UserID:      "USR_test",
		ShortCode:   "abc123",
		OriginalURL: "https://example.com",
		ShortURL:    "http://localhost/abc123",
		IsActive:    true,
		ClickCount:  10,
		CreatedAt:   now.Format(time.RFC3339),
		UpdatedAt:   now.Format(time.RFC3339),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded URLResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != resp.ID {
		t.Errorf("ID = %d, want %d", decoded.ID, resp.ID)
	}
	if decoded.ShortCode != resp.ShortCode {
		t.Errorf("ShortCode = %q, want %q", decoded.ShortCode, resp.ShortCode)
	}
	if !decoded.IsActive {
		t.Error("expected IsActive to be true")
	}
}

func TestURLListResponseJSON(t *testing.T) {
	resp := URLListResponse{
		Items:      []URLResponse{{ID: 1, ShortCode: "abc"}},
		Total:      25,
		Page:       1,
		PerPage:    10,
		TotalPages: 3,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded URLListResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Total != 25 {
		t.Errorf("Total = %d, want 25", decoded.Total)
	}
	if len(decoded.Items) != 1 {
		t.Errorf("Items length = %d, want 1", len(decoded.Items))
	}
}

func TestDeleteResponseJSON(t *testing.T) {
	resp := DeleteResponse{
		ID:        1,
		ShortCode: "abc123",
		Message:   "soft deleted",
		DeletedAt: time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded DeleteResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Message != resp.Message {
		t.Errorf("Message = %q, want %q", decoded.Message, resp.Message)
	}
}

func TestSessionResponseJSON(t *testing.T) {
	resp := SessionResponse{
		ID:           1,
		DeviceType:   "web",
		DeviceName:   "Chrome",
		IPAddress:    "127.0.0.1",
		LoggedInAt:   "2026-01-01T00:00:00Z",
		LastActiveAt: "2026-01-01T01:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded SessionResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.DeviceType != "web" {
		t.Errorf("DeviceType = %q, want %q", decoded.DeviceType, "web")
	}
	if decoded.IPAddress != "127.0.0.1" {
		t.Errorf("IPAddress = %q, want %q", decoded.IPAddress, "127.0.0.1")
	}
}

func TestAuthResponseJSON(t *testing.T) {
	resp := AuthResponse{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		User: UserResponse{
			ID:          "USR_abc",
			Email:       "test@example.com",
			DisplayName: "Test User",
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded AuthResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.AccessToken != "access-token" {
		t.Errorf("AccessToken = %q, want %q", decoded.AccessToken, "access-token")
	}
	if decoded.User.Email != "test@example.com" {
		t.Errorf("User.Email = %q, want %q", decoded.User.Email, "test@example.com")
	}
}

func TestSuccessResponseJSON(t *testing.T) {
	resp := SuccessResponse{
		StatusCode: 200,
		Message:    "ok",
		Data:       []any{"item1"},
		Pagination: &Pagination{Total: 10, Page: 1, PerPage: 5, TotalPages: 2},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded SuccessResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", decoded.StatusCode)
	}
	if decoded.Pagination == nil || decoded.Pagination.Total != 10 {
		t.Error("expected pagination with total=10")
	}
}

func TestErrorResponseJSON(t *testing.T) {
	resp := ErrorResponse{
		StatusCode: 400,
		Message:    "bad request",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ErrorResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", decoded.StatusCode)
	}
	if decoded.Message != "bad request" {
		t.Errorf("Message = %q, want %q", decoded.Message, "bad request")
	}
}

func TestPaginationJSON(t *testing.T) {
	p := Pagination{
		Total:      100,
		Page:       2,
		PerPage:    10,
		TotalPages: 10,
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Pagination
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Total != 100 || decoded.Page != 2 || decoded.PerPage != 10 || decoded.TotalPages != 10 {
		t.Errorf("pagination mismatch: %+v", decoded)
	}
}

func TestClickInfoFields(t *testing.T) {
	ci := ClickInfo{
		UserAgent: "Mozilla/5.0",
		Referrer:  "https://google.com",
	}

	if ci.UserAgent != "Mozilla/5.0" {
		t.Errorf("UserAgent = %q", ci.UserAgent)
	}
	if ci.Referrer != "https://google.com" {
		t.Errorf("Referrer = %q", ci.Referrer)
	}
}
