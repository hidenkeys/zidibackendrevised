package fulfilment

import (
	"bytes"
	"regexp"
	"testing"
)

func TestCodeManagerProtectsAndVerifiesCodes(t *testing.T) {
	manager, err := NewCodeManager(bytes.Repeat([]byte{0x2a}, 32))
	if err != nil {
		t.Fatal(err)
	}
	protected, err := manager.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9]{6}$`).MatchString(protected.Plaintext) {
		t.Fatalf("generated code is not six digits: %q", protected.Plaintext)
	}
	if bytes.Contains(protected.Ciphertext, []byte(protected.Plaintext)) {
		t.Fatal("ciphertext contains the plaintext verification code")
	}
	if !manager.Verify(protected.Plaintext, protected.Hash) || manager.Verify("999999", protected.Hash) {
		t.Fatal("verification hash did not accept only the original code")
	}
	revealed, err := manager.Reveal(protected.Ciphertext)
	if err != nil || revealed != protected.Plaintext {
		t.Fatalf("reveal: code=%q err=%v", revealed, err)
	}
}

func TestCodeManagerRejectsWeakSecretAndTamperedCiphertext(t *testing.T) {
	if _, err := NewCodeManager([]byte("too-short")); err == nil {
		t.Fatal("expected weak secret to be rejected")
	}
	manager, err := NewCodeManager(bytes.Repeat([]byte{0x3b}, 32))
	if err != nil {
		t.Fatal(err)
	}
	protected, err := manager.Generate()
	if err != nil {
		t.Fatal(err)
	}
	protected.Ciphertext[len(protected.Ciphertext)-1] ^= 0xff
	if _, err := manager.Reveal(protected.Ciphertext); err == nil {
		t.Fatal("expected authenticated decryption to reject tampering")
	}
}
