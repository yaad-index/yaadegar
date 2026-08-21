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
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	kongyaml "github.com/alecthomas/kong-yaml"

	"github.com/yaad-index/yaadegar/internal/api"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/captcha"
	"github.com/yaad-index/yaadegar/internal/clock"
	"github.com/yaad-index/yaadegar/internal/cobuy"
	"github.com/yaad-index/yaadegar/internal/decay"
	"github.com/yaad-index/yaadegar/internal/email"
	"github.com/yaad-index/yaadegar/internal/oauthlogin"
	"github.com/yaad-index/yaadegar/internal/server"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/storage/sqlstore"
)

// version is the release build version, set at link time via
// -ldflags "-X main.version=<semver>" only on a tagged release build (the publish
// pipeline). It is empty otherwise; a non-release build reports its embedded VCS
// commit instead, and a build with neither reports "unknown" — kept distinct from a
// release that silently failed to stamp, which must not look like an ordinary build
// (#225).
var version = ""

// resolveVersion decides what a build reports: the release semver when link-stamped,
// else the short VCS commit (with "-dirty" when the built tree was modified), else
// "unknown". Pure over its inputs so all three branches are unit-testable without a
// real build.
func resolveVersion(linkVersion string, settings []debug.BuildSetting) string {
	if linkVersion != "" {
		return linkVersion
	}
	var revision, modified string
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if revision == "" {
		return "unknown"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if modified == "true" {
		revision += "-dirty"
	}
	return revision
}

// reportedVersion resolves the running build's version from the link-time stamp and
// the binary's own embedded VCS metadata (debug.ReadBuildInfo).
func reportedVersion() string {
	var settings []debug.BuildSetting
	if info, ok := debug.ReadBuildInfo(); ok {
		settings = info.Settings
	}
	return resolveVersion(version, settings)
}

// CLI is the yaadegar command surface. Every config value resolves through
// file < env < flag.
type CLI struct {
	LogLevel string `name:"log-level" default:"info" enum:"debug,info,warn,error" env:"YAADEGAR_LOG_LEVEL" help:"Log verbosity."`

	Serve   ServeCmd   `cmd:"" help:"Run the HTTP API server."`
	Version VersionCmd `cmd:"" help:"Print the build version and exit."`
	Health  HealthCmd  `cmd:"" help:"Probe the server's health endpoint and exit non-zero if unhealthy (container healthcheck)."`

	// Seed / operator commands.
	CreateTenant      CreateTenantCmd      `cmd:"" name:"create-tenant" help:"Create a tenant."`
	CreateOwner       CreateOwnerCmd       `cmd:"" name:"create-owner" help:"Create an owner with a password credential in a tenant."`
	GrantAdmin        GrantAdminCmd        `cmd:"" name:"grant-admin" help:"Grant (or --revoke) the instance-admin capability on an existing owner (ADR-0010)."`
	EnableTenantOAuth EnableTenantOAuthCmd `cmd:"" name:"enable-tenant-oauth" help:"Turn Google login on (or --disable off) for a tenant (ADR-0008)."`
	HashPassword      HashPasswordCmd      `cmd:"" name:"hash-password" help:"Print the argon2id hash of a password (read from $YAADEGAR_PASSWORD or stdin)."`
	SetPassword       SetPasswordCmd       `cmd:"" name:"set-password" help:"Reset an existing user's password credential (argon2id), reading the new password from $YAADEGAR_PASSWORD or stdin."`
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

	// RegistrationPolicy gates unauthenticated self-registration (ADR-0009 Decision 2,
	// ADR-0012). Defaults to disabled — an existing instance keeps its unchanged
	// no-self-registration behavior until the operator opts in.
	RegistrationPolicy string `name:"registration-policy" default:"disabled" enum:"disabled,givers_only,owners_allowed" env:"YAADEGAR_REGISTRATION_POLICY" help:"Self-registration policy (ADR-0012): disabled (default, no self-registration) | givers_only (self-registered accounts are givers) | owners_allowed (self-registered accounts are owners)."`

	CobuyConfirmWindow time.Duration `name:"cobuy-confirm-window" default:"168h" env:"YAADEGAR_COBUY_CONFIRM_WINDOW" help:"How long a co-buying match's emailed confirm/decline link stays valid after the match is proposed (#96). 0 means it never expires."`

	DomainCNAMETarget string `name:"domain-cname-target" env:"YAADEGAR_DOMAIN_CNAME_TARGET" help:"Hostname that owners point a custom domain's CNAME at (returned by add-domain)."`

	DomainClaimTTL time.Duration `name:"domain-claim-ttl" default:"168h" env:"YAADEGAR_DOMAIN_CLAIM_TTL" help:"How long an unverified custom-domain claim holds its hostname before another tenant can reclaim it at add time (0 disables reclaiming). A verified domain is never reclaimed."`

	// Auth config (ADR-0005). The JWT secret is a secret and comes from the
	// environment only. At least one login method must be enabled and configured or
	// the instance refuses to start.
	AuthJWTSecret       string        `name:"auth-jwt-secret" env:"YAADEGAR_AUTH_JWT_SECRET" help:"JWT signing secret (HS256), >=32 bytes. Required; from the environment. The instance refuses to start if missing or too short."`
	AuthPasswordEnabled bool          `name:"auth-password-enabled" default:"true" env:"YAADEGAR_AUTH_PASSWORD_ENABLED" help:"Enable username+password login (the first login method; magic-link and OAuth land later)."`
	AuthAccessTTL       time.Duration `name:"auth-access-ttl" default:"12h" env:"YAADEGAR_AUTH_ACCESS_TTL" help:"Access-token lifetime; re-login on expiry (refresh tokens are a later cut)."`

	// Google OAuth owner login (ADR-0008, #21). All three are required to enable
	// Google login; the client secret is a secret and comes from the environment.
	// Any subset present but not all → the instance refuses to start (fail-closed);
	// none present → Google login is simply off. It is an add-on to password login,
	// gated per-tenant (see 'enable-tenant-oauth').
	OAuthGoogleClientID     string `name:"oauth-google-client-id" env:"YAADEGAR_OAUTH_GOOGLE_CLIENT_ID" help:"Google OAuth client id for owner login (#21). Set with the client secret and redirect base to enable Google login."`
	OAuthGoogleClientSecret string `name:"oauth-google-client-secret" env:"YAADEGAR_OAUTH_GOOGLE_CLIENT_SECRET" help:"Google OAuth client secret (provide via the environment)."`
	OAuthRedirectBase       string `name:"oauth-redirect-base" env:"YAADEGAR_OAUTH_REDIRECT_BASE" help:"Public https base URL of the fixed OAuth redirect host (e.g. https://yaadegar.example). The single registered redirect_uri is this base + /api/v1/auth/oauth/google/callback."`

	// Anti-bot captcha on the low-trust reserve tiers (ADR-0013, #45). none (default)
	// disables it (behaviour unchanged). A managed provider (turnstile|hcaptcha|
	// recaptcha) needs its verify secret and surfaces a public site key to the giver
	// widget; altcha is self-hosted proof-of-work (cut 2) that needs no site key and
	// reuses the secret as its HMAC signing key. An unknown provider, or a provider
	// with no secret, fails startup (fail-closed).
	CaptchaProvider string `name:"captcha-provider" default:"none" enum:"none,turnstile,hcaptcha,recaptcha,altcha" env:"YAADEGAR_CAPTCHA_PROVIDER" help:"Anti-bot captcha provider for low-trust reserve (ADR-0013): none (default, disabled) | turnstile | hcaptcha | recaptcha | altcha (self-hosted proof-of-work)."`
	CaptchaSecret   string `name:"captcha-secret" env:"YAADEGAR_CAPTCHA_SECRET" help:"Captcha secret: the managed-provider verify secret, or the HMAC signing key for altcha (server-side, never exposed). Required when --captcha-provider is not none."`
	CaptchaSiteKey  string `name:"captcha-site-key" env:"YAADEGAR_CAPTCHA_SITE_KEY" help:"Public captcha site key rendered by the managed-provider giver widget (safe to expose). Unused by altcha."`

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
		"version", reportedVersion(), "http_addr", c.HTTPAddr, "storage_driver", c.StorageDriver)

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

	// Google OAuth owner login (ADR-0008). Built after the auth service so the JWT
	// secret (reused to sign the state cookie + handoff ticket) is already validated.
	// A partial client config fails closed here; none configured leaves it nil (off).
	oauthAuth, err := buildOAuthAuthenticator(ctx, c, logger)
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

	// Fail closed on a bogus registration policy rather than silently disabling
	// self-registration (ADR-0012).
	registrationPolicy := storage.RegistrationPolicy(c.RegistrationPolicy)
	switch registrationPolicy {
	case storage.RegistrationDisabled, storage.RegistrationGiversOnly, storage.RegistrationOwnersAllowed:
	default:
		return fmt.Errorf("invalid --registration-policy %q (want disabled, givers_only, or owners_allowed)", c.RegistrationPolicy)
	}

	// Anti-bot captcha verifier (ADR-0013). none → nil (disabled, NoopVerifier); a
	// managed provider with no secret, or an unknown provider, fails startup here.
	captchaVerifier, err := captcha.New(c.CaptchaProvider, c.CaptchaSecret)
	if err != nil {
		return err
	}

	handler := api.NewHandler(store, api.Options{
		BaseDomain:          c.BaseDomain,
		Logger:              logger,
		Email:               sender,
		Auth:                authService,
		TrustForwardedHost:  c.TrustForwardedHost,
		LoginLimiter:        auth.NewInMemoryLimiter(c.LoginRateMaxFailures, c.LoginRateWindow, clock.Real{}),
		DomainCNAMETarget:   c.DomainCNAMETarget,
		DomainClaimTTL:      c.DomainClaimTTL,
		DefaultReserverTier: defaultTier,
		PublicLinkBase:      linkBase,
		CobuyConfirmWindow:  c.CobuyConfirmWindow,
		OAuth:               oauthAuth,
		RegistrationPolicy:  registrationPolicy,
		Captcha:             captchaVerifier,
		CaptchaProvider:     c.CaptchaProvider,
		CaptchaSiteKey:      c.CaptchaSiteKey,
		Version:             reportedVersion(),
	})

	// Run the reservation-decay sweeper on a ticker alongside the server.
	decaySweeper := decay.NewSweeper(store, sender, clock.Real{}, decay.Config{
		DefaultDecayDays: c.DecayDefaultDays,
		ResponseWindow:   c.DecayResponseWindow,
		ConfirmWindow:    c.ReserverConfirmWindow,
		LinkBase:         linkBase,
	}, logger)
	go runSweeper(ctx, "decay", decaySweeper, c.DecaySweepInterval, logger)

	// Run the co-buy match auto-expiry sweeper on the same ticker cadence (#101): it
	// dissolves proposed matches whose confirm window has elapsed, freeing the item.
	cobuySweeper := cobuy.NewSweeper(store, clock.Real{}, c.CobuyConfirmWindow, logger)
	go runSweeper(ctx, "cobuy-expiry", cobuySweeper, c.DecaySweepInterval, logger)

	return server.New(c.HTTPAddr, handler, logger).Run(ctx)
}

// oauthCallbackPath is the single callback path registered as the redirect_uri;
// it must match the route the API layer serves (ADR-0008 §2).
const oauthCallbackPath = "/api/v1/auth/oauth/google/callback"

// buildOAuthAuthenticator wires Google OAuth login when a full client config is
// present, nil when none is, and fails closed on a partial config (ADR-0008 §7).
// The state cookie and handoff ticket are signed with the validated JWT secret.
func buildOAuthAuthenticator(ctx context.Context, c *ServeCmd, logger *slog.Logger) (*oauthlogin.Authenticator, error) {
	id, secret, base := c.OAuthGoogleClientID, c.OAuthGoogleClientSecret, c.OAuthRedirectBase
	present := 0
	for _, v := range []string{id, secret, base} {
		if v != "" {
			present++
		}
	}
	switch present {
	case 0:
		return nil, nil // Google login is off
	case 3:
		// all present — build below
	default:
		return nil, errors.New(
			"google OAuth requires all of --oauth-google-client-id, --oauth-google-client-secret, and " +
				"--oauth-redirect-base (set none to disable); refusing to start")
	}
	redirectURL := strings.TrimRight(base, "/") + oauthCallbackPath
	a, err := oauthlogin.New(ctx, oauthlogin.Config{
		ClientID:     id,
		ClientSecret: secret,
		RedirectURL:  redirectURL,
		HMACKey:      []byte(c.AuthJWTSecret),
	})
	if err != nil {
		return nil, fmt.Errorf("configure google OAuth: %w", err)
	}
	logger.Info("google OAuth login enabled", "redirect_uri", redirectURL)
	return a, nil
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

// sweeper is one background sweep pass — the decay and co-buy-expiry sweepers both
// satisfy it, so they share the ticker loop below.
type sweeper interface {
	Sweep(ctx context.Context) error
}

// runSweeper runs a sweep on interval until ctx is cancelled. A non-positive
// interval disables it. name labels the sweep in error logs.
func runSweeper(ctx context.Context, name string, s sweeper, interval time.Duration, logger *slog.Logger) {
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
				logger.Error("sweep failed", "sweep", name, "err", err)
			}
		}
	}
}

// VersionCmd prints the build version and exits.
type VersionCmd struct{}

// Run prints the version to stdout.
func (VersionCmd) Run() error {
	fmt.Println(reportedVersion())
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
