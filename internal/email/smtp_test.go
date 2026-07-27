package email

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSMTPSenderValidation(t *testing.T) {
	t.Run("host required", func(t *testing.T) {
		_, err := NewSMTPSender(SMTPConfig{From: "a@example.com"}, nil)
		require.Error(t, err)
	})
	t.Run("from required", func(t *testing.T) {
		_, err := NewSMTPSender(SMTPConfig{Host: "smtp.example.com"}, nil)
		require.Error(t, err)
	})
	t.Run("unknown TLS mode rejected", func(t *testing.T) {
		_, err := NewSMTPSender(SMTPConfig{Host: "smtp.example.com", From: "a@example.com", TLSMode: "bogus"}, nil)
		require.Error(t, err)
	})
	t.Run("defaults", func(t *testing.T) {
		s, err := NewSMTPSender(SMTPConfig{Host: "smtp.example.com", From: "a@example.com"}, nil)
		require.NoError(t, err)
		assert.Equal(t, 587, s.cfg.Port)
		assert.Equal(t, TLSStartTLS, s.cfg.TLSMode)
	})
}

// TestDialGuardLoopback is the TLS-required unit: plaintext (TLSNone) is allowed
// only when the RESOLVED dial IP is loopback, so a hostname that resolves off-box
// can never leak credentials/tokens in the clear.
func TestDialGuardLoopback(t *testing.T) {
	allow := []string{"127.0.0.1:25", "127.0.0.5:587", "[::1]:465", "[::ffff:127.0.0.1]:25"}
	for _, addr := range allow {
		require.NoError(t, dialGuardLoopback("tcp", addr, nil), "expected loopback %q allowed", addr)
	}
	refuse := []string{"8.8.8.8:25", "192.168.1.10:587", "10.0.0.1:25", "[2001:4860:4860::8888]:25", "not-an-address"}
	for _, addr := range refuse {
		require.Error(t, dialGuardLoopback("tcp", addr, nil), "expected non-loopback %q refused", addr)
	}
}

// TestPlaintextNonLoopbackRefused exercises the guard end-to-end: a TLSNone send
// to a non-loopback IP literal is refused at dial time (the Control hook fires
// before any bytes leave the box), so plaintext creds never go out.
func TestPlaintextNonLoopbackRefused(t *testing.T) {
	s, err := NewSMTPSender(SMTPConfig{
		Host:     "8.8.8.8",
		Port:     2525,
		Username: "user",
		Password: "secret",
		From:     "noreply@example.com",
		TLSMode:  TLSNone,
	}, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = s.Send(ctx, Message{To: "giver@example.com", Subject: "hi", Body: "b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-loopback")
}

func TestBuildMessage(t *testing.T) {
	msg := buildMessage("noreply@example.com", Message{
		To:      "giver@example.com",
		Subject: "Still planning to buy your reserved gift?",
		Body:    "line one\nline two",
	})
	s := string(msg)

	assert.Contains(t, s, "From: noreply@example.com\r\n")
	assert.Contains(t, s, "To: giver@example.com\r\n")
	assert.Contains(t, s, "Subject: Still planning to buy your reserved gift?\r\n")
	assert.Contains(t, s, "MIME-Version: 1.0\r\n")
	assert.Contains(t, s, "Content-Type: text/plain; charset=utf-8\r\n")
	// Headers end, then the CRLF-normalized body.
	assert.Contains(t, s, "\r\n\r\nline one\r\nline two")
}

func TestBuildMessageStripsHeaderInjection(t *testing.T) {
	msg := buildMessage("noreply@example.com", Message{
		To:      "giver@example.com\r\nBcc: victim@example.com",
		Subject: "hi\r\nX-Injected: yes",
		Body:    "body",
	})
	s := string(msg)
	// The CRLF is stripped so the injected text folds into the header value rather
	// than becoming its own header line — no smuggled Bcc/X-Injected header.
	assert.NotContains(t, s, "\r\nBcc:")
	assert.NotContains(t, s, "\r\nX-Injected:")
	assert.Contains(t, s, "To: giver@example.comBcc: victim@example.com\r\n")
	assert.Contains(t, s, "Subject: hiX-Injected: yes\r\n")
}

func TestClientHelloName(t *testing.T) {
	assert.Equal(t, "example.com", clientHelloName("noreply@example.com"))
	assert.Equal(t, "localhost", clientHelloName("no-at-sign"))
	assert.Equal(t, "localhost", clientHelloName("trailing@"))
}

// TestSendPlaintextLoopback drives a real send against a hand-rolled in-process
// SMTP capture server on 127.0.0.1 (no network, no dependency). It confirms the
// full envelope + authenticated relay + message body reach the server.
func TestSendPlaintextLoopback(t *testing.T) {
	srv := newCaptureServer(t)
	defer srv.close()

	s, err := NewSMTPSender(SMTPConfig{
		Host:     "127.0.0.1",
		Port:     srv.port,
		Username: "relay-user",
		Password: "app-password",
		From:     "noreply@example.com",
		TLSMode:  TLSNone,
	}, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, s.Send(ctx, Message{
		To:      "giver@example.com",
		Subject: "Still planning to buy your reserved gift?",
		Body:    "Keep it: https://example/keep?token=abc",
	}))

	got := srv.result()
	assert.Equal(t, "noreply@example.com", got.mailFrom)
	assert.Equal(t, "giver@example.com", got.rcptTo)
	assert.True(t, got.authed, "expected AUTH PLAIN to be exercised")
	assert.Contains(t, got.data, "Subject: Still planning to buy your reserved gift?")
	assert.Contains(t, got.data, "Keep it: https://example/keep?token=abc")
}

// TestStartTLSRequiredRefusesPlaintextServer confirms the TLS-required posture:
// with TLSStartTLS the send is refused when the server does not advertise
// STARTTLS — credentials/tokens never go out over the resulting plaintext
// channel. (The capture server advertises AUTH but no STARTTLS, so this errors
// before any TLS handshake is attempted.)
func TestStartTLSRequiredRefusesPlaintextServer(t *testing.T) {
	srv := newCaptureServer(t)
	defer srv.close()

	s, err := NewSMTPSender(SMTPConfig{
		Host:    "127.0.0.1",
		Port:    srv.port,
		From:    "noreply@example.com",
		TLSMode: TLSStartTLS,
	}, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = s.Send(ctx, Message{To: "giver@example.com", Subject: "hi", Body: "b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "STARTTLS")
}

// captureResult holds what the fake server observed.
type captureResult struct {
	mailFrom string
	rcptTo   string
	data     string
	authed   bool
}

type captureServer struct {
	ln   net.Listener
	port int
	mu   sync.Mutex
	res  captureResult
	done chan struct{}
}

// newCaptureServer starts a minimal one-shot SMTP server on a random loopback
// port that speaks just enough of the protocol (EHLO/AUTH/MAIL/RCPT/DATA/QUIT)
// to capture what the sender delivers.
func newCaptureServer(t *testing.T) *captureServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	cs := &captureServer{ln: ln, port: ln.Addr().(*net.TCPAddr).Port, done: make(chan struct{})}
	go cs.serve()
	return cs
}

func (cs *captureServer) close() { _ = cs.ln.Close() }

func (cs *captureServer) result() captureResult {
	<-cs.done
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.res
}

func (cs *captureServer) serve() {
	defer close(cs.done)
	conn, err := cs.ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	writeLine := func(s string) { _, _ = w.WriteString(s + "\r\n"); _ = w.Flush() }

	writeLine("220 capture ready")
	var res captureResult
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			break
		}
		cmd := strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			writeLine("250-capture")
			writeLine("250 AUTH PLAIN")
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			res.authed = true
			writeLine("235 ok")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			res.mailFrom = extractAddr(cmd[len("MAIL FROM:"):])
			writeLine("250 ok")
		case strings.HasPrefix(upper, "RCPT TO:"):
			res.rcptTo = extractAddr(cmd[len("RCPT TO:"):])
			writeLine("250 ok")
		case upper == "DATA":
			writeLine("354 end with .")
			var b strings.Builder
			for {
				dl, derr := r.ReadString('\n')
				if derr != nil {
					break
				}
				if strings.TrimRight(dl, "\r\n") == "." {
					break
				}
				b.WriteString(dl)
			}
			res.data = b.String()
			writeLine("250 queued")
		case upper == "QUIT":
			writeLine("221 bye")
			cs.mu.Lock()
			cs.res = res
			cs.mu.Unlock()
			return
		default:
			writeLine("250 ok")
		}
	}
	cs.mu.Lock()
	cs.res = res
	cs.mu.Unlock()
}

// extractAddr pulls the bare address out of a "<addr>" argument.
func extractAddr(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<")
	if i := strings.IndexByte(s, '>'); i >= 0 {
		s = s[:i]
	}
	return s
}
