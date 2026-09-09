// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package taskcrypto

import (
	"os"
	"testing"
	"time"
)

func setupTestKey(t *testing.T) {
	os.Setenv("MCP_TOOLBOX_TASK_ENCRYPTION_KEY", "12345678901234567890123456789012")
	t.Cleanup(func() {
		os.Unsetenv("MCP_TOOLBOX_TASK_ENCRYPTION_KEY")
	})
}

func TestEncryptDecryptTaskID(t *testing.T) {
	setupTestKey(t)
	payload := TaskTokenPayload{
		Source:    "bigquery-prod",
		Engine:    "bigquery",
		NativeID:  "bqux_job_12345678_abcdef",
		CreatedAt: time.Now().Unix(),
	}

	token, err := EncryptTaskID(payload)
	if err != nil {
		t.Fatalf("EncryptTaskID failed: %v", err)
	}

	decrypted, err := DecryptTaskID(token)
	if err != nil {
		t.Fatalf("DecryptTaskID failed: %v", err)
	}

	if decrypted.Source != payload.Source ||
		decrypted.Engine != payload.Engine ||
		decrypted.NativeID != payload.NativeID ||
		decrypted.CreatedAt != payload.CreatedAt {
		t.Errorf("Decrypted payload %v does not match original %v", decrypted, payload)
	}
}

func TestDecryptTaskID_InvalidBase64(t *testing.T) {
	setupTestKey(t)
	_, err := DecryptTaskID("invalid_base64!")
	if err != ErrInvalidToken {
		t.Errorf("Expected ErrInvalidToken, got: %v", err)
	}
}

func TestDecryptTaskID_TamperedToken(t *testing.T) {
	setupTestKey(t)
	payload := TaskTokenPayload{
		Source:    "bigquery-prod",
		Engine:    "bigquery",
		NativeID:  "bqux_job_12345678_abcdef",
		CreatedAt: time.Now().Unix(),
	}

	token, err := EncryptTaskID(payload)
	if err != nil {
		t.Fatalf("EncryptTaskID failed: %v", err)
	}

	// Tamper with the base64 token
	tamperedToken := token[:len(token)-5] + "AAAAA"
	_, err = DecryptTaskID(tamperedToken)
	if err != ErrInvalidToken {
		t.Errorf("Expected ErrInvalidToken for tampered token, got: %v", err)
	}
}

func TestGetEncryptionKey(t *testing.T) {
	// Test missing
	os.Unsetenv("MCP_TOOLBOX_TASK_ENCRYPTION_KEY")
	_, err := GetEncryptionKey()
	if err != ErrMissingEncryptionKey {
		t.Errorf("Expected ErrMissingEncryptionKey, got %v", err)
	}

	// Test invalid length
	os.Setenv("MCP_TOOLBOX_TASK_ENCRYPTION_KEY", "too-short")
	_, err = GetEncryptionKey()
	if err != ErrMissingEncryptionKey {
		t.Errorf("Expected ErrMissingEncryptionKey, got %v", err)
	}

	// Test valid custom
	customKey := "12345678901234567890123456789012"
	os.Setenv("MCP_TOOLBOX_TASK_ENCRYPTION_KEY", customKey)
	defer os.Unsetenv("MCP_TOOLBOX_TASK_ENCRYPTION_KEY")

	key, err := GetEncryptionKey()
	if err != nil {
		t.Fatalf("Expected valid key, got error %v", err)
	}
	if string(key) != customKey {
		t.Errorf("Expected custom key %v, got %v", customKey, string(key))
	}
}
