// Package email is the outbound-email seam. Real SMTP is a later issue; this
// interface plus a logging sender keep delivery observable and testable now. A
// send must never fail silently — callers log any error, and the default sender
// records every message.
package email

import (
	"context"
	"log/slog"
	"sync"
)

// Message is one outbound email.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Sender delivers messages. Implementations must surface failures (return an
// error and/or log), never swallow them.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// LogSender logs each message and reports success. It exists so a running
// instance has observable, non-silent email until real SMTP lands.
type LogSender struct{ logger *slog.Logger }

// NewLogSender returns a LogSender writing to logger (slog.Default if nil).
func NewLogSender(logger *slog.Logger) *LogSender {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogSender{logger: logger}
}

// Send logs the message envelope. The body is intentionally not logged (it may
// carry a co-buyer's revealed contact).
func (s *LogSender) Send(ctx context.Context, m Message) error {
	s.logger.InfoContext(ctx, "email sent", "to", m.To, "subject", m.Subject)
	return nil
}

// FakeSender records sent messages for tests. Safe for concurrent use.
type FakeSender struct {
	mu   sync.Mutex
	sent []Message
}

// Send records m.
func (f *FakeSender) Send(_ context.Context, m Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

// Sent returns a copy of the recorded messages.
func (f *FakeSender) Sent() []Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Message, len(f.sent))
	copy(out, f.sent)
	return out
}
