package codex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	clientID  = "app_EMoamEEZ73f0CkXaXp7hrann"
	issuer    = "https://auth.openai.com"
	userAgent = "freegent/0.1"
)

type credentials struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
	ExpiresAt    int64  `json:"expires_at"`
}

type tokenResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type jwtClaims struct {
	AccountID string `json:"chatgpt_account_id"`
	Residency string `json:"chatgpt_compute_residency"`
	Auth      struct {
		AccountID string `json:"chatgpt_account_id"`
		Residency string `json:"chatgpt_compute_residency"`
	} `json:"https://api.openai.com/auth"`
	Organizations []struct {
		ID string `json:"id"`
	} `json:"organizations"`
}

type deviceCode struct {
	ID       string `json:"device_auth_id"`
	UserCode string `json:"user_code"`
	Interval string `json:"interval"`
}

type deviceToken struct {
	Code     string `json:"authorization_code"`
	Verifier string `json:"code_verifier"`
}

type Access struct {
	Token     string
	AccountID string
	Residency string
}

type TokenSource struct {
	mu       sync.Mutex
	client   *http.Client
	filename string
	auth     credentials
}

func DefaultAuthPath() (string, error) {
	if filename := strings.TrimSpace(os.Getenv("FREEGENT_CODEX_AUTH_FILE")); filename != "" {
		return filename, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, "freegent", "codex-auth.json"), nil
}

func NewTokenSource(filename string, client *http.Client) (*TokenSource, error) {
	auth, err := loadCredentials(filename)
	if err != nil {
		return nil, err
	}
	return &TokenSource{client: client, filename: filename, auth: auth}, nil
}

func (s *TokenSource) Access(ctx context.Context) (Access, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.auth.ExpiresAt <= time.Now().Add(time.Minute).Unix() {
		auth, err := refresh(ctx, s.client, s.auth)
		if err != nil {
			return Access{}, err
		}
		if err := saveCredentials(s.filename, auth); err != nil {
			return Access{}, err
		}
		s.auth = auth
	}
	return Access{Token: s.auth.AccessToken, AccountID: s.auth.AccountID, Residency: tokenResidency(s.auth.AccessToken)}, nil
}

func Authenticate(ctx context.Context, client *http.Client, filename string, output io.Writer) error {
	auth, err := login(ctx, client, output)
	if err != nil {
		return err
	}
	if err := saveCredentials(filename, auth); err != nil {
		return err
	}
	fmt.Fprintf(output, "Freegent Codex authentication saved to %s\n", filename)
	return nil
}

func loadCredentials(filename string) (credentials, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return credentials{}, fmt.Errorf("read Codex credentials: %w", err)
	}
	var auth credentials
	if err := json.Unmarshal(data, &auth); err != nil {
		return credentials{}, fmt.Errorf("decode Codex credentials: %w", err)
	}
	if auth.RefreshToken == "" {
		return credentials{}, errors.New("decode Codex credentials: missing refresh token")
	}
	return auth, nil
}

func saveCredentials(filename string, auth credentials) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return fmt.Errorf("create Codex credentials directory: %w", err)
	}
	data, err := json.Marshal(auth)
	if err != nil {
		return fmt.Errorf("encode Codex credentials: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".codex-auth-*")
	if err != nil {
		return fmt.Errorf("create temporary Codex credentials: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure temporary Codex credentials: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write Codex credentials: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close Codex credentials: %w", err)
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return fmt.Errorf("replace Codex credentials: %w", err)
	}
	return nil
}

func login(ctx context.Context, client *http.Client, output io.Writer) (credentials, error) {
	var device deviceCode
	if err := postJSON(ctx, client, issuer+"/api/accounts/deviceauth/usercode", map[string]string{"client_id": clientID}, &device); err != nil {
		return credentials{}, fmt.Errorf("start device login: %w", err)
	}
	if device.ID == "" || device.UserCode == "" {
		return credentials{}, errors.New("start device login: incomplete response")
	}
	authURL := issuer + "/codex/device"
	fmt.Fprintf(output, "Open %s and enter code %s\n", authURL, device.UserCode)
	openBrowser(authURL)
	wait, err := time.ParseDuration(device.Interval + "s")
	if err != nil || wait < time.Second {
		wait = 5 * time.Second
	}
	wait += 3 * time.Second
	loginCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	for {
		request, err := json.Marshal(map[string]string{"device_auth_id": device.ID, "user_code": device.UserCode})
		if err != nil {
			return credentials{}, fmt.Errorf("encode device login request: %w", err)
		}
		response, err := do(loginCtx, client, issuer+"/api/accounts/deviceauth/token", "application/json", request)
		if err != nil {
			return credentials{}, fmt.Errorf("poll device login: %w", err)
		}
		if response.StatusCode == http.StatusOK {
			var token deviceToken
			if err := decodeResponse(response, &token); err != nil {
				return credentials{}, fmt.Errorf("decode device login: %w", err)
			}
			if token.Code == "" || token.Verifier == "" {
				return credentials{}, errors.New("decode device login: incomplete response")
			}
			return exchange(ctx, client, token)
		}
		if response.StatusCode != http.StatusForbidden && response.StatusCode != http.StatusNotFound {
			return credentials{}, responseError("poll device login", response)
		}
		if err := response.Body.Close(); err != nil {
			return credentials{}, fmt.Errorf("close device login response: %w", err)
		}
		timer := time.NewTimer(wait)
		select {
		case <-loginCtx.Done():
			timer.Stop()
			return credentials{}, fmt.Errorf("wait for device login: %w", loginCtx.Err())
		case <-timer.C:
		}
	}
}

func openBrowser(target string) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", target)
	case "linux":
		command = exec.Command("xdg-open", target)
	default:
		return
	}
	_ = command.Start()
}

func exchange(ctx context.Context, client *http.Client, device deviceToken) (credentials, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {device.Code},
		"redirect_uri":  {issuer + "/deviceauth/callback"},
		"client_id":     {clientID},
		"code_verifier": {device.Verifier},
	}
	var tokens tokenResponse
	if err := postForm(ctx, client, issuer+"/oauth/token", form, &tokens); err != nil {
		return credentials{}, fmt.Errorf("exchange authorization code: %w", err)
	}
	return credentialsFrom(tokens, credentials{})
}

func refresh(ctx context.Context, client *http.Client, current credentials) (credentials, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {current.RefreshToken},
		"client_id":     {clientID},
	}
	var tokens tokenResponse
	if err := postForm(ctx, client, issuer+"/oauth/token", form, &tokens); err != nil {
		return credentials{}, fmt.Errorf("refresh Codex access token: %w", err)
	}
	return credentialsFrom(tokens, current)
}

func credentialsFrom(tokens tokenResponse, current credentials) (credentials, error) {
	if tokens.AccessToken == "" {
		return credentials{}, errors.New("token response is missing access token")
	}
	if tokens.RefreshToken == "" {
		tokens.RefreshToken = current.RefreshToken
	}
	if tokens.RefreshToken == "" {
		return credentials{}, errors.New("token response is missing refresh token")
	}
	expires := tokens.ExpiresIn
	if expires == 0 {
		expires = 3600
	}
	account := accountID(tokens.IDToken)
	if account == "" {
		account = accountID(tokens.AccessToken)
	}
	if account == "" {
		account = current.AccountID
	}
	return credentials{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken, AccountID: account, ExpiresAt: time.Now().Add(time.Duration(expires) * time.Second).Unix()}, nil
}

func postJSON(ctx context.Context, client *http.Client, endpoint string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	response, err := do(ctx, client, endpoint, "application/json", body)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return responseError("request", response)
	}
	return decodeResponse(response, output)
}

func postForm(ctx context.Context, client *http.Client, endpoint string, form url.Values, output any) error {
	response, err := do(ctx, client, endpoint, "application/x-www-form-urlencoded", []byte(form.Encode()))
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return responseError("request", response)
	}
	return decodeResponse(response, output)
}

func do(ctx context.Context, client *http.Client, endpoint, contentType string, body []byte) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("User-Agent", userAgent)
	return client.Do(request)
}

func decodeResponse(response *http.Response, output any) error {
	defer response.Body.Close()
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func responseError(operation string, response *http.Response) error {
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%s: status %s; read body: %w", operation, response.Status, err)
	}
	return fmt.Errorf("%s: status %s: %s", operation, response.Status, strings.TrimSpace(string(body)))
}

func accountID(token string) string {
	claims, ok := parseClaims(token)
	if !ok {
		return ""
	}
	if claims.AccountID != "" {
		return claims.AccountID
	}
	if claims.Auth.AccountID != "" {
		return claims.Auth.AccountID
	}
	if len(claims.Organizations) > 0 {
		return claims.Organizations[0].ID
	}
	return ""
}

func tokenResidency(token string) string {
	claims, ok := parseClaims(token)
	if !ok {
		return ""
	}
	residency := claims.Residency
	if claims.Auth.Residency != "" {
		residency = claims.Auth.Residency
	}
	if residency == "no_constraint" {
		return ""
	}
	return residency
}

func parseClaims(token string) (jwtClaims, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return jwtClaims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtClaims{}, false
	}
	var claims jwtClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return jwtClaims{}, false
	}
	return claims, true
}
