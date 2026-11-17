// services/connectors/standard_webhooks.go
//
// Verification of webhook requests signed per the Standard Webhooks
// specification (https://www.standardwebhooks.com): the sender puts
//
//	webhook-id         a message ID, the same on every retry of the message
//	webhook-timestamp  Unix seconds when this attempt was signed
//	webhook-signature  space-separated "v1,<base64 HMAC-SHA256>" values
//
// on the request, where each HMAC is over "<id>.<timestamp>.<raw body>"
// under the shared secret. The secret is "whsec_" followed by the key in
// base64. A sender rotating its secret signs with both the old and the new
// one, so a receiver needs only one secret at a time: any matching
// signature is enough.
//
// The timestamp tolerance bounds replay of a captured request. A repeated
// webhook-id is NOT refused: senders reuse it on every retry, and a retry
// after a failed delivery must be accepted. Receivers that must not act
// twice de-duplicate on their own content (e.g. an event sequence).
package connectors

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Standard Webhooks header names.
const (
	StandardWebhookIDHeader        = "webhook-id"
	StandardWebhookTimestampHeader = "webhook-timestamp"
	StandardWebhookSignatureHeader = "webhook-signature"

	standardWebhookSecretPrefix = "whsec_"
	// DefaultStandardWebhookTolerance is the specification's recommended
	// window for the signing timestamp.
	DefaultStandardWebhookTolerance = 5 * time.Minute
)

// standardWebhookVerifier checks one request against one secret.
type standardWebhookVerifier struct {
	key       []byte
	tolerance time.Duration
	now       func() time.Time
}

// parseStandardWebhookSecret decodes "whsec_<base64>" (the prefix is
// optional) into the HMAC key. Keys shorter than 24 bytes are refused.
func parseStandardWebhookSecret(secret string) ([]byte, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, errors.New("webhook_secret is required when authentication_type is 'standard_webhooks'")
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, standardWebhookSecretPrefix))
	if err != nil {
		return nil, errors.New("webhook_secret must be whsec_ followed by the key in base64")
	}
	if len(key) < 24 || len(key) > 64 {
		return nil, errors.New("webhook_secret's key must be 24 to 64 bytes")
	}
	return key, nil
}

func newStandardWebhookVerifier(secret string, tolerance time.Duration) (*standardWebhookVerifier, error) {
	key, err := parseStandardWebhookSecret(secret)
	if err != nil {
		return nil, err
	}
	if tolerance <= 0 {
		tolerance = DefaultStandardWebhookTolerance
	}
	return &standardWebhookVerifier{key: key, tolerance: tolerance, now: time.Now}, nil
}

// Verify returns nil when one of the request's signatures matches the body
// and its timestamp is within tolerance of now.
func (v *standardWebhookVerifier) Verify(h http.Header, body []byte) error {
	id := h.Get(StandardWebhookIDHeader)
	ts := h.Get(StandardWebhookTimestampHeader)
	sigs := h.Get(StandardWebhookSignatureHeader)
	if id == "" || ts == "" || sigs == "" {
		return errors.New("missing webhook signature headers")
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errors.New("invalid webhook timestamp")
	}
	if d := v.now().Sub(time.Unix(sec, 0)); d > v.tolerance || d < -v.tolerance {
		return errors.New("webhook timestamp outside tolerance")
	}
	mac := hmac.New(sha256.New, v.key)
	mac.Write([]byte(id + "." + ts + "."))
	mac.Write(body)
	want := mac.Sum(nil)
	for _, sig := range strings.Fields(sigs) {
		version, value, ok := strings.Cut(sig, ",")
		if !ok || version != "v1" {
			continue
		}
		got, err := base64.StdEncoding.DecodeString(value)
		if err == nil && hmac.Equal(got, want) {
			return nil
		}
	}
	return errors.New("no matching webhook signature")
}
