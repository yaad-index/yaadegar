package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/captcha"
	"github.com/yaad-index/yaadegar/internal/clock"
	"github.com/yaad-index/yaadegar/internal/email"
	"github.com/yaad-index/yaadegar/internal/oauthlogin"
	"github.com/yaad-index/yaadegar/internal/preview"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// Server implements the generated strict server interface against the storage
// layer.
type Server struct {
	store              storage.Store
	baseDomain         string
	email              email.Sender
	clock              clock.Clock
	previewer          *preview.Previewer
	resolver           Resolver
	auth               *auth.Service
	trustForwardedHost bool
	loginLimiter       auth.Limiter
	domainCNAMETarget  string
	domainClaimTTL     time.Duration
	// defaultReserverTier is the instance-wide reserver tier a list inherits when
	// it sets no per-list override (ADR-0007). Empty behaves as full_guest.
	defaultReserverTier storage.ReserverTier
	// publicLinkBase is the base URL of the giver-facing site, used to build the
	// email_confirmed confirmation link. Empty yields a relative link.
	publicLinkBase string
	// cobuyConfirmWindow is how long a scoped match-action token stays valid after a
	// match is proposed (#96). Non-positive means the token never expires.
	cobuyConfirmWindow time.Duration
	// oauth is the OIDC login client (ADR-0008). nil when no Google client is
	// configured, in which case every OAuth endpoint reports 404 (the method is
	// absent, not a failure).
	oauth *oauthlogin.Authenticator
	// ticketGuard enforces one-time use of the cross-host handoff ticket.
	ticketGuard oauthlogin.TicketGuard
	// registrationPolicy gates the unauthenticated self-registration endpoint
	// (ADR-0012). Empty behaves as RegistrationDisabled (fail-closed): the register
	// endpoint answers 403 until the operator opts in.
	registrationPolicy storage.RegistrationPolicy
	// captcha verifies the human-challenge token on the low-trust surfaces — the
	// register path (ADR-0012) and the low-trust reserve tiers (ADR-0013). It
	// defaults to a no-op verifier (accepts every token) when nil.
	captcha captcha.Verifier
	// captchaEnabled is true when a real verifier was configured (Options.Captcha
	// non-nil), so the reserve gate knows to require a token; false leaves the
	// low-trust reserve paths untouched (ADR-0013 §2).
	captchaEnabled bool
	// captchaProvider and captchaSiteKey are the public-safe identifiers surfaced to
	// the giver page so it can load the right provider SDK and render the widget
	// (ADR-0013 §5). Empty/"none" when captcha is disabled.
	captchaProvider string
	captchaSiteKey  string
	logger          *slog.Logger
}

var _ gen.StrictServerInterface = (*Server)(nil)

// Options configures the API handler.
type Options struct {
	// BaseDomain is the host suffix under which tenant subdomains live
	// (e.g. "example.wish.list" → "alice.example.wish.list"). Hosts that are not
	// under it are treated as custom domains.
	BaseDomain string
	Logger     *slog.Logger
	// Email delivers co-buying handshake notifications. Defaults to a logging
	// sender (observable, never silent) when nil.
	Email email.Sender
	// Clock supplies "now" for time-gated checks (event-dated auto-disable).
	// Defaults to the real clock when nil.
	Clock clock.Clock
	// Previewer scrapes item drafts from product URLs. Defaults to the
	// SSRF-guarded fetcher when nil; tests inject one over a fake fetcher.
	Previewer *preview.Previewer
	// Resolver does DNS TXT lookups for custom-domain verification. Defaults to
	// the system resolver when nil; tests inject a fake.
	Resolver Resolver
	// Auth is the validated authentication core (ADR-0005): the owner-auth
	// middleware and the login handler use it. It is required — NewHandler panics
	// if nil, since the owner surface must never fall open (the fail-closed
	// construction lives in NewService, called at startup).
	Auth *auth.Service
	// LoginLimiter throttles brute-force login attempts (owner + admin). Defaults
	// to a no-op limiter (no limiting) when nil.
	LoginLimiter auth.Limiter
	// TrustForwardedHost enables X-Forwarded-Host for tenant resolution (ADR-0004
	// §7). DEFAULT FALSE — enable ONLY when the backend is reachable exclusively
	// behind the trusted frontend proxy; a directly-exposed backend must keep it
	// off, since the header is client-settable and would otherwise let any caller
	// spoof any tenant.
	TrustForwardedHost bool
	// DomainCNAMETarget is the hostname owners point their custom domain's CNAME
	// at; returned by addDomain.
	DomainCNAMETarget string
	// DomainClaimTTL is how long an unverified custom-domain claim holds its
	// hostname before another tenant can reclaim it at add time (ADR-0004 §4). A
	// verified domain is never reclaimed. Zero disables reclaiming.
	DomainClaimTTL time.Duration
	// DefaultReserverTier is the instance-wide reserver tier lists inherit absent a
	// per-list override (ADR-0007). Empty behaves as full_guest.
	DefaultReserverTier storage.ReserverTier
	// PublicLinkBase is the base URL of the giver-facing site, used to build the
	// email_confirmed confirmation link (mirrors the decay link base). Empty
	// yields a relative link.
	PublicLinkBase string
	// CobuyConfirmWindow is how long a scoped match-action token stays valid after
	// a match is proposed (#96). Non-positive means it never expires.
	CobuyConfirmWindow time.Duration
	// OAuth is the OIDC owner-login client (ADR-0008). nil disables Google login
	// (the endpoints report 404). Built at startup from the env config once the
	// three client fields are present; a partial config fails startup, not here.
	OAuth *oauthlogin.Authenticator
	// TicketGuard enforces one-time use of the OAuth cross-host ticket. Defaults to
	// an in-memory guard when nil.
	TicketGuard oauthlogin.TicketGuard
	// RegistrationPolicy gates unauthenticated self-registration (ADR-0012 Decision
	// 2). Empty behaves as RegistrationDisabled (fail-closed): existing instances keep
	// their unchanged no-self-registration behavior until the operator opts in.
	RegistrationPolicy storage.RegistrationPolicy
	// Captcha verifies the human-challenge token on the low-trust surfaces — the
	// self-registration path (ADR-0012) and the low-trust reserve tiers (ADR-0013).
	// Defaults to a no-op verifier (accepts every token) when nil, so those paths are
	// wired and testable and disabled instances are unchanged. A non-nil verifier
	// turns the reserve-path gate on.
	Captcha captcha.Verifier
	// CaptchaProvider and CaptchaSiteKey are the public-safe captcha identifiers
	// surfaced to the giver page (ADR-0013 §5): the provider name selects the widget
	// SDK and the site key renders it. Empty when captcha is disabled.
	CaptchaProvider string
	CaptchaSiteKey  string
}

// NewHandler builds the full HTTP handler: the generated strict router wrapped in
// tenant-resolution and owner-auth middleware. It serves both surfaces and
// /healthz.
func NewHandler(store storage.Store, opts Options) http.Handler {
	if opts.Auth == nil {
		panic("api: Options.Auth is required (owner surface must not fall open)")
	}
	s := &Server{
		store:               store,
		baseDomain:          opts.BaseDomain,
		email:               opts.Email,
		clock:               opts.Clock,
		previewer:           opts.Previewer,
		resolver:            opts.Resolver,
		auth:                opts.Auth,
		trustForwardedHost:  opts.TrustForwardedHost,
		loginLimiter:        opts.LoginLimiter,
		domainCNAMETarget:   opts.DomainCNAMETarget,
		domainClaimTTL:      opts.DomainClaimTTL,
		defaultReserverTier: opts.DefaultReserverTier,
		publicLinkBase:      opts.PublicLinkBase,
		cobuyConfirmWindow:  opts.CobuyConfirmWindow,
		oauth:               opts.OAuth,
		ticketGuard:         opts.TicketGuard,
		registrationPolicy:  opts.RegistrationPolicy,
		captcha:             opts.Captcha,
		captchaEnabled:      opts.Captcha != nil,
		captchaProvider:     opts.CaptchaProvider,
		captchaSiteKey:      opts.CaptchaSiteKey,
		logger:              opts.Logger,
	}
	if s.captcha == nil {
		s.captcha = captcha.NoopVerifier{}
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.loginLimiter == nil {
		s.loginLimiter = auth.NoopLimiter{}
	}
	if s.email == nil {
		s.email = email.NewLogSender(s.logger)
	}
	if s.clock == nil {
		s.clock = clock.Real{}
	}
	if s.ticketGuard == nil {
		// Share the server clock so ticket expiry/eviction is testable with a fake.
		s.ticketGuard = oauthlogin.NewInMemoryTicketGuard(s.clock)
	}
	if s.previewer == nil {
		s.previewer = preview.NewDefault()
	}
	if s.resolver == nil {
		s.resolver = newNetResolver()
	}

	strict := gen.NewStrictHandlerWithOptions(s, nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			// Malformed path/query/body before the handler runs.
			writeProblem(w, http.StatusBadRequest, err.Error())
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, errNotImplemented) {
				writeProblem(w, http.StatusNotImplemented, "this endpoint is not implemented yet")
				return
			}
			s.logger.Error("handler error", "err", err, "path", r.URL.Path)
			writeProblem(w, http.StatusInternalServerError, "internal error")
		},
	})

	mux := http.NewServeMux()
	gen.HandlerFromMux(strict, mux)
	// Owner list export (#26) is a raw handler on the same mux — file download with
	// Content-Disposition is a poor fit for the JSON strict server. It sits inside the
	// middleware chain below, so tenant + owner auth are identical to the typed routes.
	mux.HandleFunc("GET /api/v1/lists/{listId}/export", s.handleListExport)
	mux.HandleFunc("POST /api/v1/lists/{listId}/import", s.handleListImport)
	// Owner login via Google OAuth/OIDC (#21, ADR-0008): browser-facing redirect
	// endpoints, a poor fit for the JSON strict server. They are always registered;
	// when no Google client is configured each reports 404 (the method is absent).
	// The tenant-resolution and owner-auth middleware skip /api/v1/auth/oauth/* —
	// each endpoint carries its own tenant in the signed state, then the ticket.
	mux.HandleFunc("GET "+oauthStartPath, s.handleOAuthStart)
	mux.HandleFunc("GET "+oauthCallbackPath, s.handleOAuthCallback)
	mux.HandleFunc("GET "+oauthCompletePath, s.handleOAuthComplete)

	// Middleware order (outermost first): resolve tenant (skips /admin + /healthz),
	// enforce owner auth on /api/v1, enforce the instance-admin capability on /admin
	// (ADR-0010), then lift any capability token into context for the giver handlers.
	var h http.Handler = mux
	h = captureCapabilityToken(h)
	h = s.requireAdmin(h)
	h = s.requireOwner(h)
	h = s.resolveTenant(h)
	h = captureClientIP(h)
	return h
}

// errMissingContext is a defensive 500: the middleware should always populate the
// tenant (and, on the owner surface, the owner) before a handler runs.
var errMissingContext = errors.New("request context missing tenant or owner")

// tenantStore returns the tenant-scoped store for the request's resolved tenant.
func (s *Server) tenantStore(ctx context.Context) (storage.TenantStore, storage.Tenant, bool) {
	t, ok := tenantFromContext(ctx)
	if !ok {
		return nil, storage.Tenant{}, false
	}
	return s.store.ForTenant(t), t, true
}

// derefOr returns *p, or def when p is nil.
func derefOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

// pageParams applies the spec's pagination defaults and bounds (limit 1..200,
// default 50; offset >= 0, default 0).
func pageParams(limit, offset *int) storage.Page {
	l := derefOr(limit, 50)
	switch {
	case l < 1:
		l = 1
	case l > 200:
		l = 200
	}
	o := derefOr(offset, 0)
	if o < 0 {
		o = 0
	}
	return storage.Page{Limit: l, Offset: o}
}
