// Command yaadegar is the CLI for the yaadegar server: a self-hosted, API-first
// gift registry (ADR-0001).
//
// The CLI stays thin (house convention): it parses config and wires the internal
// packages, then runs. All behavior lives under internal/. Config layers as
// file < env < flag via an optional YAML file (searched at
// /etc/yaadegar/config.yaml and ./config.yaml). Secrets come from the
// environment, never inlined in config.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	kongyaml "github.com/alecthomas/kong-yaml"

	"github.com/yaad-index/yaadegar/internal/api"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/clock"
	"github.com/yaad-index/yaadegar/internal/decay"
	"github.com/yaad-index/yaadegar/internal/email"
	"github.com/yaad-index/yaadegar/internal/server"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/storage/sqlstore"
)

// version is the build version, overridden at link time via -ldflags.
var version = "dev"

// CLI is the yaadegar command surface. Every config value resolves through
// file < env < flag.
type CLI struct {
	LogLevel string `name:"log-level" default:"info" enum:"debug,info,warn,error" env:"YAADEGAR_LOG_LEVEL" help:"Log verbosity."`

	Serve   ServeCmd   `cmd:"" help:"Run the HTTP API server."`
	Version VersionCmd `cmd:"" help:"Print the build version and exit."`
	Health  HealthCmd  `cmd:"" help:"Probe the server's health endpoint and exit non-zero if unhealthy (container healthcheck)."`

	// Seed / operator commands.
	CreateTenant CreateTenantCmd `cmd:"" name:"create-tenant" help:"Create a tenant."`
	CreateOwner  CreateOwnerCmd  `cmd:"" name:"create-owner" help:"Create an owner with a password credential in a tenant."`
	HashPassword HashPasswordCmd `cmd:"" name:"hash-password" help:"Print the argon2id hash of a password (read from $YAADEGAR_PASSWORD or stdin) for the superadmin config."`
}

// ServeCmd runs the HTTP server until interrupted.
type ServeCmd struct {
	HTTPAddr      string `name:"http-addr" default:":8080" env:"YAADEGAR_HTTP_ADDR" help:"HTTP listen address."`
	StorageDriver string `name:"storage-driver" default:"sqlite" enum:"sqlite,postgres" env:"YAADEGAR_STORAGE_DRIVER" help:"Storage driver."`
	StorageDSN    string `name:"storage-dsn" default:"file:yaadegar.db" env:"YAADEGAR_STORAGE_DSN" help:"Storage DSN: a SQLite file path/URI or a Postgres connection URL."`
	BaseDomain    string `name:"base-domain" env:"YAADEGAR_BASE_DOMAIN" help:"Host suffix under which tenant subdomains live (e.g. example.wish.list). Hosts outside it are treated as custom domains."`

	// TrustForwardedHost is off by default (untrusted-safe). Enable it ONLY when the
	// backend port is not externally reachable and requests arrive through the
	// trusted frontend proxy; on a directly-exposed backend it is a tenant-spoofing
	// hole (ADR-0004 §7).
	TrustForwardedHost bool `name:"trust-forwarded-host" env:"YAADEGAR_TRUST_FORWARDED_HOST" help:"Resolve the tenant from X-Forwarded-Host (proxy deployments). Enable ONLY when the backend is reachable exclusively behind the trusted frontend; a directly-exposed backend must leave this off."`

	DecaySweepInterval  time.Duration `name:"decay-sweep-interval" default:"15m" env:"YAADEGAR_DECAY_SWEEP_INTERVAL" help:"How often the reservation-decay sweeper runs (0 disables it)."`
	DecayDefaultDays    int           `name:"decay-default-days" default:"0" env:"YAADEGAR_DECAY_DEFAULT_DAYS" help:"Instance-default decay period in days (0 = off) for lists that do not override it."`
	DecayResponseWindow time.Duration `name:"decay-response-window" default:"48h" env:"YAADEGAR_DECAY_RESPONSE_WINDOW" help:"How long the reserver has to keep/release a stale reservation before it auto-expires."`
	DecayLinkBase       string        `name:"decay-link-base" env:"YAADEGAR_DECAY_LINK_BASE" help:"DEPRECATED alias for --public-link-base (still honoured for the keep/release links when --public-link-base is unset)."`

	// PublicLinkBase is the giver-facing site base for every emailed link (confirm
	// + decay keep/release). It supersedes DecayLinkBase, which stays as a
	// back-compat alias so existing self-hosts keep working.
	PublicLinkBase string `name:"public-link-base" env:"YAADEGAR_PUBLIC_LINK_BASE" help:"Base URL of the giver-facing site for links in emails (reservation confirm, decay keep/release). Supersedes --decay-link-base."`

	ReserverConfirmWindow time.Duration `name:"reserver-confirm-window" default:"30m" env:"YAADEGAR_RESERVER_CONFIRM_WINDOW" help:"How long an email_confirmed reservation may sit unconfirmed before it auto-expires and frees the item (ADR-0007). 0 disables the confirm-window sweep."`
	ReserverDefaultTier   string        `name:"reserver-default-tier" default:"full_guest" env:"YAADEGAR_RESERVER_DEFAULT_TIER" help:"Instance-default reserver tier for lists that set no override (ADR-0007): full_guest | email_confirmed | registered."`

	CobuyConfirmWindow time.Duration `name:"cobuy-confirm-window" default:"168h" env:"YAADEGAR_COBUY_CONFIRM_WINDOW" help:"How long a co-buying match's emailed confirm/decline link stays valid after the match is proposed (#96). 0 means it never expires."`

	DomainCNAMETarget string `name:"domain-cname-target" env:"YAADEGAR_DOMAIN_CNAME_TARGET" help:"Hostname that owners point a custom domain's CNAME at (returned by add-domain)."`

	DomainClaimTTL time.Duration `name:"domain-claim-ttl" default:"168h" env:"YAADEGAR_DOMAIN_CLAIM_TTL" help:"How long an unverified custom-domain claim holds its hostname before another tenant can reclaim it at add time (0 disables reclaiming). A verified domain is never reclaimed."`

	// Auth config (ADR-0005). The JWT secret is a secret and comes from the
	// environment only. At least one login method must be enabled and configured or
	// the instance refuses to start.
	AuthJWTSecret       string        `name:"auth-jwt-secret" env:"YAADEGAR_AUTH_JWT_SECRET" help:"JWT signing secret (HS256), >=32 bytes. Required; from the environment. The instance refuses to start if missing or too short."`
	AuthPasswordEnabled bool          `name:"auth-password-enabled" default:"true" env:"YAADEGAR_AUTH_PASSWORD_ENABLED" help:"Enable username+password login (the first login method; magic-link and OAuth land later)."`
	AuthAccessTTL       time.Duration `name:"auth-access-ttl" default:"12h" env:"YAADEGAR_AUTH_ACCESS_TTL" help:"Access-token lifetime; re-login on expiry (refresh tokens are a later cut)."`

	// Superadmin config (ADR-0005 §6). Both set → the /admin surface is enabled and
	// the identity is ensured at startup; neither set → the admin surface is
	// disabled (not an error). The password is provided as an argon2id hash from
	// `yaadegar hash-password`, never as plaintext.
	SuperadminUsername     string `name:"superadmin-username" env:"YAADEGAR_SUPERADMIN_USERNAME" help:"Superadmin login username. Set together with the password hash to enable the /admin surface."`
	SuperadminPasswordHash string `name:"superadmin-password-hash" env:"YAADEGAR_SUPERADMIN_PASSWORD_HASH" help:"Superadmin argon2id password hash (from 'yaadegar hash-password'). Never a plaintext password."`

	// Login brute-force rate limit (applies to both owner and admin login), per IP
	// and per username. In-memory (single-instance); a multi-instance deployment
	// would swap in a shared-state limiter behind the same interface.
	LoginRateMaxFailures int           `name:"login-rate-max-failures" default:"10" env:"YAADEGAR_LOGIN_RATE_MAX_FAILURES" help:"Failed login attempts per IP and per username before rate-limiting kicks in (0 disables)."`
	LoginRateWindow      time.Duration `name:"login-rate-window" default:"15m" env:"YAADEGAR_LOGIN_RATE_WINDOW" help:"Window over which failed login attempts accumulate and the lockout lasts."`

	// SMTP config. If SMTPHost is empty the server logs emails instead of sending
	// them (dev default). Secrets (SMTPPassword) come from the environment.
	SMTPHost     string `name:"smtp-host" env:"YAADEGAR_SMTP_HOST" help:"SMTP server host. Empty logs emails instead of sending (dev default)."`
	SMTPPort     int    `name:"smtp-port" default:"587" env:"YAADEGAR_SMTP_PORT" help:"SMTP server port (587 STARTTLS, 465 implicit TLS)."`
	SMTPUsername string `name:"smtp-username" env:"YAADEGAR_SMTP_USERNAME" help:"SMTP auth username (relay-with-auth, e.g. a Gmail address + app password)."`
	SMTPPassword string `name:"smtp-password" env:"YAADEGAR_SMTP_PASSWORD" help:"SMTP auth password (use an app password; provide via the environment)."`
	SMTPFrom     string `name:"smtp-from" env:"YAADEGAR_SMTP_FROM" help:"Envelope/header From address for outgoing mail."`
	SMTPTLSMode  string `name:"smtp-tls-mode" default:"starttls" enum:"starttls,tls,none" env:"YAADEGAR_SMTP_TLS_MODE" help:"TLS mode: starttls (587, required), tls (465 implicit), or none (plaintext, loopback only)."`
}

// Run opens and migrates storage, builds the API handler, and serves until
// SIGINT/SIGTERM, then shuts down cleanly.
func (c *ServeCmd) Run(cli *CLI) error {
	logger := newLogger(cli.LogLevel)
	logger.Info("yaadegar starting",
		"version", version, "http_addr", c.HTTPAddr, "storage_driver", c.StorageDriver)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := sqlstore.Open(ctx, storage.Config{
		Driver: storage.Driver(c.StorageDriver),
		DSN:    c.StorageDSN,
	})
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer func() { _ = store.Close() }()

	if err := store.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate storage: %w", err)
	}

	sender, err := buildSender(c, logger)
	if err != nil {
		return fmt.Errorf("configure email sender: %w", err)
	}

	// Fail-closed: the owner surface must never fall open. NewService validates the
	// signing secret and the at-least-one-method-enabled invariant; a violation
	// aborts startup with a clear, actionable error (ADR-0005 §4).
	authService, err := auth.NewService(auth.Config{
		JWTSecret:       c.AuthJWTSecret,
		AccessTTL:       c.AuthAccessTTL,
		PasswordEnabled: c.AuthPasswordEnabled,
	}, clock.Real{})
	if err != nil {
		return err
	}

	// Superadmin bootstrap (ADR-0005 §6): both fields set → ensure the identity
	// (idempotent) and enable /admin; neither set → the admin surface stays
	// disabled; exactly one set → a misconfiguration, fail closed with a clear error.
	adminEnabled, err := ensureSuperadmin(ctx, store, c, logger)
	if err != nil {
		return err
	}

	// The public link base (giver-facing site) feeds every emailed link; the old
	// --decay-link-base is honoured as a back-compat alias when it is unset.
	linkBase := c.PublicLinkBase
	if linkBase == "" {
		linkBase = c.DecayLinkBase
	}
	// Fail closed on a bogus instance-default tier rather than silently reserving
	// as full_guest (ADR-0007).
	defaultTier := storage.ReserverTier(c.ReserverDefaultTier)
	switch defaultTier {
	case storage.TierFullGuest, storage.TierEmailConfirmed, storage.TierRegistered:
	default:
		return fmt.Errorf("invalid --reserver-default-tier %q (want full_guest, email_confirmed, or registered)", c.ReserverDefaultTier)
	}

	handler := api.NewHandler(store, api.Options{
		BaseDomain:          c.BaseDomain,
		Logger:              logger,
		Email:               sender,
		Auth:                authService,
		AdminEnabled:        adminEnabled,
		TrustForwardedHost:  c.TrustForwardedHost,
		LoginLimiter:        auth.NewInMemoryLimiter(c.LoginRateMaxFailures, c.LoginRateWindow, clock.Real{}),
		DomainCNAMETarget:   c.DomainCNAMETarget,
		DomainClaimTTL:      c.DomainClaimTTL,
		DefaultReserverTier: defaultTier,
		PublicLinkBase:      linkBase,
		CobuyConfirmWindow:  c.CobuyConfirmWindow,
	})

	// Run the reservation-decay sweeper on a ticker alongside the server.
	sweeper := decay.NewSweeper(store, sender, clock.Real{}, decay.Config{
		DefaultDecayDays: c.DecayDefaultDays,
		ResponseWindow:   c.DecayResponseWindow,
		ConfirmWindow:    c.ReserverConfirmWindow,
		LinkBase:         linkBase,
	}, logger)
	go runSweeper(ctx, sweeper, c.DecaySweepInterval, logger)

	return server.New(c.HTTPAddr, handler, logger).Run(ctx)
}

// ensureSuperadmin applies the superadmin bootstrap rule and reports whether the
// admin surface is enabled. Both fields set → EnsureAdmin (idempotent) + enabled;
// neither → disabled; exactly one → a fail-closed configuration error.
func ensureSuperadmin(ctx context.Context, store storage.Store, c *ServeCmd, logger *slog.Logger) (bool, error) {
	switch {
	case c.SuperadminUsername != "" && c.SuperadminPasswordHash != "":
		if _, err := store.EnsureAdmin(ctx, c.SuperadminUsername, c.SuperadminPasswordHash); err != nil {
			return false, fmt.Errorf("ensure superadmin: %w", err)
		}
		logger.Info("admin surface enabled", "superadmin", c.SuperadminUsername)
		return true, nil
	case c.SuperadminUsername != "" || c.SuperadminPasswordHash != "":
		return false, errors.New(
			"superadmin requires both --superadmin-username and --superadmin-password-hash (set neither to disable the admin surface)")
	default:
		return false, nil // no superadmin configured; admin surface disabled
	}
}

// buildSender returns the real SMTP sender when an SMTP host is configured, and
// the log-only sender (dev default) otherwise. The same sender backs both the API
// handler and the decay sweeper.
func buildSender(c *ServeCmd, logger *slog.Logger) (email.Sender, error) {
	if c.SMTPHost == "" {
		logger.Info("email: no SMTP host configured, logging emails instead of sending")
		return email.NewLogSender(logger), nil
	}
	return email.NewSMTPSender(email.SMTPConfig{
		Host:     c.SMTPHost,
		Port:     c.SMTPPort,
		Username: c.SMTPUsername,
		Password: c.SMTPPassword,
		From:     c.SMTPFrom,
		TLSMode:  email.TLSMode(c.SMTPTLSMode),
	}, logger)
}

// runSweeper runs the decay sweep on interval until ctx is cancelled. A
// non-positive interval disables it.
func runSweeper(ctx context.Context, s *decay.Sweeper, interval time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.Sweep(ctx); err != nil {
				logger.Error("decay sweep failed", "err", err)
			}
		}
	}
}

// VersionCmd prints the build version and exits.
type VersionCmd struct{}

// Run prints the version to stdout.
func (VersionCmd) Run() error {
	fmt.Println(version)
	return nil
}

// HealthCmd probes the server's /healthz over HTTP and exits non-zero if it is
// not reachable or not OK. It exists so the distroless runtime image (no shell,
// no curl) can still expose a container healthcheck against the binary itself.
type HealthCmd struct {
	URL string `name:"url" default:"http://127.0.0.1:8080/healthz" env:"YAADEGAR_HEALTH_URL" help:"Health endpoint to probe."`
}

// Run performs a single bounded GET and returns an error (non-zero exit) unless
// the endpoint answers 200.
func (c *HealthCmd) Run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: %s returned %d", c.URL, resp.StatusCode)
	}
	return nil
}

// newLogger builds a text/slog logger at the given level (defaulting to info for
// an unrecognized value; the CLI enum already constrains it).
func newLogger(level string) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: parseLevel(level)}))
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func main() {
	cli := &CLI{}
	kctx := kong.Parse(cli,
		kong.Name("yaadegar"),
		kong.Description("Yaadegar — a self-hosted, API-first gift registry."),
		kong.Configuration(kongyaml.Loader, "/etc/yaadegar/config.yaml", "./config.yaml"),
		kong.UsageOnError(),
	)
	kctx.FatalIfErrorf(kctx.Run(cli))
}
