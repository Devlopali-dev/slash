// Package version provides a runner to check the latest version of the application.
package version

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/devlopali-dev/slash/server/profile"
	"github.com/devlopali-dev/slash/store"
)

type Runner struct {
	Store   *store.Store
	Profile *profile.Profile
}

func NewRunner(store *store.Store, profile *profile.Profile) *Runner {
	return &Runner{
		Store:   store,
		Profile: profile,
	}
}

// Schedule checker every 8 hours.
const runnerInterval = time.Hour * 8

func (r *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(runnerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.RunOnce(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (r *Runner) RunOnce(ctx context.Context) {
	if r.Profile.Mode == "prod" {
		checkLatestRelease(ctx, r.Profile.Version)
	}
}

func checkLatestRelease(ctx context.Context, currentVersion string) {
	// Fetch latest release from GitHub API.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/devlopali/slash/releases/latest", nil)
	if err != nil {
		slog.Warn("version runner: failed to create request", slog.String("error", err.Error()))
		return
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "slash/"+currentVersion)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("version runner: failed to fetch latest release", slog.String("error", err.Error()))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("version runner: GitHub API returned non-200", slog.Int("status", resp.StatusCode))
		return
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		slog.Warn("version runner: failed to decode release", slog.String("error", err.Error()))
		return
	}

	if release.TagName == "" || release.TagName == currentVersion {
		return
	}

	slog.Info("version runner: new version available",
		slog.String("current", currentVersion),
		slog.String("latest", release.TagName),
	)
}
