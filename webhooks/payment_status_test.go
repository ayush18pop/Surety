package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ayush18pop/surety/storage"
)

func TestSendPaymentStatus(t *testing.T) {
	const secret = "test-secret"

	var gotBody []byte
	var gotSignature string
	var gotContentType string

	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading request body failed: %v", err)
		}
		gotSignature = r.Header.Get("X-Signature")
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	tr := storage.Transfer{
		BlockNumber: 100,
		LogIndex:    3,
		TxHash:      "0xdeadbeef",
		TokenSymbol: "USDC",
		From:        "0xfrom",
		To:          "0xto",
		RawAmount:   "500000000",
	}

	if err := SendPaymentStatus(receiver.URL, secret, tr, StatusFinal); err != nil {
		t.Fatalf("SendPaymentStatus failed: %v", err)
	}

	if gotContentType != "application/json" {
		t.Fatalf("got Content-Type %q, want application/json", gotContentType)
	}

	var payload PaymentStatusPayload
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshaling received body failed: %v", err)
	}
	want := PaymentStatusPayload{
		Status:      StatusFinal,
		BlockNumber: tr.BlockNumber,
		TxHash:      tr.TxHash,
		LogIndex:    tr.LogIndex,
		Token:       tr.TokenSymbol,
		From:        tr.From,
		To:          tr.To,
		Amount:      tr.RawAmount,
	}
	if payload != want {
		t.Fatalf("got payload %+v, want %+v", payload, want)
	}

	// The receiver must be able to independently recompute the same
	// signature from the secret and the exact bytes it received - that's
	// the entire point of signing. Recomputing it here rather than
	// asserting a hardcoded string also means this test doesn't silently
	// stop testing anything if the payload shape ever changes.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(gotBody)
	wantSignature := hex.EncodeToString(mac.Sum(nil))
	if gotSignature != wantSignature {
		t.Fatalf("got signature %q, want %q", gotSignature, wantSignature)
	}
}

func TestSendPaymentStatus_NonSuccessStatusIsAnError(t *testing.T) {
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer receiver.Close()

	err := SendPaymentStatus(receiver.URL, "secret", storage.Transfer{}, StatusFinal)
	if err == nil {
		t.Fatal("got nil error, want an error since the receiver returned 500")
	}
}

// The status genuinely has to flow through, not just be accepted and
// ignored - each of the three real statuses gets its own delivery loop in
// main.go, so a receiver needs to be able to tell them apart.
func TestSendPaymentStatus_StatusFlowsIntoPayload(t *testing.T) {
	for _, status := range []Status{StatusSeen, StatusSafe, StatusFinal} {
		var gotBody []byte
		receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var err error
			gotBody, err = io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("reading request body failed: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		}))

		if err := SendPaymentStatus(receiver.URL, "secret", storage.Transfer{TxHash: "0xtx"}, status); err != nil {
			t.Fatalf("SendPaymentStatus(%s) failed: %v", status, err)
		}
		receiver.Close()

		var payload PaymentStatusPayload
		if err := json.Unmarshal(gotBody, &payload); err != nil {
			t.Fatalf("unmarshaling received body failed: %v", err)
		}
		if payload.Status != status {
			t.Fatalf("got status %q, want %q", payload.Status, status)
		}
	}
}
