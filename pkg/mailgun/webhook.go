package mailgun

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type WebhookPayload struct {
	Recipient     string `json:"recipient"`
	Sender        string `json:"sender"`
	Subject       string `json:"subject"`
	BodyPlain     string `json:"body-plain"`
	StrippedText  string `json:"stripped-text"`
	BodyHtml      string `json:"body-html"`
	Timestamp     string `json:"timestamp"`
	Token         string `json:"token"`
	Signature     string `json:"signature"`
}

type JSONWebhookEnvelope struct {
	Signature struct {
		Timestamp string `json:"timestamp"`
		Token     string `json:"token"`
		Signature string `json:"signature"`
	} `json:"signature"`
	EventData struct {
		Recipient string `json:"recipient"`
		Message   struct {
			Headers struct {
				Subject string `json:"subject"`
				To      string `json:"to"`
				From    string `json:"from"`
			} `json:"headers"`
		} `json:"message"`
	} `json:"event-data"`
	Recipient string `json:"recipient"`
	BodyPlain string `json:"body-plain"`
}

func VerifySignature(signingKey, timestamp, token, signature string) bool {
	if signingKey == "" {
		// If signing key is not set, allow webhook for local dev/simulation
		return true
	}
	if timestamp == "" || token == "" || signature == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write([]byte(timestamp + token))
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

func ParseWebhook(r *http.Request) (*WebhookPayload, error) {
	contentType := r.Header.Get("Content-Type")

	// 1. If JSON payload
	if strings.Contains(contentType, "application/json") {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}

		// Try direct WebhookPayload
		var p WebhookPayload
		if err := json.Unmarshal(bodyBytes, &p); err == nil && p.Recipient != "" {
			return &p, nil
		}

		// Try nested envelope
		var env JSONWebhookEnvelope
		if err := json.Unmarshal(bodyBytes, &env); err == nil {
			recipient := env.Recipient
			if recipient == "" {
				recipient = env.EventData.Recipient
			}
			if recipient == "" {
				recipient = env.EventData.Message.Headers.To
			}
			return &WebhookPayload{
				Recipient: recipient,
				Subject:   env.EventData.Message.Headers.Subject,
				Sender:    env.EventData.Message.Headers.From,
				BodyPlain: env.BodyPlain,
				Timestamp: env.Signature.Timestamp,
				Token:     env.Signature.Token,
				Signature: env.Signature.Signature,
			}, nil
		}
		return nil, fmt.Errorf("unable to parse JSON webhook payload")
	}

	// 2. Form or Multipart
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if err := r.ParseForm(); err != nil {
			return nil, fmt.Errorf("failed to parse form data: %w", err)
		}
	}

	recipient := r.FormValue("recipient")
	if recipient == "" {
		recipient = r.FormValue("To")
	}
	if recipient == "" {
		recipient = r.FormValue("to")
	}

	// Normalize recipient: remove <>, quotes, lowercase
	recipient = strings.Trim(strings.TrimSpace(recipient), "<>\"'")
	recipient = strings.ToLower(recipient)

	bodyPlain := r.FormValue("body-plain")
	if bodyPlain == "" {
		bodyPlain = r.FormValue("stripped-text")
	}
	if bodyPlain == "" {
		bodyPlain = r.FormValue("body-html")
	}
	if bodyPlain == "" {
		bodyPlain = r.FormValue("stripped-html")
	}

	return &WebhookPayload{
		Recipient:    recipient,
		Sender:       strings.TrimSpace(r.FormValue("sender")),
		Subject:      strings.TrimSpace(r.FormValue("subject")),
		BodyPlain:    bodyPlain,
		StrippedText: r.FormValue("stripped-text"),
		BodyHtml:     r.FormValue("body-html"),
		Timestamp:    strings.TrimSpace(r.FormValue("timestamp")),
		Token:        strings.TrimSpace(r.FormValue("token")),
		Signature:    strings.TrimSpace(r.FormValue("signature")),
	}, nil
}
