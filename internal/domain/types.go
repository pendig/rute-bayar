package domain

import (
	"encoding/json"
	"time"
)

type ProviderCode string

const (
	ProviderMidtrans ProviderCode = "midtrans"
	ProviderXendit   ProviderCode = "xendit"
)

type Environment string

const (
	EnvironmentSandbox    Environment = "sandbox"
	EnvironmentProduction Environment = "production"
)

type PaymentStatus string

const (
	PaymentStatusPending         PaymentStatus = "pending"
	PaymentStatusPaid            PaymentStatus = "paid"
	PaymentStatusFailed          PaymentStatus = "failed"
	PaymentStatusExpired         PaymentStatus = "expired"
	PaymentStatusCancelled       PaymentStatus = "cancelled"
	PaymentStatusRefunded        PaymentStatus = "refunded"
	PaymentStatusPartialRefunded PaymentStatus = "partial_refunded"
	PaymentStatusSettled         PaymentStatus = "settled"
	PaymentStatusAuthorized      PaymentStatus = "authorized"
	PaymentStatusCaptured        PaymentStatus = "captured"
)

type ProviderAccount struct {
	ID             string
	ProviderCode   ProviderCode
	Environment    Environment
	DisplayName    string
	CredentialJSON json.RawMessage
	ConfigJSON     json.RawMessage
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type PaymentIntent struct {
	ID           string
	ExternalRef  string
	ProviderCode ProviderCode
	Amount       int64
	Currency     string
	Status       PaymentStatus
	MetadataJSON json.RawMessage
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type PaymentAttempt struct {
	ID                string
	PaymentIntentID   string
	ProviderCode      ProviderCode
	RequestJSON       json.RawMessage
	ResponseJSON      json.RawMessage
	Status            PaymentStatus
	ProviderReference string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PaymentStatusCheck struct {
	ID                string
	PaymentIntentID   string
	ProviderCode      ProviderCode
	RequestJSON       json.RawMessage
	ResponseJSON      json.RawMessage
	Status            PaymentStatus
	ProviderReference string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type WebhookEvent struct {
	ID               string
	ProviderCode     ProviderCode
	ProviderEventID  string
	EventType        string
	SignatureValid   bool
	PayloadJSON      json.RawMessage
	HeadersJSON      json.RawMessage
	ReceivedAt       time.Time
	ProcessedAt      *time.Time
	ProcessingStatus string
}

type Refund struct {
	ID                string
	PaymentIntentID   string
	ProviderCode      ProviderCode
	Amount            int64
	Status            PaymentStatus
	RequestJSON       json.RawMessage
	ResponseJSON      json.RawMessage
	ProviderReference string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
