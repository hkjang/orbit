package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

type Vault struct{ master []byte }

func NewVault(master []byte) (*Vault, error) {
	if len(master) != 32 {
		return nil, errors.New("master key must be 32 bytes")
	}
	return &Vault{master: append([]byte(nil), master...)}, nil
}

func (v *Vault) NewDataKey() ([]byte, error) {
	key := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, key)
	return key, err
}

func (v *Vault) WrapKey(key []byte) (string, error) {
	return encrypt(v.master, key, []byte("orbit:user-data-key:v1"))
}
func (v *Vault) UnwrapKey(value string) ([]byte, error) {
	return decrypt(v.master, value, []byte("orbit:user-data-key:v1"))
}
func (v *Vault) Encrypt(key []byte, plaintext string, context string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	return encrypt(key, []byte(plaintext), []byte(context))
}
func (v *Vault) Decrypt(key []byte, value string, context string) (string, error) {
	if value == "" {
		return "", nil
	}
	plain, err := decrypt(key, value, []byte(context))
	return string(plain), err
}
func (v *Vault) EncryptSystem(plaintext, context string) (string, error) {
	return v.Encrypt(v.master, plaintext, "orbit:system:"+context)
}
func (v *Vault) DecryptSystem(value, context string) (string, error) {
	return v.Decrypt(v.master, value, "orbit:system:"+context)
}

func encrypt(key, plaintext, aad []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, aad)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func decrypt(key []byte, value string, aad []byte) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, errors.New("invalid encrypted value")
	}
	return gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], aad)
}

func SHA256(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
