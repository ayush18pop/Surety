package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ayush18pop/surety/storage"
)

// httpClient is shared across sends rather than constructed per call, and
// carries an explicit timeout - the zero-value http.Client has none, so an
// unresponsive receiver could otherwise hang a caller indefinitely.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// PaymentStatusPayload is the JSON body sent when a transfer reaches final
// status. Status is always "final" for now - that's the one transition
// Surety currently detects (see storage.MarkFinalized) - but it's a named
// field rather than an implicit assumption, so a future status doesn't
// require breaking the payload shape.
type PaymentStatusPayload struct {
	Status      string `json:"status"`
	BlockNumber uint64 `json:"block_number"`
	TxHash      string `json:"tx_hash"`
	LogIndex    uint   `json:"log_index"`
	Token       string `json:"token"`
	From        string `json:"from"`
	To          string `json:"to"`
	Amount      string `json:"amount"`
}

// SendPaymentStatus notifies url that a transfer has reached final status.
// The body is signed with HMAC-SHA256 using secret, carried in the
// X-Signature header, so the receiver can verify the payload genuinely came
// from Surety and wasn't altered in transit - the same pattern Stripe,
// GitHub, and most webhook providers use.
func SendPaymentStatus(url, secret string, t storage.Transfer) error {
	payload := PaymentStatusPayload{
		Status:      "final",
		BlockNumber: t.BlockNumber,
		TxHash:      t.TxHash,
		LogIndex:    t.LogIndex,
		Token:       t.TokenSymbol,
		From:        t.From,
		To:          t.To,
		Amount:      t.RawAmount,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", sign(secret, body))

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook endpoint returned status %d", resp.StatusCode)
	}
	return nil
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
