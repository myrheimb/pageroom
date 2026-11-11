// Package update asks GitHub whether a newer Pageroom release is out.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// LatestURL is GitHub's answer for the newest published, non-prerelease release.
const LatestURL = "https://api.github.com/repos/myrheimb/pageroom/releases/latest"

const (
	timeout      = 5 * time.Second
	maxBodyBytes = 1 << 20
)

type Release struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// Check returns the latest release if it is newer than current, and nil when
// it isn't or when anything goes wrong: an update notice is never worth an error.
func Check(ctx context.Context, client *http.Client, url, current string) *Release {
	currentParts, ok := parse(current)
	if !ok {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	request.Header.Set("User-Agent", "Pageroom/"+current)
	request.Header.Set("Accept", "application/vnd.github+json")

	response, err := client.Do(request)
	if err != nil {
		return nil
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil
	}

	var latest struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxBodyBytes)).Decode(&latest); err != nil {
		return nil
	}

	version := strings.TrimPrefix(latest.Tag, "v")
	latestParts, ok := parse(version)
	if !ok || !newer(latestParts, currentParts) {
		return nil
	}
	return &Release{Version: version, URL: "https://github.com/myrheimb/pageroom/releases/tag/v" + version}
}

func parse(version string) ([3]int, bool) {
	var parts [3]int
	fields := strings.Split(version, ".")
	if len(fields) != len(parts) {
		return parts, false
	}
	for index, field := range fields {
		number, err := strconv.Atoi(field)
		if err != nil || number < 0 || fmt.Sprint(number) != field {
			return parts, false
		}
		parts[index] = number
	}
	return parts, true
}

func newer(candidate, current [3]int) bool {
	for index := range candidate {
		if candidate[index] != current[index] {
			return candidate[index] > current[index]
		}
	}
	return false
}
