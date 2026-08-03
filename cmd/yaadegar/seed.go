package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/storage/sqlstore"
)

// HashPasswordCmd prints the argon2id hash of a password (the same hash the login
// flow verifies against). The password is read from $YAADEGAR_PASSWORD or, if unset,
// from stdin — never a flag, so it does not land in shell history or the process list.
type HashPasswordCmd struct{}

// Run reads the password and prints its argon2id hash.
func (c *HashPasswordCmd) Run() error {
	pw, err := readPassword()
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	fmt.Println(hash)
	return nil
}

// readPassword reads a password from $YAADEGAR_PASSWORD or, if unset, from stdin —
// never a flag, so it stays out of shell history and the process list. Shared by the
// hash-password and set-password commands. Returns an error if none is provided.
func readPassword() (string, error) {
	pw := os.Getenv("YAADEGAR_PASSWORD")
	if pw == "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read password from stdin: %w", err)
		}
		pw = strings.TrimRight(string(data), "\r\n")
	}
	if pw == "" {
		return "", errors.New("no password provided — set YAADEGAR_PASSWORD or pipe the password on stdin")
	}
	return pw, nil
}

// SetPasswordCmd resets an existing user's password, replacing their argon2id
// credential via the same hashing path as create-owner (#141). It is the recovery
// path for an owner/admin lockout: resolve the user by tenant subdomain + login
// handle (like grant-admin) and update the credential in place — no schema change.
// The new password is read from $YAADEGAR_PASSWORD or stdin, never a flag.
type SetPasswordCmd struct {
	storageFlags
	Tenant   string `name:"tenant" required:"" help:"Subdomain of the tenant the user lives in."`
	Username string `name:"username" required:"" help:"Login handle of the user whose password to reset."`
}

// Run reads the new password, resolves the tenant + user, and replaces the credential.
func (c *SetPasswordCmd) Run() error {
	pw, err := readPassword()
	if err != nil {
		return err
	}
	ctx := context.Background()
	store, err := c.open(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	tenant, err := store.TenantBySubdomain(ctx, c.Tenant)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("no tenant with subdomain %q — create it first with create-tenant", c.Tenant)
		}
		return fmt.Errorf("resolve tenant: %w", err)
	}
	ts := store.ForTenant(tenant)
	user, err := ts.Users().ByUsername(ctx, c.Username)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("no user with username %q in tenant %q", c.Username, c.Tenant)
		}
		return fmt.Errorf("resolve user: %w", err)
	}
	hash, err := auth.HashNewPassword(pw)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if err := ts.Users().SetPasswordHash(ctx, user.ID, hash); err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	fmt.Printf("password updated for %q (%s) in tenant %q\n", c.Username, user.ID, tenant.Subdomain)
	return nil
}

// storageFlags are the storage connection flags shared by the seed commands
// (the same env vars ServeCmd reads, so `docker compose run` picks up the compose
// environment automatically).
type storageFlags struct {
	StorageDriver string `name:"storage-driver" default:"sqlite" enum:"sqlite,postgres" env:"YAADEGAR_STORAGE_DRIVER" help:"Storage driver."`
	StorageDSN    string `name:"storage-dsn" default:"file:yaadegar.db" env:"YAADEGAR_STORAGE_DSN" help:"Storage DSN: a SQLite file path/URI or a Postgres connection URL."`
}

// open opens the store and applies migrations, so a seed command works against a
// fresh database as well as an existing one.
func (f storageFlags) open(ctx context.Context) (storage.Store, error) {
	store, err := sqlstore.Open(ctx, storage.Config{
		Driver: storage.Driver(f.StorageDriver),
		DSN:    f.StorageDSN,
	})
	if err != nil {
		return nil, fmt.Errorf("open storage: %w", err)
	}
	if err := store.Migrate(ctx); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("migrate storage: %w", err)
	}
	return store, nil
}

// CreateTenantCmd seeds a tenant. Owner self-registration is still deferred, so this
// (with create-owner, then grant-admin for the first admin) is how a local instance
// gets its first tenant + login for hands-on testing.
type CreateTenantCmd struct {
	storageFlags
	Subdomain string `name:"subdomain" required:"" help:"Tenant subdomain (lowercase slug), addressed as <subdomain>.<base-domain>."`
}

// Run creates the tenant and prints its id.
func (c *CreateTenantCmd) Run() error {
	ctx := context.Background()
	store, err := c.open(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	tenant, err := store.CreateTenant(ctx, storage.Tenant{Subdomain: c.Subdomain})
	if err != nil {
		return fmt.Errorf("create tenant: %w", err)
	}
	fmt.Printf("created tenant %s (subdomain %q)\n", tenant.ID, tenant.Subdomain)
	return nil
}

// CreateOwnerCmd seeds an owner with a password credential in an existing tenant,
// hashing the password with the same argon2id path the login flow verifies against.
type CreateOwnerCmd struct {
	storageFlags
	Tenant   string `name:"tenant" required:"" help:"Subdomain of the tenant to create the owner in."`
	Username string `name:"username" required:"" help:"Login handle (unique within the tenant)."`
	Password string `name:"password" required:"" help:"Password (dev/local use); stored only as an argon2id hash."`
	Email    string `name:"email" help:"Owner email (optional; server-side use)."`
	Name     string `name:"name" help:"Display name (defaults to the username)."`
}

// Run resolves the tenant by subdomain and creates the credentialed owner.
func (c *CreateOwnerCmd) Run() error {
	ctx := context.Background()
	store, err := c.open(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	tenant, err := store.TenantBySubdomain(ctx, c.Tenant)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("no tenant with subdomain %q — create it first with create-tenant", c.Tenant)
		}
		return fmt.Errorf("resolve tenant: %w", err)
	}

	hash, err := auth.HashNewPassword(c.Password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	name := c.Name
	if name == "" {
		name = c.Username
	}
	username := c.Username
	user, err := store.ForTenant(tenant).Users().Create(ctx, storage.User{
		Name:         name,
		Email:        c.Email,
		Username:     &username,
		PasswordHash: hash,
	})
	if err != nil {
		return fmt.Errorf("create owner: %w", err)
	}
	fmt.Printf("created owner %s (username %q) in tenant %q — log in at %s.<base-domain>\n",
		user.ID, username, tenant.Subdomain, tenant.Subdomain)
	return nil
}

// GrantAdminCmd grants (or --revoke) the instance-admin capability on an existing
// owner, resolved by tenant subdomain + login handle (ADR-0010). This is the
// headless bootstrap for the admin surface: there is no separate admin credential —
// the granted owner reaches /admin with their ordinary owner session. A fresh
// instance provisions its first admin with create-tenant → create-owner → grant-admin.
type GrantAdminCmd struct {
	storageFlags
	Tenant   string `name:"tenant" required:"" help:"Subdomain of the tenant the owner lives in."`
	Username string `name:"username" required:"" help:"Login handle of the owner to grant/revoke the admin capability on."`
	Revoke   bool   `name:"revoke" help:"Revoke the admin capability instead of granting it."`
}

// Run resolves the tenant and user, then flips the is_admin capability.
func (c *GrantAdminCmd) Run() error {
	ctx := context.Background()
	store, err := c.open(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	tenant, err := store.TenantBySubdomain(ctx, c.Tenant)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("no tenant with subdomain %q — create it first with create-tenant", c.Tenant)
		}
		return fmt.Errorf("resolve tenant: %w", err)
	}
	ts := store.ForTenant(tenant)
	user, err := ts.Users().ByUsername(ctx, c.Username)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("no user with username %q in tenant %q", c.Username, c.Tenant)
		}
		return fmt.Errorf("resolve user: %w", err)
	}
	grant := !c.Revoke
	if err := ts.Users().SetAdmin(ctx, user.ID, grant); err != nil {
		return fmt.Errorf("set admin capability: %w", err)
	}
	state := "granted"
	if !grant {
		state = "revoked"
	}
	fmt.Printf("instance-admin capability %s for %q (%s) in tenant %q\n", state, c.Username, user.ID, tenant.Subdomain)
	return nil
}

// EnableTenantOAuthCmd flips a tenant's Google-login toggle (ADR-0008 §6). Google
// login stays inert until BOTH this per-tenant toggle is on AND the instance has a
// configured Google client (the --oauth-google-* env config). The owner-settings
// UI for this toggle lands with the frontend cut; this is the operator/demo path.
type EnableTenantOAuthCmd struct {
	storageFlags
	Tenant  string `name:"tenant" required:"" help:"Subdomain of the tenant to toggle."`
	Disable bool   `name:"disable" help:"Turn Google login OFF for the tenant instead of on."`
}

// Run resolves the tenant by subdomain and sets its Google-login toggle.
func (c *EnableTenantOAuthCmd) Run() error {
	ctx := context.Background()
	store, err := c.open(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	tenant, err := store.TenantBySubdomain(ctx, c.Tenant)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("no tenant with subdomain %q — create it first with create-tenant", c.Tenant)
		}
		return fmt.Errorf("resolve tenant: %w", err)
	}
	enabled := !c.Disable
	if err := store.SetTenantOAuthGoogle(ctx, tenant.ID, enabled); err != nil {
		return fmt.Errorf("set google login toggle: %w", err)
	}
	state := "enabled"
	if !enabled {
		state = "disabled"
	}
	fmt.Printf("google login %s for tenant %q (%s)\n", state, tenant.Subdomain, tenant.ID)
	return nil
}
