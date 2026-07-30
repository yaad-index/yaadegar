package sqlstore

import (
	"context"
	"time"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// ExpiredMatchCandidates returns still-proposed matches created before `before`
// across all tenants — the input to the co-buy match auto-expiry sweep (#101). It
// is a trusted system-level read (the same unscoped class as DecayCandidates); the
// dissolution it drives is applied tenant-scoped via ForTenant + ExpireIfProposed.
//
// The state filter runs in SQL (index-backed by matches(state, created_at)); the
// `before` cutoff is applied in Go on the parsed timestamp rather than in SQL,
// because fmtTime stores RFC3339Nano — which omits the fractional part at zero
// nanoseconds — so a string `created_at < ?` comparison is not lexicographically
// safe across the sub-second boundary. This mirrors the domain-reclaim cutoff,
// which likewise compares parsed times in Go.
func (s *sqlStore) ExpiredMatchCandidates(ctx context.Context, before time.Time) ([]storage.MatchExpiryCandidate, error) {
	rows, err := s.db.QueryContext(ctx, s.d.rebind(
		`SELECT tenant_id, id, item_id, created_at FROM matches
		  WHERE state = ?
		  ORDER BY tenant_id, id`),
		string(storage.MatchProposed))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []storage.MatchExpiryCandidate
	for rows.Next() {
		var (
			c         storage.MatchExpiryCandidate
			createdAt string
		)
		if err := rows.Scan(&c.TenantID, &c.MatchID, &c.ItemID, &createdAt); err != nil {
			return nil, err
		}
		ts, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		if !ts.Before(before) {
			continue
		}
		c.CreatedAt = ts
		out = append(out, c)
	}
	return out, rows.Err()
}
