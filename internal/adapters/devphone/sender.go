package devphone

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

var _ identity.PhoneCodeSender = (*LocalPhoneSender)(nil)

type LocalPhoneSender struct {
	smtpAddr string
}

func NewLocalPhoneSender(smtpAddr string) *LocalPhoneSender {
	if smtpAddr == "" {
		smtpAddr = "127.0.0.1:1025"
	}
	return &LocalPhoneSender{smtpAddr: smtpAddr}
}

func (s *LocalPhoneSender) SendPhoneCode(ctx context.Context, phoneE164, code string) error {
	log.Printf("[DEV SMS] Verification code for %s: %s (expires in 10 minutes)", phoneE164, code)

	if s.smtpAddr == "" {
		return nil
	}

	recipientEmail := fmt.Sprintf("sms-%s@mailpit.local", strings.TrimPrefix(phoneE164, "+"))
	subject := fmt.Sprintf("[SMS Dev] Código de verificación: %s", code)
	body := fmt.Sprintf("Tu código de verificación de EspaciGo para el número %s es: %s\r\n\r\nEste código expira en 10 minutos.\r\n", phoneE164, code)

	return s.sendMail(ctx, recipientEmail, subject, body)
}

func (s *LocalPhoneSender) sendMail(ctx context.Context, email, subject, body string) error {
	conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", s.smtpAddr)
	if err != nil {
		log.Printf("[DEV SMS] Mailpit unreachable at %s (%v), SMS logged to console only", s.smtpAddr, err)
		return nil
	}
	defer conn.Close()

	deadline := time.Now().Add(5 * time.Second)
	if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
		deadline = value
	}
	_ = conn.SetDeadline(deadline)

	host, _, err := net.SplitHostPort(s.smtpAddr)
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if err = client.Mail("dev-sms@espacigo.invalid"); err != nil {
		return err
	}
	if err = client.Rcpt(email); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	recipient := (&mail.Address{Address: email}).String()
	_, err = fmt.Fprintf(writer, "From: EspaciGo SMS <dev-sms@espacigo.invalid>\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", recipient, subject, body)
	if err != nil {
		return err
	}
	return writer.Close()
}

// StubPhoneSender provides a safe stub for production or environments without SMS integration.
type StubPhoneSender struct{}

var _ identity.PhoneCodeSender = (*StubPhoneSender)(nil)

func NewStubPhoneSender() *StubPhoneSender {
	return &StubPhoneSender{}
}

func (s *StubPhoneSender) SendPhoneCode(_ context.Context, phoneE164, code string) error {
	return nil
}
