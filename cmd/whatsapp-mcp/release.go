package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/brand"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/panel"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/updater"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/wacli"
)

// newRelease is the panel's version card for the command line and the Docker
// image. It looks for a new version a minute after start and then twice a
// day, and only says so: installing is `whatsapp-mcp update` on the server,
// or a new image in Docker.
func newRelease(cfg config, logger *slog.Logger) *panel.Release {
	u := &updater.Updater{
		Repo: strings.TrimPrefix(brand.RepositoryURL, "https://github.com/"),
		// WHATSAPP_MCP_UPDATES_API points the checks at a stand-in for
		// GitHub's API, to try the notice before publishing a version.
		API:     os.Getenv("WHATSAPP_MCP_UPDATES_API"),
		Version: strings.TrimPrefix(version, "v"),
		Logger:  logger,
		Dir:     filepath.Join(cfg.DataDir, "updates"),
		Target:  updater.Target{Kind: "cli", Path: platform.Executable()},
	}
	check := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = u.Check(ctx)
	}
	if u.Enabled() {
		go func() {
			time.Sleep(time.Minute)
			for {
				check()
				time.Sleep(12 * time.Hour)
			}
		}()
	}
	return &panel.Release{
		Version:      version,
		WacliVersion: wacli.Version(context.Background(), cfg.WacliBin),
		Container:    inContainer(),
		Update: func() panel.UpdateState {
			s := u.State()
			return panel.UpdateState{State: s.Phase, Current: version, Latest: s.Latest, Page: s.Page, Error: s.Error, CheckedAt: s.CheckedAt}
		},
		Check: check,
	}
}
