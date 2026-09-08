package codex

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadCredentials(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "nested", "auth.json")
	want := credentials{AccessToken: "access", RefreshToken: "refresh", AccountID: "account", ExpiresAt: time.Now().Unix()}
	if err := saveCredentials(filename, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadCredentials(filename)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("loadCredentials() = %+v, want %+v", got, want)
	}
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("credential mode = %o, want %o", got, want)
	}
}

func TestTokenClaims(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id":        "account",
			"chatgpt_compute_residency": "eu",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	token := "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
	if got := accountID(token); got != "account" {
		t.Fatalf("accountID() = %q, want account", got)
	}
	if got := tokenResidency(token); got != "eu" {
		t.Fatalf("tokenResidency() = %q, want eu", got)
	}
}
