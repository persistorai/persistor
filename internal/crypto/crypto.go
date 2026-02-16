package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// Service provides tenant-aware AES-256-GCM encryption and decryption.
type Service struct {
	keys KeyProvider
}

// NewService creates an encryption service backed by the given key provider.
func NewService(keys KeyProvider) *Service {
	return &Service{keys: keys}
}

// Encrypt encrypts plaintext with AES-256-GCM for the given tenant.
// Returns base64-encoded nonce+ciphertext.
func (s *Service) Encrypt(ctx context.Context, tenantID string, plaintext []byte) (string, error) {
	key, err := s.keys.GetKey(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("crypto: get key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: generate nonce: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, plaintext, []byte(tenantID))

	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt decrypts a base64-encoded ciphertext (nonce prepended) for the given tenant.
func (s *Service) Decrypt(ctx context.Context, tenantID, ciphertext string) ([]byte, error) {
	key, err := s.keys.GetKey(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("crypto: get key: %w", err)
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("crypto: base64 decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("crypto: ciphertext too short")
	}

	nonce, sealed := data[:nonceSize], data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, sealed, []byte(tenantID))
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt failed: %w", err)
	}

	return plaintext, nil
}
