package oauth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"google.golang.org/api/option"
)

// KMSService defines the interface for KMS cryptographic operations.
type KMSService interface {
	Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}

// GCPKMSService implements KMSService using Google Cloud KMS.
type GCPKMSService struct {
	client *kms.KeyManagementClient
	keyURI string
}

// NewGCPKMSService creates a new GCPKMSService client.
func NewGCPKMSService(ctx context.Context, keyURI string, opts ...option.ClientOption) (*GCPKMSService, error) {
	client, err := kms.NewKeyManagementClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("oauth/kms: failed to create KMS client: %w", err)
	}
	return &GCPKMSService{
		client: client,
		keyURI: keyURI,
	}, nil
}

// Encrypt encrypts raw bytes using Google Cloud KMS.
func (s *GCPKMSService) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	req := &kmspb.EncryptRequest{
		Name:      s.keyURI,
		Plaintext: plaintext,
	}
	resp, err := s.client.Encrypt(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("oauth/kms: GCP KMS encrypt failed: %w", err)
	}
	return resp.Ciphertext, nil
}

// Decrypt decrypts ciphertext bytes using Google Cloud KMS.
func (s *GCPKMSService) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	req := &kmspb.DecryptRequest{
		Name:       s.keyURI,
		Ciphertext: ciphertext,
	}
	resp, err := s.client.Decrypt(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("oauth/kms: GCP KMS decrypt failed: %w", err)
	}
	return resp.Plaintext, nil
}

// Close closes the underlying KMS client connection.
func (s *GCPKMSService) Close() error {
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

// MockKMSService implements KMSService using a local key for unit testing.
type MockKMSService struct {
	key [32]byte
}

// NewMockKMSService creates a new MockKMSService with a static 32-byte passphrase key.
func NewMockKMSService(passphrase string) *MockKMSService {
	var key [32]byte
	copy(key[:], passphrase)
	return &MockKMSService{key: key}
}

// Encrypt simulates KMS encryption using AES-GCM and the local static key.
func (s *MockKMSService) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return nil, fmt.Errorf("oauth/mock_kms: failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("oauth/mock_kms: failed to create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("oauth/mock_kms: failed to generate nonce: %w", err)
	}
	// The ciphertext structure here is nonce + actual ciphertext
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt simulates KMS decryption using AES-GCM and the local static key.
func (s *MockKMSService) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return nil, fmt.Errorf("oauth/mock_kms: failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("oauth/mock_kms: failed to create GCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("oauth/mock_kms: ciphertext too short")
	}
	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("oauth/mock_kms: failed to decrypt: %w", err)
	}
	return plaintext, nil
}

// EncryptedPayload represents the structured JSON containing the wrapped DEK,
// IV/nonce, and ciphertext of the encrypted token.
type EncryptedPayload struct {
	WrappedDEK []byte `json:"wrapped_dek"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// EncryptToken executes envelope encryption on a token string.
// It generates a random 32-byte DEK, encrypts the token using AES-256-GCM,
// wraps the DEK using the provided KMSService, serializes the payload to JSON,
// and returns the base64-encoded string.
func EncryptToken(ctx context.Context, kmsService KMSService, token string) (string, error) {
	if kmsService == nil {
		return "", errors.New("oauth/encryption: KMS service is required")
	}

	// 1. Generate a random 32-byte Data Encryption Key (DEK)
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return "", fmt.Errorf("oauth/encryption: failed to generate DEK: %w", err)
	}

	// 2. Encrypt the token with the DEK using AES-256-GCM
	block, err := aes.NewCipher(dek)
	if err != nil {
		return "", fmt.Errorf("oauth/encryption: failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("oauth/encryption: failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("oauth/encryption: failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(token), nil)

	// 3. Wrap the DEK using KMSService
	wrappedDEK, err := kmsService.Encrypt(ctx, dek)
	if err != nil {
		return "", fmt.Errorf("oauth/encryption: failed to wrap DEK: %w", err)
	}

	// 4. Construct and serialize the encrypted payload
	payload := EncryptedPayload{
		WrappedDEK: wrappedDEK,
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("oauth/encryption: failed to marshal payload: %w", err)
	}

	// 5. Base64 encode the serialized JSON
	encoded := base64.StdEncoding.EncodeToString(jsonBytes)
	return encoded, nil
}

// DecryptToken executes envelope decryption on a base64-encoded encrypted token payload.
// It base64 decodes the payload, unmarshals the JSON, unwraps the DEK using the provided KMSService,
// and decrypts the ciphertext using AES-256-GCM to retrieve the token string.
func DecryptToken(ctx context.Context, kmsService KMSService, encryptedPayloadB64 string) (string, error) {
	if kmsService == nil {
		return "", errors.New("oauth/encryption: KMS service is required")
	}

	// 1. Base64 decode the payload
	jsonBytes, err := base64.StdEncoding.DecodeString(encryptedPayloadB64)
	if err != nil {
		return "", fmt.Errorf("oauth/encryption: failed to decode base64: %w", err)
	}

	// 2. Unmarshal the JSON payload
	var payload EncryptedPayload
	if err = json.Unmarshal(jsonBytes, &payload); err != nil {
		return "", fmt.Errorf("oauth/encryption: failed to unmarshal payload: %w", err)
	}

	// 3. Unwrap the DEK using KMSService
	dek, err := kmsService.Decrypt(ctx, payload.WrappedDEK)
	if err != nil {
		return "", fmt.Errorf("oauth/encryption: failed to unwrap DEK: %w", err)
	}

	if len(dek) != 32 {
		return "", fmt.Errorf("oauth/encryption: invalid DEK length: got %d, expected 32", len(dek))
	}

	// 4. Decrypt the ciphertext with the DEK using AES-256-GCM
	block, err := aes.NewCipher(dek)
	if err != nil {
		return "", fmt.Errorf("oauth/encryption: failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("oauth/encryption: failed to create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, payload.Nonce, payload.Ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("oauth/encryption: failed to decrypt ciphertext: %w", err)
	}

	return string(plaintext), nil
}
