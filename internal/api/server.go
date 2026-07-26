package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// Server implements the generated strict server interface against the storage
// layer.
type Server struct {
	store      storage.Store
	baseDomain string
	logger     *slog.Logger
}

var _ gen.StrictServerInterface = (*Server)(nil)

// Options configures the API handler.
type Options struct {
	// BaseDomain is the host suffix under which tenant subdomains live
	// (e.g. "example.wish.list" → "alice.example.wish.list"). Hosts that are not
	// under it are treated as custom domains.
	BaseDomain string
	Logger     *slog.Logger
}

// NewHandler builds the full HTTP handler: the generated strict router wrapped in
// tenant-resolution and owner-auth middleware. It serves both surfaces and
// /healthz.
func NewHandler(store storage.Store, opts Options) http.Handler {
	s := &Server{store: store, baseDomain: opts.BaseDomain, logger: opts.Logger}
	if s.logger == nil {
		s.logger = slog.Default()
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

	// Middleware order (outermost first): resolve tenant, enforce owner auth, then
	// lift any capability token into context for the giver handlers.
	var h http.Handler = mux
	h = captureCapabilityToken(h)
	h = s.requireOwner(h)
	h = s.resolveTenant(h)
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
