package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/account"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/localasr"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/mcp"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/state"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/wacli"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/webhook"
)

// Account is one WhatsApp number with everything that is its own: wacli's
// store and the sync that keeps it, the index over it, the state database
// (transcripts, triage, webhooks, settings), the MCP server and the webhook
// deliveries. The daemon runs the main number as one; the app runs further
// numbers the same way, each in its own folders.
type Account struct {
	// Slug is empty for the main number. A further number's slug names its
	// folder and its MCP address, and never changes.
	Slug     string
	StoreDir string
	DataDir  string

	logger *slog.Logger
	cancel context.CancelFunc

	cli    *wacli.CLI
	idx    *index.Index
	st     *state.State
	sup    *wacli.Supervisor
	server *mcp.Server

	hooks      *webhook.Manager
	relay      *webhook.Relay
	webhooksOn bool

	syncDone chan struct{}
}

// AccountSpec says where a number keeps its data.
type AccountSpec struct {
	Slug     string
	StoreDir string
	DataDir  string
}

// startAccount opens a number's databases and starts its sync, webhooks and
// MCP server. It stops with ctx or Stop.
func (d *Daemon) startAccount(ctx context.Context, spec AccountSpec, asr *localasr.Engine) (*Account, error) {
	for _, dir := range []string{spec.StoreDir, spec.DataDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("cannot create %s: %w", dir, err)
		}
	}
	logger := d.logger
	if spec.Slug != "" {
		logger = logger.With("account", spec.Slug)
	}
	a := &Account{Slug: spec.Slug, StoreDir: spec.StoreDir, DataDir: spec.DataDir, logger: logger, syncDone: make(chan struct{})}
	a.cli = &wacli.CLI{Bin: d.cfg.WacliBin, StoreDir: spec.StoreDir}
	var err error
	if a.idx, err = index.Open(filepath.Join(spec.StoreDir, "wacli.db")); err != nil {
		return nil, err
	}
	if a.st, err = state.Open(spec.DataDir); err != nil {
		a.idx.Close()
		return nil, err
	}

	ctx, a.cancel = context.WithCancel(ctx)
	a.sup = wacli.NewSupervisor(a.cli, logger)
	a.startWebhooks(ctx, d.cfg.WacliBin)
	go func() {
		a.sup.Run(ctx)
		close(a.syncDone)
	}()

	a.server = mcp.New(mcp.Config{CLI: a.cli, Supervisor: a.sup, Index: a.idx, State: a.st, Logger: logger,
		BaseURL: baseURL(d.Port()), MediaDir: filepath.Join(spec.DataDir, "media"), ExportDir: filepath.Join(spec.DataDir, "exports"), ASR: asr,
		Account: a.Info, Accounts: d.accountCount})
	return a, nil
}

// stop ends the number's sync, giving it up to 20 seconds to close its
// store, then its webhooks and databases.
func (a *Account) stop() {
	a.cancel()
	<-a.syncDone
	if a.relay != nil {
		_ = a.relay.Close()
	}
	a.st.Close()
	a.idx.Close()
}

// Info is the number as the person knows it: its label, phone and profile
// name. It reads only local databases, so it is cheap to ask often.
func (a *Account) Info(ctx context.Context) account.Info {
	info := account.Info{Slug: a.Slug}
	info.Label, _ = a.st.Setting(ctx, account.LabelSetting)
	phone, _ := a.idx.Names.Own(ctx)
	info.Phone = account.FormatPhone(phone)
	info.Name = a.idx.Names.Self(ctx)
	return info
}

// SetLabel names the number ("Pessoal", "Trabalho"); an empty label clears it.
func (a *Account) SetLabel(ctx context.Context, label string) error {
	return a.st.SetSetting(ctx, account.LabelSetting, account.CleanLabel(label))
}

// Server is the number's MCP server.
func (a *Account) Server() *mcp.Server { return a.server }

// Supervisor is the number's wacli supervisor.
func (a *Account) Supervisor() *wacli.Supervisor { return a.sup }

// startWebhooks opens the relay sync posts new messages to and starts
// delivering them to the configured webhooks. A wacli too old to post
// messages leaves webhooks off rather than failing sync on an unknown flag.
func (a *Account) startWebhooks(ctx context.Context, wacliBin string) {
	a.hooks = webhook.New(a.st, a.logger, mcp.Version, a.idx.Names.Same)
	a.hooks.SetAccount(a.Info)
	relay, err := webhook.NewRelay(a.hooks, a.idx.Names)
	if err != nil {
		a.logger.Warn("webhooks are off: no loopback port for the relay", "error", err)
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if !a.cli.Supports(cctx, "--webhook-events", "sync") {
		a.logger.Warn("webhooks are off: this wacli cannot post new messages", "wacli", wacliBin)
		_ = relay.Close()
		return
	}
	a.relay, a.webhooksOn = relay, true
	a.sup.SetWebhook(relay.URL(), relay.Secret())
	if err := a.hooks.Start(ctx); err != nil {
		a.logger.Warn("could not load the webhooks", "error", err)
	}
}
