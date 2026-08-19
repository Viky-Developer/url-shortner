package contextutil

import (
	"context"
	"testing"
)

func TestUserIDKeyIsNonEmpty(t *testing.T) {
	if string(UserIDKey) == "" {
		t.Error("UserIDKey should not be empty")
	}
}

func TestUserIDKeyContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	expected := int64(42)

	ctx = context.WithValue(ctx, UserIDKey, expected)

	got, ok := ctx.Value(UserIDKey).(int64)
	if !ok {
		t.Fatal("expected int64 type assertion to succeed")
	}
	if got != expected {
		t.Errorf("expected %d, got %d", expected, got)
	}
}

func TestContextKeyIsStringType(t *testing.T) {
	var _ ContextKey = "test"
	_ = UserIDKey
}
