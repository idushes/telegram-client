package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gotd/td/session"
)

// ETCDSessionStorage implements telegram.SessionStorage interface using ETCD over HTTP
type ETCDSessionStorage struct {
	endpoint string
	key      string
	client   *http.Client
}

// ETCD API request and response structures
type etcdPutRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type etcdGetRequest struct {
	Key string `json:"key"`
}

type etcdGetResponse struct {
	Kvs []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"kvs"`
}

// ETCDConnectionError представляет ошибку соединения с ETCD
type ETCDConnectionError struct {
	Err error
	Msg string
}

func (e *ETCDConnectionError) Error() string {
	return fmt.Sprintf("ETCD connection error: %s - %v", e.Msg, e.Err)
}

func (e *ETCDConnectionError) Unwrap() error {
	return e.Err
}

// NewETCDSessionStorage creates a new ETCD session storage using HTTP
func NewETCDSessionStorage(endpoint string, phoneHash string) (*ETCDSessionStorage, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("ETCD endpoint cannot be empty")
	}

	// Ensure endpoint format is correct
	endpoint = strings.TrimSuffix(endpoint, "/")

	// Test connection to ETCD server
	client := &http.Client{Timeout: 5 * time.Second}
	healthCheckURL := getBaseURL(endpoint) + "/health"

	resp, err := client.Get(healthCheckURL)
	if err != nil {
		return nil, &ETCDConnectionError{
			Err: err,
			Msg: fmt.Sprintf("failed to connect to ETCD at %s", endpoint),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &ETCDConnectionError{
			Err: fmt.Errorf("status code: %d", resp.StatusCode),
			Msg: fmt.Sprintf("ETCD health check failed: %s", body),
		}
	}

	key := fmt.Sprintf("telegram/sessions/%s", phoneHash)
	log.Printf("Using ETCD storage via HTTP with key: %s", key)

	return &ETCDSessionStorage{
		endpoint: endpoint,
		key:      key,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// getBaseURL extracts the base URL (without API path) from the ETCD endpoint
func getBaseURL(endpoint string) string {
	// Remove any API path like /v3/kv
	parts := strings.Split(endpoint, "/v")
	if len(parts) > 1 {
		return parts[0]
	}
	return endpoint
}

// LoadSession implements telegram.SessionStorage.LoadSession
func (s *ETCDSessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	return s.Load(ctx)
}

// StoreSession implements telegram.SessionStorage.StoreSession
func (s *ETCDSessionStorage) StoreSession(ctx context.Context, data []byte) error {
	return s.Save(ctx, data)
}

// Load implements session.Storage.Load
func (s *ETCDSessionStorage) Load(ctx context.Context) ([]byte, error) {
	// Base64 encode the key for ETCD
	encodedKey := base64.StdEncoding.EncodeToString([]byte(s.key))

	// Create request body
	reqBody := etcdGetRequest{
		Key: encodedKey,
	}

	reqData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ETCD request: %w", err)
	}

	// Create HTTP request
	rangeURL := fmt.Sprintf("%s/v3/kv/range", strings.TrimSuffix(s.endpoint, "/v3/kv"))
	req, err := http.NewRequestWithContext(ctx, "POST", rangeURL, bytes.NewBuffer(reqData))
	if err != nil {
		return nil, &ETCDConnectionError{
			Err: err,
			Msg: "failed to create HTTP request",
		}
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, &ETCDConnectionError{
			Err: err,
			Msg: "failed to send HTTP request to ETCD",
		}
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ETCDConnectionError{
			Err: err,
			Msg: "failed to read ETCD response body",
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &ETCDConnectionError{
			Err: fmt.Errorf("status code: %d", resp.StatusCode),
			Msg: fmt.Sprintf("ETCD returned non-OK status: %s", respBody),
		}
	}

	// Parse response
	var etcdResp etcdGetResponse
	if err := json.Unmarshal(respBody, &etcdResp); err != nil {
		return nil, fmt.Errorf("failed to parse ETCD response: %w", err)
	}

	// Check if key exists
	if len(etcdResp.Kvs) == 0 {
		log.Printf("Session key not found in ETCD: %s", s.key)
		return nil, session.ErrNotFound
	}

	// Decode and return the value
	return base64.StdEncoding.DecodeString(etcdResp.Kvs[0].Value)
}

// Save implements session.Storage.Save
func (s *ETCDSessionStorage) Save(ctx context.Context, data []byte) error {
	// Base64 encode the key and value for ETCD
	encodedKey := base64.StdEncoding.EncodeToString([]byte(s.key))
	encodedValue := base64.StdEncoding.EncodeToString(data)

	// Create request body
	reqBody := etcdPutRequest{
		Key:   encodedKey,
		Value: encodedValue,
	}

	reqData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal ETCD request: %w", err)
	}

	// Create HTTP request
	putURL := fmt.Sprintf("%s/v3/kv/put", strings.TrimSuffix(s.endpoint, "/v3/kv"))
	req, err := http.NewRequestWithContext(ctx, "POST", putURL, bytes.NewBuffer(reqData))
	if err != nil {
		return &ETCDConnectionError{
			Err: err,
			Msg: "failed to create HTTP request",
		}
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		return &ETCDConnectionError{
			Err: err,
			Msg: "failed to send HTTP request to ETCD",
		}
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return &ETCDConnectionError{
			Err: fmt.Errorf("status code: %d", resp.StatusCode),
			Msg: fmt.Sprintf("ETCD returned non-OK status: %s", respBody),
		}
	}

	log.Printf("Successfully saved session to ETCD: %s", s.key)
	return nil
}

// Close closes the HTTP client (no-op)
func (s *ETCDSessionStorage) Close() error {
	// HTTP client doesn't need explicit closing
	return nil
}
