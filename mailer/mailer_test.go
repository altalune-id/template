package mailer

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestConsole_Send(t *testing.T) {
	buf := &bytes.Buffer{}
	c := Console{W: buf}
	err := c.Send(context.Background(), Message{To: "a@b", From: "f@x", Subject: "hi", TextBody: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hi") || !strings.Contains(buf.String(), "hello") {
		t.Errorf("console output missing content: %s", buf.String())
	}
}

func TestConsole_DefaultsFromCfgWhenMissing(t *testing.T) {
	buf := &bytes.Buffer{}
	c := Console{W: buf, From: "default@x"}
	if err := c.Send(context.Background(), Message{To: "a@b", Subject: "s", TextBody: "b"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "default@x") {
		t.Errorf("expected default from address: %s", buf.String())
	}
}

func TestNew_ConsoleDriver(t *testing.T) {
	m, err := New(Config{Driver: "console", From: "f@x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(*Console); !ok {
		t.Errorf("expected *Console, got %T", m)
	}
}

func TestNew_EmptyDriverDefaultsConsole(t *testing.T) {
	m, err := New(Config{From: "f@x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(*Console); !ok {
		t.Errorf("expected *Console for empty driver, got %T", m)
	}
}

func TestNew_MissingFrom(t *testing.T) {
	if _, err := New(Config{Driver: "console"}); err == nil {
		t.Fatal("expected error when from missing")
	}
}

func TestNew_SMTPDriver_OK(t *testing.T) {
	m, err := New(Config{Driver: "smtp", From: "f@x", SMTP: SMTPConfig{Host: "smtp.example.com", Port: 587}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(*SMTP); !ok {
		t.Errorf("expected *SMTP, got %T", m)
	}
}

func TestNew_SMTPDriver_RequiresHost(t *testing.T) {
	_, err := New(Config{Driver: "smtp", From: "f@x"})
	if err == nil {
		t.Fatal("expected error for smtp without host")
	}
}

func TestNew_UnknownDriver(t *testing.T) {
	_, err := New(Config{Driver: "carrier-pigeon", From: "f@x"})
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
}

func TestSMTP_Send_FailsOnUnreachableHost(t *testing.T) {
	s := &SMTP{Cfg: Config{From: "f@x", SMTP: SMTPConfig{Host: "127.0.0.1", Port: 1, User: "u", Pass: "p"}}}
	err := s.Send(context.Background(), Message{To: "t@x", Subject: "s", TextBody: "b"})
	if err == nil {
		t.Fatal("expected connection error against unreachable SMTP host")
	}
}

func TestBuildMIME_TextAndHTML(t *testing.T) {
	txt := buildMIME("f@x", Message{To: "t@x", Subject: "s", TextBody: "hi"})
	if !strings.Contains(txt, "text/plain") || !strings.Contains(txt, "hi") {
		t.Errorf("plain MIME missing: %s", txt)
	}
	html := buildMIME("f@x", Message{To: "t@x", Subject: "s", HTMLBody: "<b>hi</b>"})
	if !strings.Contains(html, "text/html") || !strings.Contains(html, "<b>hi</b>") {
		t.Errorf("html MIME missing: %s", html)
	}
}
