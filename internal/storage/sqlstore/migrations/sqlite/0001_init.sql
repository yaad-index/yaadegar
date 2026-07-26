-- Yaadegar initial schema (ADR-0002 entities, ADR-0003 storage model).
-- Every domain table carries tenant_id; every repository query is scoped to it.
-- Portable types only: TEXT ids/slugs/timestamps, INTEGER money/counts/bools.

CREATE TABLE tenants (
    id         TEXT PRIMARY KEY,
    subdomain  TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL
);

CREATE TABLE users (
    id         TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_users_tenant ON users (tenant_id);

CREATE TABLE lists (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    owner_id    TEXT NOT NULL,
    title       TEXT NOT NULL,
    visibility  TEXT NOT NULL,
    share_slug  TEXT NOT NULL,
    event_date  TEXT,
    decay_days  INTEGER NOT NULL,
    active      INTEGER NOT NULL,
    created_at  TEXT NOT NULL,
    UNIQUE (tenant_id, share_slug)
);
CREATE INDEX idx_lists_tenant_owner ON lists (tenant_id, owner_id);

CREATE TABLE items (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL,
    list_id            TEXT NOT NULL,
    name               TEXT NOT NULL,
    url                TEXT,
    image_url          TEXT,
    price_amount_minor INTEGER,
    price_currency     TEXT,
    note               TEXT,
    priority           INTEGER NOT NULL,
    quantity_wanted    INTEGER NOT NULL,
    created_at         TEXT NOT NULL
);
CREATE INDEX idx_items_tenant_list ON items (tenant_id, list_id);

CREATE TABLE reservations (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    item_id     TEXT NOT NULL,
    giver_name  TEXT,
    giver_email TEXT,
    quantity    INTEGER NOT NULL,
    token_hash  TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    UNIQUE (tenant_id, token_hash)
);
CREATE INDEX idx_reservations_tenant_item ON reservations (tenant_id, item_id);

CREATE TABLE contributions (
    id                   TEXT PRIMARY KEY,
    tenant_id            TEXT NOT NULL,
    item_id              TEXT NOT NULL,
    pledged_amount_minor INTEGER NOT NULL,
    pledged_currency     TEXT NOT NULL,
    giver_name           TEXT,
    contact_email        TEXT NOT NULL,
    status               TEXT NOT NULL,
    match_id             TEXT,
    token_hash           TEXT NOT NULL,
    created_at           TEXT NOT NULL,
    UNIQUE (tenant_id, token_hash)
);
CREATE INDEX idx_contributions_tenant_item ON contributions (tenant_id, item_id);

CREATE TABLE matches (
    id         TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL,
    item_id    TEXT NOT NULL,
    state      TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_matches_tenant_item ON matches (tenant_id, item_id);

CREATE TABLE match_contributions (
    tenant_id       TEXT NOT NULL,
    match_id        TEXT NOT NULL,
    contribution_id TEXT NOT NULL,
    PRIMARY KEY (match_id, contribution_id)
);

CREATE TABLE domains (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    hostname     TEXT NOT NULL UNIQUE,
    cname_target TEXT NOT NULL,
    verified     INTEGER NOT NULL,
    tls_status   TEXT NOT NULL,
    created_at   TEXT NOT NULL
);
CREATE INDEX idx_domains_tenant ON domains (tenant_id);
