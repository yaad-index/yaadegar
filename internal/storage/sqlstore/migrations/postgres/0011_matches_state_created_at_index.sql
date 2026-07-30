-- Index supporting the co-buy match auto-expiry sweep (#101): the candidate read
-- filters matches by state = 'proposed' and orders by created_at, so a scan of the
-- proposed matches stays cheap as resolved (confirmed/declined/expired) matches
-- accumulate. Additive; no data change.
CREATE INDEX idx_matches_state_created_at ON matches (state, created_at);
