package secure

import (
	"strings"
	"testing"
)

func TestVaultRoundTripAndContextBinding(t *testing.T) {
	vault, err := NewVault([]byte(strings.Repeat("m", 32)))
	if err != nil {
		t.Fatal(err)
	}
	dataKey, err := vault.NewDataKey()
	if err != nil {
		t.Fatal(err)
	}
	wrapped, err := vault.WrapKey(dataKey)
	if err != nil {
		t.Fatal(err)
	}
	unwrapped, err := vault.UnwrapKey(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := vault.Encrypt(unwrapped, "private memory", "memory:one")
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := vault.Decrypt(unwrapped, ciphertext, "memory:one")
	if err != nil || plaintext != "private memory" {
		t.Fatalf("round trip failed: %q %v", plaintext, err)
	}
	if _, err := vault.Decrypt(unwrapped, ciphertext, "memory:other"); err == nil {
		t.Fatal("ciphertext must be bound to its field context")
	}
}

func TestCiphertextUsesRandomNonce(t *testing.T) {
	vault, _ := NewVault([]byte(strings.Repeat("m", 32)))
	key := []byte(strings.Repeat("d", 32))
	one, _ := vault.Encrypt(key, "same", "context")
	two, _ := vault.Encrypt(key, "same", "context")
	if one == two {
		t.Fatal("expected randomized ciphertext")
	}
}
