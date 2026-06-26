package oauth

import (
	"context"
	"testing"
)

func TestEnvelopeEncryptionRoundTrip(t *testing.T) {
	ctx := context.Background()
	mockKMS := NewMockKMSService("my-super-secret-key-32-bytes-long!")

	originalToken := "google-oauth-access-token-1234567890"

	// Encrypt the token
	encryptedPayloadB64, err := EncryptToken(ctx, mockKMS, originalToken)
	if err != nil {
		t.Fatalf("failed to encrypt token: %v", err)
	}

	if encryptedPayloadB64 == "" {
		t.Fatal("encrypted payload base64 is empty")
	}

	// Decrypt the token
	decryptedToken, err := DecryptToken(ctx, mockKMS, encryptedPayloadB64)
	if err != nil {
		t.Fatalf("failed to decrypt token: %v", err)
	}

	if decryptedToken != originalToken {
		t.Errorf("decrypted token does not match original; got %q, expected %q", decryptedToken, originalToken)
	}
}

func TestMockKMSServiceRoundTrip(t *testing.T) {
	ctx := context.Background()
	mockKMS := NewMockKMSService("shortkey") // passphrase gets copied with zero padding

	plaintext := []byte("hello world")

	ciphertext, err := mockKMS.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("mock KMS encrypt failed: %v", err)
	}

	decrypted, err := mockKMS.Decrypt(ctx, ciphertext)
	if err != nil {
		t.Fatalf("mock KMS decrypt failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted text does not match plaintext; got %q, expected %q", decrypted, plaintext)
	}
}
