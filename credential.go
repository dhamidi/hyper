package hyper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

// Credential represents an authentication credential with its placement strategy.
type Credential struct {
	// Scheme determines how the credential is attached to requests.
	// Well-known values: "bearer", "apikey-header", "apikey-query".
	Scheme string `json:"scheme"`

	// Value is the credential value (token, key, etc.).
	Value string `json:"value"`

	// Header is the header name for "apikey-header" scheme.
	Header string `json:"header,omitempty"`

	// Param is the query parameter name for "apikey-query" scheme.
	Param string `json:"param,omitempty"`
}

// BearerToken returns a Credential that attaches as "Authorization: Bearer <token>".
func BearerToken(token string) Credential {
	return Credential{Scheme: "bearer", Value: token}
}

// APIKeyHeader returns a Credential that attaches the key in the named header.
func APIKeyHeader(header, key string) Credential {
	return Credential{Scheme: "apikey-header", Header: header, Value: key}
}

// APIKeyQuery returns a Credential that appends the key as a query parameter.
func APIKeyQuery(param, key string) Credential {
	return Credential{Scheme: "apikey-query", Param: param, Value: key}
}

// CredentialStore retrieves and persists authentication credentials.
type CredentialStore interface {
	// Credential returns the current credential for the given base URL.
	Credential(ctx context.Context, baseURL *url.URL) (Credential, error)

	// Store persists a credential for the given base URL.
	Store(ctx context.Context, baseURL *url.URL, cred Credential) error

	// Delete removes the stored credential for the given base URL.
	Delete(ctx context.Context, baseURL *url.URL) error
}

// staticCredentialStore always returns the same credential.
type staticCredentialStore struct {
	cred Credential
}

func (s *staticCredentialStore) Credential(_ context.Context, _ *url.URL) (Credential, error) {
	return s.cred, nil
}

func (s *staticCredentialStore) Store(_ context.Context, _ *url.URL, _ Credential) error {
	return nil
}

func (s *staticCredentialStore) Delete(_ context.Context, _ *url.URL) error {
	return nil
}

// defaultCredentialPath returns ~/.config/hyper/credentials.json.
func defaultCredentialPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", "hyper", "credentials.json")
}

// FileCredentialStore persists credentials as a JSON file keyed by host.
type FileCredentialStore struct {
	// Path is the file path. Defaults to ~/.config/hyper/credentials.json when empty.
	Path string

	mu sync.Mutex
}

func (f *FileCredentialStore) path() string {
	if f.Path != "" {
		return f.Path
	}
	return defaultCredentialPath()
}

func (f *FileCredentialStore) load() (map[string]Credential, error) {
	data, err := os.ReadFile(f.path())
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]Credential), nil
		}
		return nil, fmt.Errorf("hyper: read credentials file: %w", err)
	}
	var creds map[string]Credential
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("hyper: decode credentials file: %w", err)
	}
	return creds, nil
}

func (f *FileCredentialStore) save(creds map[string]Credential) error {
	p := f.path()
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return fmt.Errorf("hyper: create credentials directory: %w", err)
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("hyper: encode credentials: %w", err)
	}
	if err := os.WriteFile(p, data, 0600); err != nil {
		return fmt.Errorf("hyper: write credentials file: %w", err)
	}
	return nil
}

// Credential returns the stored credential for the given base URL's host.
func (f *FileCredentialStore) Credential(_ context.Context, baseURL *url.URL) (Credential, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	creds, err := f.load()
	if err != nil {
		return Credential{}, err
	}
	return creds[baseURL.Host], nil
}

// Store persists a credential keyed by the base URL's host.
func (f *FileCredentialStore) Store(_ context.Context, baseURL *url.URL, cred Credential) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	creds, err := f.load()
	if err != nil {
		return err
	}
	creds[baseURL.Host] = cred
	return f.save(creds)
}

// Delete removes the credential for the given base URL's host.
func (f *FileCredentialStore) Delete(_ context.Context, baseURL *url.URL) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	creds, err := f.load()
	if err != nil {
		return err
	}
	delete(creds, baseURL.Host)
	return f.save(creds)
}
