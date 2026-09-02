// Package mailer sends transactional email. Console driver writes to a writer; SMTP uses STARTTLS.
package mailer

import (
	"context"
	"errors"
	"fmt"
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
