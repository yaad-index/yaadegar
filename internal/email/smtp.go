package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"syscall"
	"time"
)

// TLSMode selects how the SMTP connection is secured.
type TLSMode string

const (
	// TLSStartTLS upgrades a plaintext connection with STARTTLS (the default;
	// smtp.gmail.com:587 etc.). Required: the send fails if the server does not
	// offer STARTTLS.
	TLSStartTLS TLSMode = "starttls"
	// TLSImplicit dials TLS directly (SMTPS, port 465).
	TLSImplicit TLSMode = "tls"
	// TLSNone is plaintext — dev/test only, and refused for a non-loopback host.
	TLSNone TLSMode = "none"
)

// SMTPConfig configures the SMTP sender.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	TLSMode  TLSMode
}

// SMTPSender sends mail over SMTP with authenticated relay support. Messages
// carry co-buyer contacts and capability-token links, so TLS is required by
// default; TLSNone is allowed only for a loopback host.
type SMTPSender struct {
	cfg    SMTPConfig
	logger *slog.Logger
}

// NewSMTPSender validates cfg and returns a sender. It refuses a plaintext
// (TLSNone) connection to a non-loopback host so credentials and tokens are
// never sent in the clear.
func NewSMTPSender(cfg SMTPConfig, logger *slog.Logger) (*SMTPSender, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("email: SMTP host is required")
	}
	if cfg.From == "" {
		return nil, fmt.Errorf("email: SMTP from address is required")
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	if cfg.TLSMode == "" {
		cfg.TLSMode = TLSStartTLS
	}
	if cfg.TLSMode != TLSStartTLS && cfg.TLSMode != TLSImplicit && cfg.TLSMode != TLSNone {
		return nil, fmt.Errorf("email: unknown TLS mode %q", cfg.TLSMode)
	}
	// TLSNone is enforced loopback-only at DIAL time (on the resolved IP, not the
	// host string) — see dialGuardLoopback — so a hostname that resolves off-box
	// can never leak plaintext credentials/tokens.
	return &SMTPSender{cfg: cfg, logger: logger}, nil
}

// Send delivers m. Errors are returned and logged — never swallowed.
func (s *SMTPSender) Send(ctx context.Context, m Message) error {
	if err := s.send(ctx, m); err != nil {
		s.logger.ErrorContext(ctx, "smtp send failed", "err", err, "to", m.To, "subject", m.Subject)
		return fmt.Errorf("email: send to %q: %w", m.To, err)
	}
	return nil
}

func (s *SMTPSender) send(ctx context.Context, m Message) error {
	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprintf("%d", s.cfg.Port))
	dialer := &net.Dialer{}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(15 * time.Second)
	}
	dialer.Deadline = deadline

	// Plaintext (TLSNone) is loopback-only, checked on the resolved dial IP.
	if s.cfg.TLSMode == TLSNone {
		dialer.Control = dialGuardLoopback
	}

	var conn net.Conn
	var err error
	if s.cfg.TLSMode == TLSImplicit {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: s.cfg.Host})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	// Bound the whole SMTP conversation so a hung/slow server can't block the
	// caller (e.g. the decay sweeper) past the deadline.
	_ = conn.SetDeadline(deadline)

	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("new client: %w", err)
	}
	defer func() { _ = c.Close() }()

	if err := c.Hello(clientHelloName(s.cfg.From)); err != nil {
		return fmt.Errorf("hello: %w", err)
	}

	if s.cfg.TLSMode == TLSStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("server does not offer STARTTLS (TLS is required)")
		}
		if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host}); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}

	if s.cfg.Username != "" {
		if ok, _ := c.Extension("AUTH"); !ok {
			return fmt.Errorf("server does not offer AUTH but credentials were configured")
		}
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}

	if err := c.Mail(s.cfg.From); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := c.Rcpt(m.To); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(buildMessage(s.cfg.From, m)); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}
	return c.Quit()
}

// buildMessage renders an RFC 5322 message. Header values are sanitized against
// CRLF injection and the body is CRLF-normalized.
func buildMessage(from string, m Message) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", sanitizeHeader(from))
	fmt.Fprintf(&b, "To: %s\r\n", sanitizeHeader(m.To))
	fmt.Fprintf(&b, "Subject: %s\r\n", sanitizeHeader(m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(m.Body, "\r\n", "\n"), "\n", "\r\n"))
	return []byte(b.String())
}

// sanitizeHeader strips CR/LF to prevent header injection via a crafted address
// or subject.
func sanitizeHeader(v string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(v)
}

func clientHelloName(from string) string {
	if i := strings.LastIndex(from, "@"); i >= 0 && i+1 < len(from) {
		return from[i+1:]
	}
	return "localhost"
}

// dialGuardLoopback is the net.Dialer.Control hook used for plaintext (TLSNone)
// sends: it inspects the actual resolved dial address and refuses anything that
// is not a loopback IP, so plaintext credentials/tokens can never leave the box
// (post-DNS, same principle as the SSRF guard). Fails closed on a parse error.
func dialGuardLoopback(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("email: unparseable dial address %q", address)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("email: refusing plaintext (TLS mode %q) to non-loopback address %q", TLSNone, host)
	}
	return nil
}
