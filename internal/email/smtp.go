package email

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/ChrisMarxDev/tinkercloud/internal/otp"
)

type smtpSession interface {
	Mail(string) error
	Rcpt(string) error
	Data() (io.WriteCloser, error)
	Quit() error
	Close() error
}

type smtpDialFunc func(context.Context, SMTP) (smtpSession, error)

// SMTP submits outbound OTP mail to one operator-configured authenticated
// server. TLSMode accepts only "starttls" or implicit "tls"; plaintext SMTP
// is deliberately unavailable.
type SMTP struct {
	Host, Username, Password, From, TLSMode string
	Port                                    int
	dial                                    smtpDialFunc
}

func (s SMTP) EnqueueOTP(ctx context.Context, message otp.Message) error {
	from, to, ok := s.mailboxes(message)
	if !ok || strings.ContainsAny(message.Code, "\r\n\x00") {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	session, err := s.open(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer session.Close()
	if err = session.Mail(from.Address); err != nil {
		return ErrUnavailable
	}
	if err = session.Rcpt(to.Address); err != nil {
		return ErrUnavailable
	}
	w, err := session.Data()
	if err != nil {
		return ErrUnavailable
	}
	body := "From: " + from.String() + "\r\n" +
		"To: " + to.String() + "\r\n" +
		"Subject: Your sign-in code\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n" +
		"Your code: " + message.Code + "\r\n"
	if _, err = io.WriteString(w, body); err != nil {
		_ = w.Close()
		return ErrUnavailable
	}
	if err = w.Close(); err != nil {
		return ErrUnavailable
	}
	if err = session.Quit(); err != nil {
		return ErrUnavailable
	}
	return nil
}

// Check authenticates without creating an envelope or sending a message.
func (s SMTP) Check(ctx context.Context) error {
	if _, _, ok := s.mailboxes(otp.Message{Email: "doctor@example.invalid", Code: "000000"}); !ok {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	session, err := s.open(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer session.Close()
	if err = session.Quit(); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (s SMTP) mailboxes(message otp.Message) (*mail.Address, *mail.Address, bool) {
	if s.Host == "" || s.Port < 1 || s.Port > 65535 || s.Username == "" || s.Password == "" ||
		(s.TLSMode != "starttls" && s.TLSMode != "tls") || strings.ContainsAny(s.Host+s.Username+s.Password, "\r\n\x00") {
		return nil, nil, false
	}
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return nil, nil, false
	}
	to, err := mail.ParseAddress(message.Email)
	if err != nil || to.Name != "" {
		return nil, nil, false
	}
	return from, to, true
}

func (s SMTP) open(ctx context.Context) (smtpSession, error) {
	dial := s.dial
	if dial == nil {
		dial = dialSMTP
	}
	return dial(ctx, s)
}

func dialSMTP(ctx context.Context, settings SMTP) (smtpSession, error) {
	address := net.JoinHostPort(settings.Host, strconv.Itoa(settings.Port))
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	closeConn := true
	defer func() {
		if closeConn {
			_ = conn.Close()
		}
	}()
	if deadline, ok := ctx.Deadline(); ok {
		if err = conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
	}
	tlsConfig := &tls.Config{ServerName: settings.Host, MinVersion: tls.VersionTLS12}
	if settings.TLSMode == "tls" {
		tlsConn := tls.Client(conn, tlsConfig)
		if err = tlsConn.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = tlsConn
	}
	client := &wireSMTPSession{conn: conn, text: textproto.NewConn(conn)}
	if err = client.response(220); err != nil {
		return nil, err
	}
	if err = client.command(250, "EHLO tinkercloud"); err != nil {
		return nil, err
	}
	if settings.TLSMode == "starttls" {
		if err = client.command(220, "STARTTLS"); err != nil {
			return nil, err
		}
		tlsConn := tls.Client(conn, tlsConfig)
		if err = tlsConn.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = tlsConn
		client.conn = tlsConn
		client.text = textproto.NewConn(tlsConn)
		if err = client.command(250, "EHLO tinkercloud"); err != nil {
			return nil, err
		}
	}
	token := base64.StdEncoding.EncodeToString([]byte("\x00" + settings.Username + "\x00" + settings.Password))
	if err = client.command(235, "AUTH PLAIN "+token); err != nil {
		return nil, errors.New("smtp authentication unavailable")
	}
	closeConn = false
	return client, nil
}

type wireSMTPSession struct {
	conn net.Conn
	text *textproto.Conn
}

func (s *wireSMTPSession) response(code int) error {
	if _, _, err := s.text.ReadResponse(code); err != nil {
		return errors.New("smtp response unavailable")
	}
	return nil
}

func (s *wireSMTPSession) command(code int, command string) error {
	id, err := s.text.Cmd("%s", command)
	if err != nil {
		return errors.New("smtp command unavailable")
	}
	s.text.StartResponse(id)
	defer s.text.EndResponse(id)
	return s.response(code)
}

func (s *wireSMTPSession) Mail(address string) error {
	if strings.ContainsAny(address, "<>\r\n\x00") {
		return errors.New("smtp envelope unavailable")
	}
	return s.command(250, "MAIL FROM:<"+address+">")
}

func (s *wireSMTPSession) Rcpt(address string) error {
	if strings.ContainsAny(address, "<>\r\n\x00") {
		return errors.New("smtp envelope unavailable")
	}
	return s.command(250, "RCPT TO:<"+address+">")
}

func (s *wireSMTPSession) Data() (io.WriteCloser, error) {
	if err := s.command(354, "DATA"); err != nil {
		return nil, err
	}
	return &smtpDataWriter{session: s, writer: s.text.DotWriter()}, nil
}

func (s *wireSMTPSession) Quit() error  { return s.command(221, "QUIT") }
func (s *wireSMTPSession) Close() error { return s.text.Close() }

type smtpDataWriter struct {
	session *wireSMTPSession
	writer  io.WriteCloser
}

func (w *smtpDataWriter) Write(p []byte) (int, error) { return w.writer.Write(p) }
func (w *smtpDataWriter) Close() error {
	if err := w.writer.Close(); err != nil {
		return err
	}
	return w.session.response(250)
}
