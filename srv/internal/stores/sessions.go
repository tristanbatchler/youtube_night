package stores

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Single instance of the session store
var (
	globalSessionStore *SessionStore
	once               sync.Once
)

type SessionData struct {
	UserId    int32
	GangId    int32
	GangName  string
	Name      string
	Avatar    string
	CreatedAt int64
	Expiry    int64
}

type SessionStore struct {
	token []byte
	// Optional: add a logger
	logger *log.Logger
}

func NewSessionStore(token []byte) *SessionStore {
	store := &SessionStore{
		token: token,
	}

	// Set this as the global session store
	once.Do(func() {
		globalSessionStore = store
	})

	return store
}

// GetSessionStore returns the global session store instance
func GetSessionStore() *SessionStore {
	if globalSessionStore == nil {
		panic("Session store not initialized")
	}
	return globalSessionStore
}

// CreateToken generates a new session token containing the provided data
func (s *SessionStore) CreateToken(data *SessionData) (string, error) {
	// Add timestamps
	now := time.Now().Unix()
	data.CreatedAt = now
	data.Expiry = now + int64(24*time.Hour.Seconds())

	// Serialize the data
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("error marshalling session data: %w", err)
	}

	// Generate a random ID
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("error generating random bytes: %w", err)
	}
	randomID := base64.URLEncoding.EncodeToString(randomBytes)

	// Create the payload
	payload := fmt.Sprintf("%s.%s", base64.URLEncoding.EncodeToString(jsonData), randomID)

	// Sign the payload
	h := hmac.New(sha256.New, s.token)
	h.Write([]byte(payload))
	signature := base64.URLEncoding.EncodeToString(h.Sum(nil))

	// Combine payload and signature
	return fmt.Sprintf("%s.%s", payload, signature), nil
}

// ValidateToken verifies a token and returns the session data if valid
func (s *SessionStore) ValidateToken(token string) (*SessionData, bool, error) {
	// Split token into payload and signature
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false, errors.New("invalid token format")
	}

	encodedData := parts[0]
	randomID := parts[1]
	signature := parts[2]

	// Verify signature
	h := hmac.New(sha256.New, s.token)
	h.Write([]byte(fmt.Sprintf("%s.%s", encodedData, randomID)))
	expectedSig := base64.URLEncoding.EncodeToString(h.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return nil, false, errors.New("invalid token signature")
	}

	// Decode data
	jsonData, err := base64.URLEncoding.DecodeString(encodedData)
	if err != nil {
		return nil, false, fmt.Errorf("error decoding token data: %w", err)
	}

	// Unmarshal data
	var sessionData SessionData
	if err := json.Unmarshal(jsonData, &sessionData); err != nil {
		return nil, false, fmt.Errorf("error unmarshalling token data: %w", err)
	}

	// Check expiration
	if time.Now().Unix() > sessionData.Expiry {
		return nil, false, errors.New("token expired")
	}

	return &sessionData, true, nil
}

// ShouldRotateToken checks if a token should be rotated based on age
func (s *SessionStore) ShouldRotateToken(token string) bool {
	// Parse the token to get its creation time
	data, valid, err := s.ValidateToken(token)
	if err != nil || !valid {
		return false
	}

	// Rotate if older than 30 minutes
	return time.Now().Unix()-data.CreatedAt > 1800
}

// RotateToken creates a new token with the same data but new timestamps
func (s *SessionStore) RotateToken(oldToken string, data *SessionData) (string, error) {
	return s.CreateToken(data)
}
