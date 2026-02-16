package crypto_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/persistorai/persistor/internal/crypto"
)

const testKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestEncryptDecryptRoundtrip(t *testing.T) {
	provider, err := crypto.NewStaticProvider(testKeyHex)
	if err != nil {
		t.Fatalf("new static provider: %v", err)
	}

	svc := crypto.NewService(provider)
	ctx := context.Background()
	plaintext := []byte("hello, persistor")

	encrypted, err := svc.Encrypt(ctx, "tenant-1", plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if encrypted == string(plaintext) {
		t.Fatal("ciphertext should differ from plaintext")
	}

	decrypted, err := svc.Decrypt(ctx, "tenant-1", encrypted)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	provider, _ := crypto.NewStaticProvider(testKeyHex)
	svc := crypto.NewService(provider)
	ctx := context.Background()

	a, _ := svc.Encrypt(ctx, "t", []byte("same"))
	b, _ := svc.Encrypt(ctx, "t", []byte("same"))

	if a == b {
		t.Fatal("two encryptions of same plaintext should differ (random nonce)")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	provider1, _ := crypto.NewStaticProvider(testKeyHex)
	provider2, _ := crypto.NewStaticProvider("abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")

	ctx := context.Background()

	encrypted, err := crypto.NewService(provider1).Encrypt(ctx, "t", []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	_, err = crypto.NewService(provider2).Decrypt(ctx, "t", encrypted)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestDecryptCorruptedCiphertext(t *testing.T) {
	provider, _ := crypto.NewStaticProvider(testKeyHex)
	svc := crypto.NewService(provider)
	ctx := context.Background()

	encrypted, _ := svc.Encrypt(ctx, "t", []byte("data"))

	raw, _ := base64.StdEncoding.DecodeString(encrypted)
	raw[len(raw)-1] ^= 0xff
	corrupted := base64.StdEncoding.EncodeToString(raw)

	_, err := svc.Decrypt(ctx, "t", corrupted)
	if err == nil {
		t.Fatal("expected error decrypting corrupted ciphertext")
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	provider, _ := crypto.NewStaticProvider(testKeyHex)
	svc := crypto.NewService(provider)

	_, err := svc.Decrypt(context.Background(), "t", "not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestDecryptTooShort(t *testing.T) {
	provider, _ := crypto.NewStaticProvider(testKeyHex)
	svc := crypto.NewService(provider)

	short := base64.StdEncoding.EncodeToString([]byte("tiny"))

	_, err := svc.Decrypt(context.Background(), "t", short)
	if err == nil {
		t.Fatal("expected error for too-short ciphertext")
	}
}

func TestStaticProviderReturnsKey(t *testing.T) {
	provider, err := crypto.NewStaticProvider(testKeyHex)
	if err != nil {
		t.Fatalf("new static provider: %v", err)
	}

	key, err := provider.GetKey(context.Background(), "any-tenant")
	if err != nil {
		t.Fatalf("get key: %v", err)
	}

	if len(key) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(key))
	}
}

func TestStaticProviderBadHex(t *testing.T) {
	_, err := crypto.NewStaticProvider("not-hex")
	if err == nil {
		t.Fatal("expected error for bad hex")
	}
}

func TestStaticProviderWrongLength(t *testing.T) {
	_, err := crypto.NewStaticProvider("0123456789abcdef")
	if err == nil {
		t.Fatal("expected error for wrong key length")
	}
}

func TestEncryptEmptyPlaintext(t *testing.T) {
	provider, _ := crypto.NewStaticProvider(testKeyHex)
	svc := crypto.NewService(provider)
	ctx := context.Background()

	encrypted, err := svc.Encrypt(ctx, "t", []byte{})
	if err != nil {
		t.Fatalf("encrypt empty: %v", err)
	}

	decrypted, err := svc.Decrypt(ctx, "t", encrypted)
	if err != nil {
		t.Fatalf("decrypt empty: %v", err)
	}

	if len(decrypted) != 0 {
		t.Fatalf("expected empty, got %d bytes", len(decrypted))
	}
}

func TestStaticProviderRejectsMultiTenant(t *testing.T) {
	provider, _ := crypto.NewStaticProvider(testKeyHex)
	ctx := context.Background()

	_, err := provider.GetKey(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("first tenant should succeed: %v", err)
	}

	_, err = provider.GetKey(ctx, "tenant-b")
	if err == nil {
		t.Fatal("expected error for second tenant ID on static provider")
	}
}

func TestCiphertextIsBase64(t *testing.T) {
	provider, _ := crypto.NewStaticProvider(testKeyHex)
	svc := crypto.NewService(provider)

	encrypted, _ := svc.Encrypt(context.Background(), "t", []byte("test"))

	if strings.ContainsAny(encrypted, " \t\n") {
		t.Fatal("ciphertext should be clean base64")
	}

	_, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Fatalf("ciphertext is not valid base64: %v", err)
	}
}
