package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHubClientID is the client ID of Pageroom's GitHub App, set at build time
// with -ldflags "-X github.com/myrheimb/pageroom/internal/secrets.GitHubClientID=...".
// It's a GitHub App, not an OAuth App, so users grant access to chosen
// repositories only and tokens expire. The device flow needs no client
// secret, so the binary holds none.
var GitHubClientID = ""

// DeviceCode is shown to the user: "Go to github.com/login/device and enter WDJB-MJHT".
type DeviceCode struct {
	DeviceCode      string `json:"-"` // never sent to the UI
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
	ExpiresIn       int    `json:"expiresIn"`
	Interval        int    `json:"interval"`
}

type GitHubAuth struct {
	ClientID string
	BaseURL  string // https://github.com, overridable for tests and GHES
	HTTP     *http.Client
}

func (g *GitHubAuth) base() string {
	if g.BaseURL != "" {
		return strings.TrimRight(g.BaseURL, "/")
	}
	return "https://github.com"
}

func (g *GitHubAuth) client() *http.Client {
	if g.HTTP != nil {
		return g.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (g *GitHubAuth) post(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.base()+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := g.client().Do(req)
	if err != nil {
		return fmt.Errorf("could not reach GitHub: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("GitHub is having trouble (HTTP %d); try again shortly", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (g *GitHubAuth) Start(ctx context.Context) (*DeviceCode, error) {
	if g.ClientID == "" {
		return nil, errors.New("this build of Pageroom has no GitHub sign-in configured; use an access token instead")
	}
	var r struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		Error           string `json:"error"`
	}
	if err := g.post(ctx, "/login/device/code", url.Values{"client_id": {g.ClientID}}, &r); err != nil {
		return nil, err
	}
	if r.Error != "" {
		return nil, fmt.Errorf("GitHub refused the sign-in request (%s)", r.Error)
	}
	if r.Interval <= 0 {
		r.Interval = 5
	}
	return &DeviceCode{DeviceCode: r.DeviceCode, UserCode: r.UserCode, VerificationURI: r.VerificationURI, ExpiresIn: r.ExpiresIn, Interval: r.Interval}, nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	Interval     int    `json:"interval"`
}

func (t tokenResponse) credential() Credential {
	c := Credential{Kind: "oauth", Username: "x-access-token", Secret: t.AccessToken, RefreshToken: t.RefreshToken}
	if t.ExpiresIn > 0 {
		c.Expires = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
	}
	return c
}

// Wait polls until the user approves, declines, or the code expires.
func (g *GitHubAuth) Wait(ctx context.Context, dc *DeviceCode) (Credential, error) {
	interval := time.Duration(dc.Interval) * time.Second
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
	for {
		select {
		case <-ctx.Done():
			return Credential{}, ctx.Err()
		case <-time.After(interval):
		}
		var r tokenResponse
		err := g.post(ctx, "/login/oauth/access_token", url.Values{
			"client_id":   {g.ClientID},
			"device_code": {dc.DeviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}, &r)
		if err != nil {
			return Credential{}, err
		}
		switch r.Error {
		case "":
			return r.credential(), nil
		case "authorization_pending":
		case "slow_down":
			interval += 5 * time.Second
		case "expired_token":
			return Credential{}, errors.New("the sign-in code expired; please try again")
		case "access_denied":
			return Credential{}, errors.New("sign-in was cancelled on GitHub")
		default:
			return Credential{}, fmt.Errorf("GitHub sign-in failed (%s)", r.Error)
		}
		if dc.ExpiresIn > 0 && time.Now().After(deadline) {
			return Credential{}, errors.New("the sign-in code expired; please try again")
		}
	}
}

func (g *GitHubAuth) Refresh(ctx context.Context, c Credential) (Credential, error) {
	var r tokenResponse
	err := g.post(ctx, "/login/oauth/access_token", url.Values{
		"client_id":     {g.ClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {c.RefreshToken},
	}, &r)
	if err != nil {
		return Credential{}, err
	}
	if r.Error != "" || r.AccessToken == "" {
		return Credential{}, errors.New("your GitHub sign-in has expired; please sign in again")
	}
	return r.credential(), nil
}
