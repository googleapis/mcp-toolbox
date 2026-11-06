package tasks

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
)

var (
	ErrInvalidKey       = errors.New("invalid key size: must be 16, 24, or 32 bytes")
	ErrDecryptionFailed = errors.New("decryption failed")
)

// CryptoService provides AES-GCM encryption and decryption for task payloads.
type CryptoService struct {
	key []byte
}

// NewCryptoService initializes a new CryptoService with the provided symmetric key.
func NewCryptoService(key []byte) (*CryptoService, error) {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, ErrInvalidKey
	}
	return &CryptoService{key: key}, nil
}

// EncryptPayload serializes and encrypts the given payload into a URL-safe base64 string.
func (s *CryptoService) EncryptPayload(payload *EncryptedTaskPayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(s.key)
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

	ciphertext := aesGCM.Seal(nonce, nonce, data, nil)
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

// DecryptPayload decrypts a URL-safe base64 string back into an EncryptedTaskPayload.
func (s *CryptoService) DecryptPayload(token string) (*EncryptedTaskPayload, error) {
	ciphertext, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, ErrDecryptionFailed
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	var payload EncryptedTaskPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, err
	}

	return &payload, nil
}
