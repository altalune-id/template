// Package mailer sends transactional email. Console driver writes to a writer; SMTP uses STARTTLS.
package mailer

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Config is the input mailer needs to construct a driver.
type Config struct {
	Driver string
	From   string
	SMTP   SMTPConfig
}

// SMTPConfig configures the SMTP driver used by Config.
type SMTPConfig struct {
	Host string
	Port int
	User string
	Pass string
	TLS  bool
}

// Message is one email being sent.
type Message struct {
	To       string
	From     string
	Subject  string
	TextBody string
	HTMLBody string
}

// Validate rejects any Message whose header-line fields (From/To/Subject) contain CR or LF (RFC 5322 section 2.2.3 forbids CR/LF in header field bodies).
func (m Message) Validate() error {
	for _, f := range []struct {
		name, val string
	}{
		{"From", m.From},
		{"To", m.To},
		{"Subject", m.Subject},
	} {
		if strings.ContainsAny(f.val, "\r\n") {
			return &HeaderInjectionError{Field: f.name}
		}
	}
	return nil
}

// HeaderInjectionError is returned when a Message field that becomes an SMTP header contains CR or LF.
type HeaderInjectionError struct {
	Field string
}

func (e *HeaderInjectionError) Error() string {
	return fmt.Sprintf("mailer: header injection in %s: CR/LF not allowed", e.Field)
}

// IsHeaderInjectionError reports whether err (or anything it wraps) is a HeaderInjectionError.
func IsHeaderInjectionError(err error) bool {
	var target *HeaderInjectionError
	return errors.As(err, &target)
}

// Mailer sends a Message via whichever driver Config selected.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// New builds a Mailer from cfg. Empty driver falls back to console.
func New(cfg Config) (Mailer, error) {
	if cfg.From == "" {
		return nil, errors.New("mailer: from address required")
	}
	switch cfg.Driver {
	case "", "console":
		return &Console{From: cfg.From}, nil
	case "smtp":
		if cfg.SMTP.Host == "" {
			return nil, errors.New("mailer: smtp.host required")
		}
		return &SMTP{Cfg: cfg}, nil
	default:
		return nil, fmt.Errorf("mailer: unknown driver %q", cfg.Driver)
	}
}
