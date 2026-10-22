package tasks

import (
	"crypto/rand"
	"testing"
	"time"
)

func TestCryptoService_EncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	svc, err := NewCryptoService(key)
	if err != nil {
		t.Fatalf("failed to create crypto service: %v", err)
	}

	payload := &EncryptedTaskPayload{
		SourceType: "postgres",
		ProjectID:  "my-project",
		NativeID:   "job-123",
		CreatedAt:  time.Now().Unix(),
	}

	token, err := svc.EncryptPayload(payload)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	decrypted, err := svc.DecryptPayload(token)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if decrypted.SourceType != payload.SourceType {
		t.Errorf("expected SourceType %q, got %q", payload.SourceType, decrypted.SourceType)
	}
	if decrypted.ProjectID != payload.ProjectID {
		t.Errorf("expected ProjectID %q, got %q", payload.ProjectID, decrypted.ProjectID)
	}
	if decrypted.NativeID != payload.NativeID {
		t.Errorf("expected NativeID %q, got %q", payload.NativeID, decrypted.NativeID)
	}
	if decrypted.CreatedAt != payload.CreatedAt {
		t.Errorf("expected CreatedAt %d, got %d", payload.CreatedAt, decrypted.CreatedAt)
	}
}

func TestCryptoService_InvalidKeySize(t *testing.T) {
	_, err := NewCryptoService(make([]byte, 10))
	if err != ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey, got %v", err)
	}
}

func TestCryptoService_InvalidToken(t *testing.T) {
	key := make([]byte, 32)
	svc, _ := NewCryptoService(key)

	_, err := svc.DecryptPayload("invalid-base64-!@#")
	if err == nil {
		t.Error("expected error decrypting invalid base64")
	}

	_, err = svc.DecryptPayload("YWJjZA==") // valid base64 but invalid ciphertext
	if err == nil {
		t.Error("expected error decrypting invalid ciphertext")
	}
}
