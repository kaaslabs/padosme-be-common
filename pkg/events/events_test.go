package events

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPaymentSuccessRoundtrip(t *testing.T) {
	in := PaymentSuccessEvent{
		PaymentID:        "p1",
		ProfileID:        "1abc",
		Provider:         "razorpay",
		BaseAmountPaisa:  10000,
		GSTPaisa:         1800,
		TotalAmountPaisa: 11800,
		Purpose:          PurposeCreditPurchase,
		CreditsGranted:   10000,
		Timestamp:        time.Unix(1700000000, 0).UTC(),
	}
	b, err := Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out PaymentSuccessEvent
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.CreditsGranted != 10000 || out.Purpose != PurposeCreditPurchase {
		t.Fatalf("roundtrip mismatch: %+v", out)
	}
}

func TestRoutingKeysUnique(t *testing.T) {
	keys := []string{
		RKProfileCreated, RKPaymentSuccess, RKPaymentFailed, RKPaymentRefunded,
		RKSubscriptionActivated, RKSubscriptionExpired, RKSubscriptionDowngradedN,
		RKSubscriptionRenewalDue, RKSubscriptionPaymentFail,
		RKWalletCredited, RKWalletDebited, RKWalletLowBalance,
		RKPromoPushed, RKPromoViewed, RKPromoContextExpired, RKCouponRedeemed,
	}
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] {
			t.Fatalf("duplicate routing key %q", k)
		}
		seen[k] = true
	}
}
