package email

import (
	"context"
	"errors"
	"io"
	"net"
	"net/textproto"
	"strings"
	"testing"

	"github.com/ChrisMarxDev/tinkercloud/internal/otp"
)

type fakeSMTPSession struct {
	mailFrom, recipient, body string
	fail                      string
	quit                      bool
}

func (s *fakeSMTPSession) Mail(value string) error {
	s.mailFrom = value
	if s.fail == "mail" {
		return errors.New("sensitive mail failure")
	}
	return nil
}
func (s *fakeSMTPSession) Rcpt(value string) error {
	s.recipient = value
	if s.fail == "recipient" {
		return errors.New("sensitive recipient failure")
	}
	return nil
}
func (s *fakeSMTPSession) Data() (io.WriteCloser, error) {
	if s.fail == "data" {
		return nil, errors.New("sensitive data failure")
	}
	return &smtpTestWriter{session: s, fail: s.fail == "write"}, nil
}
func (s *fakeSMTPSession) Quit() error {
	s.quit = true
	if s.fail == "quit" {
		return errors.New("sensitive quit failure")
	}
	return nil
}
func (*fakeSMTPSession) Close() error { return nil }

type smtpTestWriter struct {
	session *fakeSMTPSession
	fail    bool
}

func (w *smtpTestWriter) Write(p []byte) (int, error) {
	if w.fail {
		return 0, errors.New("sensitive write failure")
	}
	w.session.body += string(p)
	return len(p), nil
}
func (w *smtpTestWriter) Close() error { return nil }

func testSMTP(session *fakeSMTPSession) SMTP {
	return SMTP{
		Host: "smtp.example.test", Port: 587, Username: "operator@example.test",
		Password: "smtp-secret", From: "Tinkercloud <access@example.test>", TLSMode: "starttls",
		dial: func(context.Context, SMTP) (smtpSession, error) { return session, nil },
	}
}

func TestSMTPSubmitsOneTextMessage(t *testing.T) {
	session := &fakeSMTPSession{}
	provider := testSMTP(session)
	if err := provider.EnqueueOTP(context.Background(), otp.Message{Email: "viewer@example.test", Code: "123456"}); err != nil {
		t.Fatal(err)
	}
	if session.mailFrom != "access@example.test" || session.recipient != "viewer@example.test" || !session.quit {
		t.Fatalf("envelope = %q %q, quit=%v", session.mailFrom, session.recipient, session.quit)
	}
	for _, want := range []string{"Subject: Your sign-in code\r\n", "Content-Type: text/plain; charset=UTF-8\r\n", "Your code: 123456\r\n"} {
		if !strings.Contains(session.body, want) {
			t.Fatalf("message missing %q: %q", want, session.body)
		}
	}
}

func TestSMTPCheckDoesNotCreateEnvelope(t *testing.T) {
	session := &fakeSMTPSession{}
	if err := testSMTP(session).Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if session.mailFrom != "" || session.recipient != "" || session.body != "" || !session.quit {
		t.Fatalf("check sent mail: %#v", session)
	}
}

func TestWireSMTPSessionEnvelopeAndDotEncoding(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
	serverDone := make(chan error, 1)
	go func() {
		server := textproto.NewConn(serverConn)
		defer server.Close()
		for _, exchange := range []struct{ command, response string }{
			{"MAIL FROM:<access@example.test>", "250 sender ok"},
			{"RCPT TO:<viewer@example.test>", "250 recipient ok"},
			{"DATA", "354 send data"},
		} {
			line, err := server.ReadLine()
			if err != nil || line != exchange.command {
				serverDone <- errors.New("unexpected smtp command")
				return
			}
			if err = server.PrintfLine("%s", exchange.response); err != nil {
				serverDone <- err
				return
			}
		}
		data, err := server.ReadDotBytes()
		if err != nil || string(data) != ".leading\nbody\n" {
			serverDone <- errors.New("unexpected smtp data")
			return
		}
		if err = server.PrintfLine("250 accepted"); err != nil {
			serverDone <- err
			return
		}
		line, err := server.ReadLine()
		if err != nil || line != "QUIT" {
			serverDone <- errors.New("missing smtp quit")
			return
		}
		if err = server.PrintfLine("221 goodbye"); err != nil {
			serverDone <- err
			return
		}
		serverDone <- nil
	}()

	session := &wireSMTPSession{conn: clientConn, text: textproto.NewConn(clientConn)}
	if err := session.Mail("access@example.test"); err != nil {
		t.Fatal(err)
	}
	if err := session.Rcpt("viewer@example.test"); err != nil {
		t.Fatal(err)
	}
	w, err := session.Data()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(w, ".leading\r\nbody\r\n"); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	if err = session.Quit(); err != nil {
		t.Fatal(err)
	}
	if err = <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestSMTPFailuresAreRedacted(t *testing.T) {
	message := otp.Message{Email: "viewer@example.test", Code: "sensitive-code"}
	for _, failure := range []string{"mail", "recipient", "data", "write", "quit"} {
		t.Run(failure, func(t *testing.T) {
			provider := testSMTP(&fakeSMTPSession{fail: failure})
			if err := provider.EnqueueOTP(context.Background(), message); err != ErrUnavailable {
				t.Fatalf("error = %v", err)
			}
		})
	}
	for name, mutate := range map[string]func(*SMTP, *otp.Message){
		"plaintext":         func(s *SMTP, _ *otp.Message) { s.TLSMode = "none" },
		"newline sender":    func(s *SMTP, _ *otp.Message) { s.From = "a@example.test\r\nBcc: victim@example.test" },
		"newline recipient": func(_ *SMTP, m *otp.Message) { m.Email = "a@example.test\r\nBcc: victim@example.test" },
		"missing password":  func(s *SMTP, _ *otp.Message) { s.Password = "" },
	} {
		t.Run(name, func(t *testing.T) {
			provider := testSMTP(&fakeSMTPSession{})
			candidate := message
			mutate(&provider, &candidate)
			if err := provider.EnqueueOTP(context.Background(), candidate); err != ErrUnavailable {
				t.Fatalf("error = %v", err)
			}
		})
	}
	provider := testSMTP(&fakeSMTPSession{})
	provider.dial = func(context.Context, SMTP) (smtpSession, error) { return nil, errors.New("smtp-secret") }
	if err := provider.EnqueueOTP(context.Background(), message); err != ErrUnavailable {
		t.Fatalf("dial error = %v", err)
	}
}
