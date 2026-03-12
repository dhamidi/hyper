package hyper

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestBearerToken(t *testing.T) {
	cred := BearerToken("tok123")
	if cred.Scheme != "bearer" {
		t.Errorf("Scheme = %q, want %q", cred.Scheme, "bearer")
	}
	if cred.Value != "tok123" {
		t.Errorf("Value = %q, want %q", cred.Value, "tok123")
	}
}

func TestAPIKeyHeader(t *testing.T) {
	cred := APIKeyHeader("X-API-Key", "secret")
	if cred.Scheme != "apikey-header" {
		t.Errorf("Scheme = %q, want %q", cred.Scheme, "apikey-header")
	}
	if cred.Header != "X-API-Key" {
		t.Errorf("Header = %q, want %q", cred.Header, "X-API-Key")
	}
	if cred.Value != "secret" {
		t.Errorf("Value = %q, want %q", cred.Value, "secret")
	}
}

func TestAPIKeyQuery(t *testing.T) {
	cred := APIKeyQuery("api_key", "secret")
	if cred.Scheme != "apikey-query" {
		t.Errorf("Scheme = %q, want %q", cred.Scheme, "apikey-query")
	}
	if cred.Param != "api_key" {
		t.Errorf("Param = %q, want %q", cred.Param, "api_key")
	}
	if cred.Value != "secret" {
		t.Errorf("Value = %q, want %q", cred.Value, "secret")
	}
}

func TestFileCredentialStore_StoreAndCredential(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "creds.json")
	store := &FileCredentialStore{Path: tmp}
	ctx := context.Background()
	u, _ := url.Parse("https://api.example.com/v1")

	cred := BearerToken("mytoken")
	if err := store.Store(ctx, u, cred); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, err := store.Credential(ctx, u)
	if err != nil {
		t.Fatalf("Credential: %v", err)
	}
	if got.Scheme != cred.Scheme || got.Value != cred.Value {
		t.Errorf("round-trip: got %+v, want %+v", got, cred)
	}
}

func TestFileCredentialStore_Delete(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "creds.json")
	store := &FileCredentialStore{Path: tmp}
	ctx := context.Background()
	u, _ := url.Parse("https://api.example.com/v1")

	_ = store.Store(ctx, u, BearerToken("mytoken"))

	if err := store.Delete(ctx, u); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := store.Credential(ctx, u)
	if err != nil {
		t.Fatalf("Credential after delete: %v", err)
	}
	if got.Value != "" {
		t.Errorf("expected empty credential after delete, got %+v", got)
	}
}

func TestFileCredentialStore_DefaultPath(t *testing.T) {
	store := &FileCredentialStore{}
	p := store.path()
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "hyper", "credentials.json")
	if p != want {
		t.Errorf("default path = %q, want %q", p, want)
	}
}

func TestStaticCredentialStore(t *testing.T) {
	cred := BearerToken("static-tok")
	store := &staticCredentialStore{cred: cred}
	ctx := context.Background()

	u1, _ := url.Parse("https://a.example.com")
	u2, _ := url.Parse("https://b.example.com")

	got1, err := store.Credential(ctx, u1)
	if err != nil {
		t.Fatalf("Credential u1: %v", err)
	}
	got2, err := store.Credential(ctx, u2)
	if err != nil {
		t.Fatalf("Credential u2: %v", err)
	}

	if got1 != cred || got2 != cred {
		t.Errorf("staticCredentialStore should always return same credential")
	}

	// Store and Delete are no-ops
	if err := store.Store(ctx, u1, APIKeyHeader("X", "y")); err != nil {
		t.Errorf("Store should be no-op, got %v", err)
	}
	if err := store.Delete(ctx, u1); err != nil {
		t.Errorf("Delete should be no-op, got %v", err)
	}
}
