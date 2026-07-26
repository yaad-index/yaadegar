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
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	kongyaml "github.com/alecthomas/kong-yaml"

	"github.com/yaad-index/yaadegar/internal/api"
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
}

// ServeCmd runs the HTTP server until interrupted.
type ServeCmd struct {
	HTTPAddr      string `name:"http-addr" default:":8080" env:"YAADEGAR_HTTP_ADDR" help:"HTTP listen address."`
	StorageDriver string `name:"storage-driver" default:"sqlite" enum:"sqlite,postgres" env:"YAADEGAR_STORAGE_DRIVER" help:"Storage driver."`
	StorageDSN    string `name:"storage-dsn" default:"file:yaadegar.db" env:"YAADEGAR_STORAGE_DSN" help:"Storage DSN: a SQLite file path/URI or a Postgres connection URL."`
	BaseDomain    string `name:"base-domain" env:"YAADEGAR_BASE_DOMAIN" help:"Host suffix under which tenant subdomains live (e.g. example.wish.list). Hosts outside it are treated as custom domains."`
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

	handler := api.NewHandler(store, api.Options{BaseDomain: c.BaseDomain, Logger: logger})
	return server.New(c.HTTPAddr, handler, logger).Run(ctx)
}

// VersionCmd prints the build version and exits.
type VersionCmd struct{}

// Run prints the version to stdout.
func (VersionCmd) Run() error {
	fmt.Println(version)
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
