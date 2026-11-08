package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/myrheimb/pageroom/internal/fsutil"
	"github.com/myrheimb/pageroom/internal/gitsync"
	"github.com/myrheimb/pageroom/internal/hugo"
	"github.com/myrheimb/pageroom/internal/secrets"
	"github.com/myrheimb/pageroom/internal/site"
	"github.com/myrheimb/pageroom/internal/workers"
)

// cloudflareAPI is where the Cloudflare token is kept in the keychain. One
// token serves every site on this computer.
const cloudflareAPI = "https://api.cloudflare.com"

// DirectUpload is a site Pageroom builds itself and uploads to Cloudflare,
// so it needs no git host. Only this computer has the site's files.
type DirectUpload struct {
	Account   string `json:"account"`
	Worker    string `json:"worker"`
	Address   string `json:"address"`
	Published bool   `json:"published"`
}

func directUpload(repo *gitsync.Repo) *DirectUpload {
	values := repo.LocalSettings("cloudflare")
	if values["account"] == "" || values["worker"] == "" {
		return nil
	}
	return &DirectUpload{Account: values["account"], Worker: values["worker"], Address: values["address"], Published: values["published"] == "true"}
}

func saveDirectUpload(repo *gitsync.Repo, direct *DirectUpload) error {
	if direct == nil {
		return repo.SaveLocalSettings("cloudflare", nil)
	}
	return repo.SaveLocalSettings("cloudflare", map[string]string{
		"account": direct.Account, "worker": direct.Worker, "address": direct.Address, "published": fmt.Sprint(direct.Published),
	})
}

func (a *App) cloudflareClient() (*workers.Client, error) {
	credential, err := a.creds.Get(cloudflareAPI)
	if errors.Is(err, secrets.ErrNotFound) {
		return nil, cloudflareTokenError()
	}
	if err != nil {
		return nil, err
	}
	return &workers.Client{Token: credential.Secret, HTTP: a.cloudflareHTTP}, nil
}

func cloudflareTokenError() error {
	return &AppError{Code: "cloudflare-token", Message: "Cloudflare didn't accept Pageroom's token. Create a new one and paste it under Settings → Publishing."}
}

func cloudflareError(err error) error {
	var tooLarge *workers.TooLargeError
	switch {
	case errors.Is(err, workers.ErrAuth):
		return cloudflareTokenError()
	case errors.As(err, &tooLarge):
		return &AppError{Code: "cloudflare-file-too-large", Message: err.Error(), Details: map[string]string{"file": tooLarge.Path}}
	}
	return err
}

// ConnectCloudflare checks the token and sets the site up for Cloudflare.
// If the token can use several accounts and accountID is empty, it saves
// nothing and returns the accounts to choose from. Connecting again replaces
// the token and keeps the site where it is.
func (a *App) ConnectCloudflare(token, accountID string) ([]workers.Account, error) {
	s, err := a.requireSite()
	if err != nil {
		return nil, err
	}
	repo, err := a.requireRepo()
	if err != nil {
		return nil, err
	}
	if repo.RemoteURL() != "" {
		return nil, &AppError{Code: "cloudflare-has-repository", Message: "This site is kept in an online repository. Connect Cloudflare to that repository instead."}
	}
	token = strings.TrimSpace(token)
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	defer cancel()
	client := &workers.Client{Token: token, HTTP: a.cloudflareHTTP}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, cloudflareError(err)
	}

	direct := directUpload(repo)
	if direct != nil {
		accountID = direct.Account
	}
	switch {
	case len(accounts) == 0:
		return nil, &AppError{Code: "cloudflare-no-account", Message: "This token can't use any Cloudflare account. Create it again with the link in Pageroom."}
	case accountID == "" && len(accounts) > 1:
		return accounts, nil
	case accountID == "":
		accountID = accounts[0].ID
	case !slices.ContainsFunc(accounts, func(account workers.Account) bool { return account.ID == accountID }):
		return nil, &AppError{Code: "cloudflare-wrong-account", Message: "This token is for a different Cloudflare account than your site."}
	}

	if direct == nil {
		name, err := freeWorkerName(ctx, client, accountID, s)
		if err != nil {
			return nil, cloudflareError(err)
		}
		subdomain, err := client.Subdomain(ctx, accountID, name)
		if err != nil {
			return nil, cloudflareError(err)
		}
		direct = &DirectUpload{Account: accountID, Worker: name, Address: "https://" + name + "." + subdomain + ".workers.dev/"}
	}
	if err := a.creds.Save(cloudflareAPI, secrets.Credential{Kind: "token", Secret: token}); err != nil {
		return nil, err
	}
	if err := saveDirectUpload(repo, direct); err != nil {
		return nil, err
	}

	settings := s.Settings()
	if settings.BaseURL == "" || strings.Contains(settings.BaseURL, "example.org") || strings.Contains(settings.BaseURL, "example.com") {
		settings.BaseURL = direct.Address
		if err := s.SaveSettings(settings); err != nil {
			return nil, err
		}
		if err := s.Reload(); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// freeWorkerName names the Worker after the site, without taking over a
// Worker the account already has.
func freeWorkerName(ctx context.Context, client *workers.Client, account string, s *site.Site) (string, error) {
	base := workerName(s.Settings().Title, filepath.Base(s.Root))
	for attempt := 1; attempt <= 20; attempt++ {
		name := base
		if attempt > 1 {
			name = fmt.Sprintf("%s-%d", base, attempt)
		}
		exists, err := client.WorkerExists(ctx, account, name)
		if err != nil || !exists {
			return name, err
		}
	}
	return "", errors.New("your Cloudflare account already has many sites with this name; rename your site and try again")
}

// workerName makes a name Cloudflare accepts, which is also part of the
// site's address, from the first of names that has Latin letters or digits.
func workerName(names ...string) string {
	for _, name := range names {
		ascii := strings.Map(func(r rune) rune {
			if r < unicode.MaxASCII {
				return r
			}
			return -1
		}, fsutil.Slugify(name))
		slug := strings.Join(strings.FieldsFunc(ascii, func(r rune) bool { return r == '-' }), "-")
		if slug = strings.Trim(slug[:min(len(slug), 50)], "-"); slug != "" {
			return slug
		}
	}
	return "site"
}

// publishToCloudflare builds and uploads the site. Changes count as
// published only once Cloudflare has them, so a failed upload can be retried.
func (a *App) publishToCloudflare(s *site.Site, repo *gitsync.Repo, direct *DirectUpload, message string) (*gitsync.Outcome, error) {
	changes, err := repo.Changes()
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 && direct.Published {
		return &gitsync.Outcome{State: "up-to-date", Message: "Everything is already published."}, nil
	}
	findings, err := repo.Scan()
	if err != nil {
		return nil, err
	}
	if len(findings) > 0 {
		return &gitsync.Outcome{State: "blocked", Secrets: findings}, nil
	}
	client, err := a.cloudflareClient()
	if err != nil {
		return nil, err
	}
	bin, _, err := a.hugoBin(s)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Minute)
	defer cancel()
	built := filepath.Join(a.workDir(s), "live")
	if err := hugo.Build(ctx, bin, s.Root, a.workDir(s), built); err != nil {
		return nil, &AppError{Code: "hugo", Message: redactor.String(err.Error())}
	}
	if err := client.Deploy(ctx, direct.Account, direct.Worker, built); err != nil {
		return nil, cloudflareError(err)
	}

	if len(changes) > 0 {
		if err := repo.CommitAll(message); err != nil {
			return nil, err
		}
	}
	direct.Published = true
	if err := saveDirectUpload(repo, direct); err != nil {
		return nil, err
	}
	return &gitsync.Outcome{State: "published"}, nil
}

// StopCloudflare takes the site offline by deleting it on Cloudflare, and
// forgets the connection. The site's files on this computer stay.
func (a *App) StopCloudflare() error {
	s, err := a.requireSite()
	if err != nil {
		return err
	}
	repo, err := a.requireRepo()
	if err != nil {
		return err
	}
	direct := directUpload(repo)
	if direct == nil {
		return nil
	}
	client, err := a.cloudflareClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	defer cancel()
	if err := client.DeleteWorker(ctx, direct.Account, direct.Worker); err != nil {
		return cloudflareError(err)
	}
	if err := saveDirectUpload(repo, nil); err != nil {
		return err
	}

	settings := s.Settings()
	if address, err := url.Parse(settings.BaseURL); err == nil && strings.HasSuffix(address.Hostname(), ".workers.dev") {
		settings.BaseURL = ""
		if err := s.SaveSettings(settings); err != nil {
			return err
		}
		return s.Reload()
	}
	return nil
}
