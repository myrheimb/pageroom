package app

import (
	"context"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/myrheimb/pageroom/internal/secrets"
)

// AuthStatus tells the UI whether the user is signed in to the site's host.
// It never includes the secret itself.
type AuthStatus struct {
	Host     string `json:"host"`
	SignedIn bool   `json:"signedIn"`
	Kind     string `json:"kind,omitempty"`
	Username string `json:"username,omitempty"`
	Expires  string `json:"expires,omitempty"`
}

func (a *App) remoteOrDefault(remote string) string {
	if remote != "" {
		return remote
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.repo != nil && a.repo.RemoteURL() != "" {
		return a.repo.RemoteURL()
	}
	return "https://github.com"
}

func (a *App) GetAuthStatus(remote string) AuthStatus {
	remote = a.remoteOrDefault(remote)
	st := AuthStatus{Host: hostLabel(remote)}
	if c, err := a.creds.Get(remote); err == nil {
		st.SignedIn, st.Kind = true, c.Kind
		if c.Username != "x-access-token" {
			st.Username = c.Username
		}
		if !c.Expires.IsZero() {
			st.Expires = c.Expires.Format(time.RFC3339)
		}
	}
	return st
}

// StartGitHubSignIn begins GitHub's device sign-in: the UI shows the code,
// and the browser opens github.com/login/device. Call WaitGitHubSignIn next.
func (a *App) StartGitHubSignIn() (*secrets.DeviceCode, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	dc, err := a.github.Start(ctx)
	if err != nil {
		return nil, &AppError{Code: "sign-in", Message: err.Error()}
	}
	a.mu.Lock()
	a.pendingDevice = dc
	a.mu.Unlock()
	// The UI shows the address too, so a browser that fails to open isn't fatal.
	_ = application.Get().Browser.OpenURL(dc.VerificationURI)
	return dc, nil
}

// WaitGitHubSignIn waits for the user to approve in the browser, then
// stores the token in the OS keychain.
func (a *App) WaitGitHubSignIn() (AuthStatus, error) {
	a.mu.Lock()
	dc := a.pendingDevice
	a.mu.Unlock()
	if dc == nil {
		return AuthStatus{}, &AppError{Code: "sign-in-not-started", Message: "Start signing in first."}
	}
	ctx, cancel := context.WithTimeout(a.ctx, time.Duration(max(dc.ExpiresIn, 60))*time.Second)
	a.mu.Lock()
	a.signIn = cancel
	a.mu.Unlock()
	defer cancel()
	c, err := a.github.Wait(ctx, dc)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return AuthStatus{}, &AppError{Code: "cancelled", Message: "Sign-in cancelled."}
		}
		return AuthStatus{}, &AppError{Code: "sign-in", Message: err.Error()}
	}
	if err := a.creds.Save("https://github.com", c); err != nil {
		return AuthStatus{}, err
	}
	return a.GetAuthStatus("https://github.com"), nil
}

func (a *App) CancelSignIn() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.signIn != nil {
		a.signIn()
		a.signIn = nil
	}
	a.pendingDevice = nil
}

// SaveAccessToken stores a personal access token (for GitLab, Bitbucket,
// self-hosted git, or GitHub without the sign-in app) in the keychain.
func (a *App) SaveAccessToken(remote, username, token string) (AuthStatus, error) {
	token = strings.TrimSpace(token)
	if len(token) < 8 {
		return AuthStatus{}, &AppError{Code: "token-too-short", Message: "That token looks too short."}
	}
	remote = a.remoteOrDefault(remote)
	if strings.TrimSpace(username) == "" {
		username = "x-access-token"
	}
	if err := a.creds.Save(remote, secrets.Credential{Kind: "token", Username: strings.TrimSpace(username), Secret: token}); err != nil {
		reason := redactor.String(err.Error())
		return AuthStatus{}, &AppError{Code: "keychain", Message: "Your computer's password storage couldn't save the token: " + reason, Details: map[string]string{"reason": reason}}
	}
	return a.GetAuthStatus(remote), nil
}

func (a *App) SignOut(remote string) error {
	return a.creds.Delete(a.remoteOrDefault(remote))
}
