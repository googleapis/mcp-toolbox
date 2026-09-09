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
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
)

// TaskTokenPayload represents the metadata embedded inside an encrypted task ID.
type TaskTokenPayload struct {
	Source    string `json:"src"` // Configured Toolbox source name
	Engine    string `json:"eng"` // Engine identifier
	NativeID  string `json:"nid"` // Driver native ID
	CreatedAt int64  `json:"cat"` // Creation Unix timestamp
}

var (
	// ErrInvalidToken is returned when a task token cannot be decrypted or parsed.
	ErrInvalidToken = errors.New("invalid task token")
	
	// ErrMissingEncryptionKey is returned when the encryption key is missing or invalid.
	ErrMissingEncryptionKey = errors.New("MCP_TOOLBOX_TASK_ENCRYPTION_KEY environment variable is not set or invalid")
)

// GetEncryptionKey retrieves the AES key from environment variables or Secret Manager.
func GetEncryptionKey() ([]byte, error) {
	if key := os.Getenv("MCP_TOOLBOX_TASK_ENCRYPTION_KEY"); key != "" {
		keyBytes := []byte(key)
		if len(keyBytes) == 32 {
			return keyBytes, nil
		}
	}
	return nil, ErrMissingEncryptionKey
}

// EncryptTaskID serializes the payload to JSON, encrypts it using AES-256-GCM, and returns a URL-safe base64 string.
func EncryptTaskID(payload TaskTokenPayload) (string, error) {
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	key, err := GetEncryptionKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

// DecryptTaskID decodes the URL-safe base64 string, decrypts it using AES-256-GCM, and deserializes the JSON payload.
func DecryptTaskID(token string) (*TaskTokenPayload, error) {
	ciphertext, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return nil, ErrInvalidToken
	}

	key, err := GetEncryptionKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, ErrInvalidToken
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrInvalidToken
	}

	var payload TaskTokenPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, ErrInvalidToken
	}

	return &payload, nil
}
