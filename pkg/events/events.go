// Package events defines typed payloads and routing keys for money-layer
// RMQ events exchanged between padosme-payment-service,
// padosme-subscription-service, padosme-wallet-service and producers/consumers
// (channel-service, profile/seller-service, notification-service).
//
// All monetary fields are in paisa only on the payment boundary.
// Internally, wallets operate in credits where 1 paisa = 1 credit.
// GST (18%) lives only on payment events; credits_granted excludes GST.
package events

import (
	"encoding/json"
	"time"
)

// Routing keys published on the shared topic exchange.
const (
	RKProfileCreated          = "profile.created"
	RKPaymentSuccess          = "payment.success"
	RKPaymentFailed           = "payment.failed"
	RKPaymentRefunded         = "payment.refunded"
	RKSubscriptionActivated   = "subscription.activated"
	RKSubscriptionExpired     = "subscription.expired"
	RKSubscriptionDowngradedN = "subscription.downgraded_to_n"
	RKSubscriptionRenewalDue  = "subscription.renewal_due"
	RKSubscriptionPaymentFail = "subscription.payment_failed"
	RKWalletCredited          = "wallet.credited"
	RKWalletDebited           = "wallet.debited"
	RKWalletLowBalance        = "wallet.low_balance"
	RKPromoPushed             = "promo.pushed"
	RKPromoViewed             = "promo.viewed"
	RKPromoContextExpired     = "promo.context.expired"
	RKCouponRedeemed          = "coupon.redeemed"
)

// PaymentPurpose classifies what a payment is for.
type PaymentPurpose string

const (
	PurposeCreditPurchase PaymentPurpose = "credit_purchase"
	PurposeSubscription   PaymentPurpose = "subscription"
	PurposeOther          PaymentPurpose = "other"
)

// BillingCycle for subscriptions.
type BillingCycle string

const (
	BillingMonthly BillingCycle = "monthly"
	BillingYearly  BillingCycle = "yearly"
)

// PlanTier identifiers.
const (
	TierN   = "N"
	TierXS  = "XS"
	TierS   = "S"
	TierM   = "M"
	TierL   = "L"
	TierXL  = "XL"
	Tier2XL = "2XL"
	Tier3XL = "3XL"
)

// CompanyWalletScope tells the wallet-service which company wallet receives
// or provides promotion credits, based on the promoting seller's plan tier.
type CompanyWalletScope string

const (
	ScopeGlobal    CompanyWalletScope = "global"
	ScopeTenant    CompanyWalletScope = "tenant"
	ScopeGlobalMNC CompanyWalletScope = "global_mnc"
)

// Envelope is the optional shared wrapper for all events. Consumers may also
// unmarshal directly into the payload struct if they prefer.
type Envelope struct {
	Event     string          `json:"event"`
	EventID   string          `json:"event_id"`
	Timestamp time.Time       `json:"ts"`
	Payload   json.RawMessage `json:"payload"`
}

// ProfileCreatedEvent — emitted by user-profile-service and seller-service when
// a new profile is persisted. Wallet-service uses this to auto-provision a
// wallet. OwnerType is derived from the profile_id prefix by the producer.
type ProfileCreatedEvent struct {
	ProfileID string    `json:"profile_id"`
	OwnerType string    `json:"owner_type"` // user | seller | salesman
	TenantID  string    `json:"tenant_id,omitempty"`
	Timestamp time.Time `json:"ts"`
}

// PaymentSuccessEvent — emitted by payment-service after Razorpay confirms.
// All amounts in paisa. credits_granted = base_amount_paisa for
// credit_purchase; zero for subscription.
type PaymentSuccessEvent struct {
	PaymentID        string         `json:"payment_id"`
	ProfileID        string         `json:"profile_id"`
	Provider         string         `json:"provider"` // razorpay
	ProviderOrderID  string         `json:"provider_order_id,omitempty"`
	ProviderPaymentID string        `json:"provider_payment_id,omitempty"`
	BaseAmountPaisa  int64          `json:"base_amount_paisa"`
	GSTPaisa         int64          `json:"gst_paisa"`
	TotalAmountPaisa int64          `json:"total_amount_paisa"`
	Purpose          PaymentPurpose `json:"purpose"`
	PurposeRefID     string         `json:"purpose_ref_id,omitempty"`
	CreditsGranted   int64          `json:"credits_granted"`
	CouponCode       string         `json:"coupon_code,omitempty"`
	DiscountPaisa    int64          `json:"discount_paisa,omitempty"`
	Timestamp        time.Time      `json:"ts"`
}

// PaymentFailedEvent — emitted when a payment attempt fails definitively.
type PaymentFailedEvent struct {
	PaymentID    string         `json:"payment_id"`
	ProfileID    string         `json:"profile_id"`
	Purpose      PaymentPurpose `json:"purpose"`
	PurposeRefID string         `json:"purpose_ref_id,omitempty"`
	Reason       string         `json:"reason"`
	Timestamp    time.Time      `json:"ts"`
}

// PaymentRefundedEvent — emitted on a successful refund, full or partial.
type PaymentRefundedEvent struct {
	PaymentID      string    `json:"payment_id"`
	RefundID       string    `json:"refund_id"`
	ProfileID      string    `json:"profile_id"`
	AmountPaisa    int64     `json:"amount_paisa"`
	Reason         string    `json:"reason,omitempty"`
	Timestamp      time.Time `json:"ts"`
}

// PlanLimits carries the cached, resolved plan limits so downstream services
// don't have to hit subscription-service synchronously on every event.
type PlanLimits struct {
	Tier              string             `json:"tier"`
	Segment           string             `json:"segment"`
	CompanyWalletScope CompanyWalletScope `json:"company_wallet_scope"`
	CreditLimit       int64              `json:"credit_limit"`
	PurchaseLimit     int64              `json:"purchase_limit"`
	CatalogueLimit    int64              `json:"catalogue_limit"`
	GroupLimit        int64              `json:"group_limit"`
	DelegationLimit   int64              `json:"delegation_limit"`
	BundledCredits    int64              `json:"bundled_credits"`
}

// SubscriptionActivatedEvent — emitted by subscription-service after
// payment.success for purpose=subscription. Carries full limits snapshot
// so wallet (and others) can act without a follow-up HTTP call.
type SubscriptionActivatedEvent struct {
	SubscriptionID  string       `json:"subscription_id"`
	SellerProfileID string       `json:"seller_profile_id"`
	TenantID        string       `json:"tenant_id,omitempty"`
	PackageID       string       `json:"package_id"`
	BillingCycle    BillingCycle `json:"billing_cycle"`
	StartAt         time.Time    `json:"start_at"`
	EndAt           time.Time    `json:"end_at"`
	CouponCode      string       `json:"coupon_code,omitempty"`
	DiscountPercent int          `json:"discount_percent,omitempty"`
	Limits          PlanLimits   `json:"limits"`
	Timestamp       time.Time    `json:"ts"`
}

// SubscriptionExpiredEvent — emitted when an active sub hits end_at and is
// not renewed. A separate SubscriptionDowngradedToNEvent may follow.
type SubscriptionExpiredEvent struct {
	SubscriptionID  string    `json:"subscription_id"`
	SellerProfileID string    `json:"seller_profile_id"`
	ExpiredAt       time.Time `json:"expired_at"`
	Timestamp       time.Time `json:"ts"`
}

// SubscriptionDowngradedToNEvent — emitted after expiry when the seller is
// auto-assigned the free N tier. Purchased credits are retained.
type SubscriptionDowngradedToNEvent struct {
	SellerProfileID string     `json:"seller_profile_id"`
	FromPackageID   string     `json:"from_package_id"`
	Limits          PlanLimits `json:"limits"`
	Timestamp       time.Time  `json:"ts"`
}

// SubscriptionRenewalDueEvent — emitted T-7 and T-1 days before end_at.
type SubscriptionRenewalDueEvent struct {
	SubscriptionID  string    `json:"subscription_id"`
	SellerProfileID string    `json:"seller_profile_id"`
	EndAt           time.Time `json:"end_at"`
	DaysRemaining   int       `json:"days_remaining"`
	Timestamp       time.Time `json:"ts"`
}

// WalletCreditedEvent — emitted by wallet-service on every credit mutation.
type WalletCreditedEvent struct {
	OwnerID     string    `json:"owner_id"`
	Credits     int64     `json:"credits"`
	Balance     int64     `json:"balance"`
	Reason      string    `json:"reason"`
	ReferenceID string    `json:"reference_id,omitempty"`
	Timestamp   time.Time `json:"ts"`
}

// WalletDebitedEvent — emitted by wallet-service on every debit mutation.
type WalletDebitedEvent struct {
	OwnerID     string    `json:"owner_id"`
	Credits     int64     `json:"credits"`
	Balance     int64     `json:"balance"`
	Reason      string    `json:"reason"`
	ReferenceID string    `json:"reference_id,omitempty"`
	Timestamp   time.Time `json:"ts"`
}

// WalletLowBalanceEvent — emitted when a seller wallet falls below threshold.
type WalletLowBalanceEvent struct {
	OwnerID   string    `json:"owner_id"`
	Balance   int64     `json:"balance"`
	Threshold int64     `json:"threshold"`
	Timestamp time.Time `json:"ts"`
}

// PromoPushedEvent — stage 1. Channel-service emits when a seller pushes a
// promotion. Wallet-service debits seller 10 credits and credits the company
// wallet selected by CompanyWalletScope, and opens a promo_context.
type PromoPushedEvent struct {
	PromoID         string             `json:"promo_id"`
	SellerProfileID string             `json:"seller_profile_id"`
	PromoType       string             `json:"promo_type"` // news | update | event | deal
	Locality        string             `json:"locality"`
	TargetViews     int64              `json:"target_views"`
	CompanyScope    CompanyWalletScope `json:"company_wallet_scope"`
	TenantID        string             `json:"tenant_id,omitempty"`
	Timestamp       time.Time          `json:"ts"`
}

// PromoViewedEvent — stage 2. Channel-service emits on each user view.
// Wallet moves 2 credits from the company wallet to the user wallet, up to
// target_views, then marks the context expired.
type PromoViewedEvent struct {
	PromoID       string    `json:"promo_id"`
	UserProfileID string    `json:"user_profile_id"`
	Timestamp     time.Time `json:"ts"`
}

// PromoContextExpiredEvent — wallet-service emits when target_views is reached
// or TTL elapses. Channel-service stops serving the promo in the locality.
type PromoContextExpiredEvent struct {
	PromoID     string    `json:"promo_id"`
	Reason      string    `json:"reason"` // target_reached | ttl | cancelled
	FinalViews  int64     `json:"final_views"`
	Timestamp   time.Time `json:"ts"`
}

// CouponRedeemedEvent — coupon-service emits after a subscription redeems.
type CouponRedeemedEvent struct {
	CouponCode      string    `json:"coupon_code"`
	SellerProfileID string    `json:"seller_profile_id"`
	SalesmanID      string    `json:"salesman_id,omitempty"`
	CampaignID      string    `json:"campaign_id,omitempty"`
	DiscountPercent int       `json:"discount_percent"`
	SubscriptionID  string    `json:"subscription_id"`
	Timestamp       time.Time `json:"ts"`
}

// Marshal is a small helper so producers don't have to import encoding/json
// directly and so we have a single place to add future instrumentation.
func Marshal(v any) ([]byte, error) { return json.Marshal(v) }
