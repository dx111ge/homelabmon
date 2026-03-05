package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// deriveKey loads or creates a 32-byte encryption key from a keyfile in the data directory.
func deriveKey(dataDir string) ([]byte, error) {
	keyPath := filepath.Join(dataDir, "secret.key")
	data, err := os.ReadFile(keyPath)
	if err == nil && len(data) == 32 {
		return data, nil
	}
	// Generate new key
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}
	return key, nil
}

func encrypt(key []byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func decrypt(key []byte, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// SetSecret stores an encrypted secret.
func (s *Store) SetSecret(ctx context.Context, name, value string) error {
	enc, err := encrypt(s.encKey, []byte(value))
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO secrets (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, name, enc)
	return err
}

// GetSecret retrieves and decrypts a secret.
func (s *Store) GetSecret(ctx context.Context, name string) (string, error) {
	var enc []byte
	err := s.db.QueryRowContext(ctx, "SELECT value FROM secrets WHERE key = ?", name).Scan(&enc)
	if err != nil {
		return "", err
	}
	plain, err := decrypt(s.encKey, enc)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}

// DeleteSecret removes a secret.
func (s *Store) DeleteSecret(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM secrets WHERE key = ?", name)
	return err
}

// secretKeyID returns a deterministic key name for an integration credential.
func SecretKeyID(integrationID, field string) string {
	h := sha256.Sum256([]byte(integrationID + ":" + field))
	return fmt.Sprintf("int:%s:%x", field, h[:4])
}
