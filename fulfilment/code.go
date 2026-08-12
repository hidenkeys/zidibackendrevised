package fulfilment

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
)

const verificationCodeSpace uint32 = 1_000_000

type ProtectedCode struct {
	Plaintext  string
	Hash       []byte
	Ciphertext []byte
}

type CodeManager struct {
	encryptionKey [32]byte
	hashingKey    [32]byte
}

func NewCodeManager(secret []byte) (*CodeManager, error) {
	if len(secret) < 32 {
		return nil, errors.New("fulfilment code secret must contain at least 32 bytes")
	}
	manager := &CodeManager{
		encryptionKey: sha256.Sum256(append([]byte("zidi:fulfilment:encryption:"), secret...)),
		hashingKey:    sha256.Sum256(append([]byte("zidi:fulfilment:verification:"), secret...)),
	}
	return manager, nil
}

func (m *CodeManager) Generate() (*ProtectedCode, error) {
	random, err := rand.Int(rand.Reader, big.NewInt(int64(verificationCodeSpace)))
	if err != nil {
		return nil, fmt.Errorf("generate fulfilment verification code: %w", err)
	}
	code := fmt.Sprintf("%06d", random.Int64())
	ciphertext, err := m.encrypt([]byte(code))
	if err != nil {
		return nil, err
	}
	return &ProtectedCode{Plaintext: code, Hash: m.Hash(code), Ciphertext: ciphertext}, nil
}

func (m *CodeManager) Hash(code string) []byte {
	mac := hmac.New(sha256.New, m.hashingKey[:])
	_, _ = mac.Write([]byte(code))
	return mac.Sum(nil)
}

func (m *CodeManager) Verify(code string, expected []byte) bool {
	return hmac.Equal(m.Hash(code), expected)
}

func (m *CodeManager) Reveal(ciphertext []byte) (string, error) {
	block, err := aes.NewCipher(m.encryptionKey[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", errors.New("invalid fulfilment code ciphertext")
	}
	nonce, encrypted := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", errors.New("decrypt fulfilment code")
	}
	return string(plaintext), nil
}

func (m *CodeManager) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(m.encryptionKey[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate fulfilment code nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}
