package main

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/devphone"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/twiliophone"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

func phoneDelivery(getenv func(string) string, local bool) (identity.PhoneCodeSender, []byte, error) {
	switch strings.TrimSpace(getenv("PHONE_OTP_PROVIDER")) {
	case "":
		if local {
			return devphone.NewLocalPhoneSender(getenv("LOCAL_SMTP_ADDR")), nil, nil
		}
		return nil, nil, nil
	case "twilio":
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(getenv("PHONE_OTP_HMAC_KEY")))
		if err != nil || len(key) < 32 {
			return nil, nil, errors.New("PHONE_OTP_HMAC_KEY must contain at least 32 base64-encoded bytes")
		}
		sender, err := twiliophone.New(twiliophone.Config{
			AccountSID:          strings.TrimSpace(getenv("TWILIO_ACCOUNT_SID")),
			APIKeySID:           strings.TrimSpace(getenv("TWILIO_API_KEY_SID")),
			APIKeySecret:        getenv("TWILIO_API_KEY_SECRET"),
			MessagingServiceSID: strings.TrimSpace(getenv("TWILIO_MESSAGING_SERVICE_SID")),
			WhatsAppFrom:        strings.TrimSpace(getenv("TWILIO_WHATSAPP_FROM")),
			WhatsAppContentSID:  strings.TrimSpace(getenv("TWILIO_WHATSAPP_CONTENT_SID")),
		})
		if err != nil {
			return nil, nil, err
		}
		return sender, key, nil
	default:
		return nil, nil, errors.New("unsupported PHONE_OTP_PROVIDER")
	}
}
