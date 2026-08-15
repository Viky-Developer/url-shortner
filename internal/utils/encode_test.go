package utils

import "testing"

func TestEncodeDecodeIDRoundtrip(t *testing.T) {
	secret := "test-secret-key"
	tests := []int64{1, 100000, 999999, 1<<62 - 1}

	for _, id := range tests {
		encoded := EncodeID(id, UserIDPrefix, secret)
		if len(encoded) < 5 || encoded[:4] != UserIDPrefix {
			t.Errorf("EncodeID(%d): expected USR_ prefix, got %q", id, encoded)
		}

		decoded, err := DecodeID(encoded, UserIDPrefix, secret)
		if err != nil {
			t.Errorf("DecodeID(%q): %v", encoded, err)
			continue
		}
		if decoded != id {
			t.Errorf("roundtrip: expected %d, got %d", id, decoded)
		}
	}
}

func TestEncodeIDDeterministic(t *testing.T) {
	secret := "test-secret-key"
	a := EncodeID(100000, UserIDPrefix, secret)
	b := EncodeID(100000, UserIDPrefix, secret)
	if a != b {
		t.Fatalf("EncodeID must be deterministic, got %q and %q", a, b)
	}
	if EncodeID(100000, UserIDPrefix, "key-a") == EncodeID(100000, UserIDPrefix, "key-b") {
		t.Fatal("EncodeID must differ for different secret keys")
	}
	if EncodeID(1, UserIDPrefix, secret) == EncodeID(1, OrgIDPrefix, secret) {
		t.Fatal("EncodeID must differ for different prefixes")
	}
}

func TestDecodeWrongSecretYieldsDifferentID(t *testing.T) {
	encoded := EncodeID(100000, UserIDPrefix, "correct-key")
	decoded, err := DecodeID(encoded, UserIDPrefix, "wrong-key")
	if err != nil {
		t.Fatalf("DecodeID with wrong key: %v", err)
	}
	if decoded == 100000 {
		t.Fatalf("expected wrong key to produce a different id, got %d", decoded)
	}
}

func TestDecodeRejectsTamperedPayload(t *testing.T) {
	encoded := EncodeID(100000, UserIDPrefix, "secret")
	tampered := encoded[:4] + "Z" + encoded[5:]
	if tampered == encoded {
		t.Skip("tamper did not change value")
	}
	decoded, err := DecodeID(tampered, UserIDPrefix, "secret")
	if err != nil {
		t.Fatalf("unexpected error decoding tampered payload: %v", err)
	}
	if decoded == 100000 {
		t.Fatalf("tampered payload should decode to a different id, got %d", decoded)
	}
}

func TestDecodeRejectsMissingPrefix(t *testing.T) {
	if _, err := DecodeID("100000", UserIDPrefix, "secret"); err == nil {
		t.Fatal("expected error for missing USR_ prefix")
	}
	if _, err := DecodeID("", UserIDPrefix, "secret"); err == nil {
		t.Fatal("expected error for empty input")
	}
	if _, err := DecodeID("ORG_abc", UserIDPrefix, "secret"); err == nil {
		t.Fatal("expected error for wrong prefix")
	}
}

func TestEncodeBase62(t *testing.T) {
	if got := EncodeBase62(0); got != "0" {
		t.Errorf("EncodeBase62(0) = %q, want \"0\"", got)
	}
	if got := EncodeBase62(62); got != "10" {
		t.Errorf("EncodeBase62(62) = %q, want \"10\"", got)
	}
	if got := EncodeBase62(3844); got != "100" {
		t.Errorf("EncodeBase62(3844) = %q, want \"100\"", got)
	}
}
