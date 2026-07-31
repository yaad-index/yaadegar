package oauthlogin_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/clock"
	"github.com/yaad-index/yaadegar/internal/oauthlogin"
)

func TestSigner_StateRoundTrip(t *testing.T) {
	s := oauthlogin.NewSigner([]byte("a-signing-key-of-adequate-length-01"))
	in := oauthlogin.StatePayload{
		State: "st", Nonce: "no", PKCEVerifier: "vf",
		TenantID: "t1", TenantHost: "alice.example.test", ReturnTo: "/dash", Exp: 123,
	}
	out, err := s.VerifyState(s.SignState(in))
	require.NoError(t, err)
	assert.Equal(t, in.State, out.State)
	assert.Equal(t, in.TenantID, out.TenantID)
	assert.Equal(t, in.TenantHost, out.TenantHost)
	assert.Equal(t, in.ReturnTo, out.ReturnTo)
	assert.Equal(t, in.Exp, out.Exp)
}

func TestSigner_TicketRoundTrip(t *testing.T) {
	s := oauthlogin.NewSigner([]byte("a-signing-key-of-adequate-length-01"))
	in := oauthlogin.Ticket{TenantID: "t1", UserID: "u1", ReturnTo: "/", Jti: "j1", Exp: 456}
	out, err := s.VerifyTicket(s.SignTicket(in))
	require.NoError(t, err)
	assert.Equal(t, in, structWithoutTyp(out))
}

// structWithoutTyp zeroes the internal Typ field so an equality check on the
// caller-supplied fields is clean (Typ is stamped by SignTicket).
func structWithoutTyp(tk oauthlogin.Ticket) oauthlogin.Ticket {
	tk.Typ = ""
	return tk
}

func TestSigner_RejectsTamperedPayload(t *testing.T) {
	s := oauthlogin.NewSigner([]byte("a-signing-key-of-adequate-length-01"))
	tok := s.SignState(oauthlogin.StatePayload{TenantID: "t1"})
	// Flip a character in the payload segment.
	tampered := "X" + tok[1:]
	_, err := s.VerifyState(tampered)
	assert.ErrorIs(t, err, oauthlogin.ErrBadSignature)
}

func TestSigner_RejectsWrongKey(t *testing.T) {
	a := oauthlogin.NewSigner([]byte("key-A-of-adequate-length-0123456789"))
	b := oauthlogin.NewSigner([]byte("key-B-of-adequate-length-0123456789"))
	tok := a.SignTicket(oauthlogin.Ticket{UserID: "u1"})
	_, err := b.VerifyTicket(tok)
	assert.ErrorIs(t, err, oauthlogin.ErrBadSignature)
}

// A state blob must not verify as a ticket, or vice versa (domain separation via
// the typ field) — so neither can be replayed in the other role.
func TestSigner_TypeSeparation(t *testing.T) {
	s := oauthlogin.NewSigner([]byte("a-signing-key-of-adequate-length-01"))
	stateTok := s.SignState(oauthlogin.StatePayload{TenantID: "t1"})
	_, err := s.VerifyTicket(stateTok)
	assert.ErrorIs(t, err, oauthlogin.ErrBadSignature)

	ticketTok := s.SignTicket(oauthlogin.Ticket{TenantID: "t1"})
	_, err = s.VerifyState(ticketTok)
	assert.ErrorIs(t, err, oauthlogin.ErrBadSignature)
}

func TestTicketGuard_SingleUse(t *testing.T) {
	clk := clock.NewFake(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	g := oauthlogin.NewInMemoryTicketGuard(clk)
	exp := clk.Now().Add(time.Minute)

	assert.True(t, g.Consume("jti-1", exp), "first use wins")
	assert.False(t, g.Consume("jti-1", exp), "replay is rejected")
	assert.True(t, g.Consume("jti-2", exp), "a distinct jti is independent")
}

func TestTicketGuard_EvictsExpired(t *testing.T) {
	clk := clock.NewFake(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	g := oauthlogin.NewInMemoryTicketGuard(clk)
	exp := clk.Now().Add(time.Minute)
	require.True(t, g.Consume("jti-1", exp))

	// After the jti expires it is evicted; the same value can be consumed afresh
	// (it could no longer be presented with a valid signature anyway).
	clk.Advance(2 * time.Minute)
	assert.True(t, g.Consume("jti-1", clk.Now().Add(time.Minute)),
		"an expired jti is forgotten, not retained forever")
}
