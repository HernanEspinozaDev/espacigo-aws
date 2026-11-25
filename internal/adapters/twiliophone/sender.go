package twiliophone

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
)

type Config struct {
	AccountSID, APIKeySID, APIKeySecret string
	MessagingServiceSID                 string
	WhatsAppFrom, WhatsAppContentSID    string
}

type Sender struct {
	cfg    Config
	client *http.Client
}

var (
	accountSIDPattern = regexp.MustCompile(`^AC[0-9a-fA-F]{32}$`)
	apiKeyPattern     = regexp.MustCompile(`^SK[0-9a-fA-F]{32}$`)
	servicePattern    = regexp.MustCompile(`^MG[0-9a-fA-F]{32}$`)
	contentPattern    = regexp.MustCompile(`^HX[0-9a-fA-F]{32}$`)
	senderPattern     = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
)

func New(cfg Config) (*Sender, error) {
	if !accountSIDPattern.MatchString(cfg.AccountSID) || !apiKeyPattern.MatchString(cfg.APIKeySID) ||
		cfg.APIKeySecret == "" || !servicePattern.MatchString(cfg.MessagingServiceSID) {
		return nil, fmt.Errorf("incomplete or invalid Twilio SMS configuration")
	}
	if (cfg.WhatsAppFrom != "" || cfg.WhatsAppContentSID != "") &&
		(!senderPattern.MatchString(cfg.WhatsAppFrom) || !contentPattern.MatchString(cfg.WhatsAppContentSID)) {
		return nil, fmt.Errorf("incomplete or invalid Twilio WhatsApp configuration")
	}
	return &Sender{
		cfg: cfg,
		client: &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (s *Sender) Channels() []identity.PhoneChannel {
	channels := []identity.PhoneChannel{identity.PhoneChannelSMS}
	if s.cfg.WhatsAppFrom != "" {
		channels = append(channels, identity.PhoneChannelWhatsApp)
	}
	return channels
}

func (s *Sender) SendPhoneCode(ctx context.Context, phone, code string, channel identity.PhoneChannel) error {
	form := url.Values{}
	switch channel {
	case identity.PhoneChannelSMS:
		form.Set("To", phone)
		form.Set("MessagingServiceSid", s.cfg.MessagingServiceSID)
		form.Set("Body", "EspaciGo: tu codigo de verificacion es "+code+". Vence en 10 minutos. No lo compartas.")
	case identity.PhoneChannelWhatsApp:
		if s.cfg.WhatsAppFrom == "" {
			return identity.ErrPhoneDeliveryUnavailable
		}
		variables, err := json.Marshal(map[string]string{"1": code})
		if err != nil {
			return err
		}
		form.Set("To", "whatsapp:"+phone)
		form.Set("From", "whatsapp:"+s.cfg.WhatsAppFrom)
		form.Set("ContentSid", s.cfg.WhatsAppContentSID)
		form.Set("ContentVariables", string(variables))
	default:
		return identity.ErrPhoneDeliveryUnavailable
	}

	endpoint := "https://api.twilio.com/2010-04-01/Accounts/" + s.cfg.AccountSID + "/Messages.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.cfg.APIKeySID, s.cfg.APIKeySecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("Twilio messaging request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("Twilio messaging returned status %d", resp.StatusCode)
	}

	var result struct {
		SID    string `json:"sid"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&result); err != nil || result.SID == "" {
		return fmt.Errorf("invalid response from Twilio messaging")
	}
	switch result.Status {
	case "accepted", "queued", "sending", "sent", "delivered":
		return nil
	default:
		return fmt.Errorf("Twilio did not accept the message")
	}
}
