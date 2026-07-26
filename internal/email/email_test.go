package email_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/email"
)

func TestLogSender(t *testing.T) {
	// A nil logger falls back to the default; Send always succeeds and is a no-op
	// beyond logging.
	s := email.NewLogSender(nil)
	require.NoError(t, s.Send(context.Background(), email.Message{
		To: "giver@example.com", Subject: "hi", Body: "body",
	}))
}

func TestFakeSender(t *testing.T) {
	var f email.FakeSender
	require.NoError(t, f.Send(context.Background(), email.Message{To: "a@example.com", Subject: "one"}))
	require.NoError(t, f.Send(context.Background(), email.Message{To: "b@example.com", Subject: "two"}))

	sent := f.Sent()
	require.Len(t, sent, 2)
	assert.Equal(t, "a@example.com", sent[0].To)
	assert.Equal(t, "two", sent[1].Subject)

	// Sent returns a copy — mutating it doesn't affect the recorder.
	sent[0].To = "mutated"
	assert.Equal(t, "a@example.com", f.Sent()[0].To)
}
