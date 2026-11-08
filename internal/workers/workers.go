// Package workers uploads a built site to Cloudflare as a Worker that only
// serves static files, so a site can go live without a git host.
// https://developers.cloudflare.com/workers/static-assets/direct-upload/
package workers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const api = "https://api.cloudflare.com/client/v4"

const MaxFileSize = 25 << 20

var ErrAuth = errors.New("token not accepted by Cloudflare")

var ErrNotFound = errors.New("not found on Cloudflare")

// TooLargeError names a file too big for Cloudflare to serve.
type TooLargeError struct{ Path string }

func (e *TooLargeError) Error() string {
	return fmt.Sprintf("%s is larger than the 25 MB Cloudflare allows for one file", e.Path)
}

type Account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Client struct {
	Token string
	HTTP  *http.Client
}

type envelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result json.RawMessage `json:"result"`
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

// call sends a request and decodes the result into out (when not nil). The
// token is the API token, or an upload token during an upload.
func (c *Client) call(ctx context.Context, method, endpoint, token, contentType string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, api+endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	var reply envelope
	decodeErr := json.NewDecoder(io.LimitReader(res.Body, 10<<20)).Decode(&reply)
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return ErrAuth
	case res.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case decodeErr != nil:
		return fmt.Errorf("unexpected Cloudflare response: %s", res.Status)
	case !reply.Success || res.StatusCode >= 300:
		var messages []string
		for _, e := range reply.Errors {
			messages = append(messages, e.Message)
		}
		return fmt.Errorf("error from Cloudflare: %s", strings.Join(messages, "; "))
	}
	if out == nil || len(reply.Result) == 0 {
		return nil
	}
	return json.Unmarshal(reply.Result, out)
}

func (c *Client) json(ctx context.Context, method, endpoint string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	return c.call(ctx, method, endpoint, c.Token, "application/json", body, out)
}

func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	var accounts []Account
	err := c.json(ctx, http.MethodGet, "/accounts", nil, &accounts)
	return accounts, err
}

// Subdomain returns the account's workers.dev subdomain, registering one
// based on want if the account has none yet, as is the case for new accounts.
func (c *Client) Subdomain(ctx context.Context, account, want string) (string, error) {
	endpoint := "/accounts/" + account + "/workers/subdomain"
	var current struct {
		Subdomain string `json:"subdomain"`
	}
	err := c.json(ctx, http.MethodGet, endpoint, nil, &current)
	if err == nil && current.Subdomain != "" {
		return current.Subdomain, nil
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return "", err
	}
	// Subdomains are shared by everyone on Cloudflare, so the plain name is
	// often taken.
	suffix := make([]byte, 2)
	_, _ = rand.Read(suffix)
	for _, name := range []string{want, want + "-" + hex.EncodeToString(suffix)} {
		if err = c.json(ctx, http.MethodPut, endpoint, map[string]string{"subdomain": name}, &current); err == nil {
			return current.Subdomain, nil
		}
	}
	return "", err
}

func (c *Client) WorkerExists(ctx context.Context, account, name string) (bool, error) {
	err := c.json(ctx, http.MethodGet, "/accounts/"+account+"/workers/scripts/"+url.PathEscape(name)+"/settings", nil, nil)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// DeleteWorker takes the Worker and its site offline. A Worker that's
// already gone counts as deleted.
func (c *Client) DeleteWorker(ctx context.Context, account, name string) error {
	err := c.json(ctx, http.MethodDelete, "/accounts/"+account+"/workers/scripts/"+url.PathEscape(name)+"?force=true", nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

type manifestEntry struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// manifest lists every file in dir by its address on the site. Cloudflare
// skips uploading files whose hash it already has.
func manifest(dir string) (map[string]manifestEntry, map[string]string, error) {
	entries, files := map[string]manifestEntry{}, map[string]string{}
	err := filepath.WalkDir(dir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, file)
		if err != nil {
			return err
		}
		address := "/" + filepath.ToSlash(rel)
		content, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if len(content) > MaxFileSize {
			return &TooLargeError{Path: address}
		}
		sum := sha256.Sum256([]byte(base64.StdEncoding.EncodeToString(content) + strings.TrimPrefix(path.Ext(address), ".")))
		hash := hex.EncodeToString(sum[:])[:32]
		entries[address] = manifestEntry{Hash: hash, Size: int64(len(content))}
		files[hash] = file
		return nil
	})
	return entries, files, err
}

// Deploy uploads the files in dir as the Worker's site and makes it
// reachable at its workers.dev address.
func (c *Client) Deploy(ctx context.Context, account, name, dir string) error {
	entries, files, err := manifest(dir)
	if err != nil {
		return err
	}
	worker := "/accounts/" + account + "/workers/scripts/" + url.PathEscape(name)
	var session struct {
		JWT     string     `json:"jwt"`
		Buckets [][]string `json:"buckets"`
	}
	if err := c.json(ctx, http.MethodPost, worker+"/assets-upload-session", map[string]any{"manifest": entries}, &session); err != nil {
		return err
	}

	completion := session.JWT
	for _, bucket := range session.Buckets {
		body, contentType, err := uploadForm(bucket, files)
		if err != nil {
			return err
		}
		var uploaded struct {
			JWT string `json:"jwt"`
		}
		if err := c.call(ctx, http.MethodPost, "/accounts/"+account+"/workers/assets/upload?base64=true", session.JWT, contentType, body, &uploaded); err != nil {
			return err
		}
		if uploaded.JWT != "" {
			completion = uploaded.JWT
		}
	}

	metadata, err := json.Marshal(map[string]any{
		"assets": map[string]any{
			"jwt":    completion,
			"config": map[string]string{"html_handling": "auto-trailing-slash", "not_found_handling": "404-page"},
		},
		"compatibility_date": time.Now().UTC().Format(time.DateOnly),
	})
	if err != nil {
		return err
	}
	var script bytes.Buffer
	form := multipart.NewWriter(&script)
	part, err := form.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="metadata"`}, "Content-Type": {"application/json"}})
	if err != nil {
		return err
	}
	if _, err := part.Write(metadata); err != nil {
		return err
	}
	if err := form.Close(); err != nil {
		return err
	}
	if err := c.call(ctx, http.MethodPut, worker, c.Token, form.FormDataContentType(), &script, nil); err != nil {
		return err
	}
	return c.json(ctx, http.MethodPost, worker+"/subdomain", map[string]bool{"enabled": true, "previews_enabled": false}, nil)
}

// uploadForm holds one bucket of files, base64-encoded, each named by its
// hash. Cloudflare serves each file with the type given here.
func uploadForm(bucket []string, files map[string]string) (io.Reader, string, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, hash := range bucket {
		file, ok := files[hash]
		if !ok {
			return nil, "", fmt.Errorf("a file Pageroom did not offer was requested")
		}
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, "", err
		}
		contentType := mime.TypeByExtension(filepath.Ext(file))
		if contentType == "" {
			contentType = "application/null" // Cloudflare then sends no Content-Type
		}
		part, err := form.CreatePart(textproto.MIMEHeader{
			"Content-Disposition": {fmt.Sprintf(`form-data; name=%q; filename=%q`, hash, hash)},
			"Content-Type":        {contentType},
		})
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write([]byte(base64.StdEncoding.EncodeToString(content))); err != nil {
			return nil, "", err
		}
	}
	if err := form.Close(); err != nil {
		return nil, "", err
	}
	return &body, form.FormDataContentType(), nil
}
