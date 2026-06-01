package cli

import (
	"fmt"
	"io"

	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/paymentsvc"
	"github.com/pendig/rute-bayar/internal/provider"
)

func printPaymentCreate(w io.Writer, providerCode, reference string, response provider.CreatePaymentResponse) {
	fmt.Fprintln(w, "payment created")
	fmt.Fprintf(w, "provider: %s\n", providerCode)
	fmt.Fprintf(w, "reference: %s\n", reference)
	fmt.Fprintf(w, "status: %s\n", response.Status)
	if response.TransactionID != "" {
		fmt.Fprintf(w, "transaction_id: %s\n", response.TransactionID)
	}
	if response.PaymentType != "" {
		fmt.Fprintf(w, "payment_type: %s\n", response.PaymentType)
	}
	if response.VANumber != "" {
		fmt.Fprintf(w, "va_number: %s\n", response.VANumber)
	}
	if response.ExpiryTime != "" {
		fmt.Fprintf(w, "expiry_time: %s\n", response.ExpiryTime)
	}
	if response.RedirectURL != "" {
		fmt.Fprintf(w, "redirect_url: %s\n", response.RedirectURL)
	}
	if providerCode == string(domain.ProviderDoku) {
		fmt.Fprintln(w, "note: configure the matching Notification URL in DOKU Back Office per channel before relying on webhook callbacks")
	}
}

func printPaymentStatus(w io.Writer, providerCode, reference, providerReference string, response provider.PaymentStatusResponse) {
	fmt.Fprintln(w, "payment status")
	fmt.Fprintf(w, "provider: %s\n", providerCode)
	fmt.Fprintf(w, "reference: %s\n", reference)
	if providerReference != "" {
		fmt.Fprintf(w, "provider_reference: %s\n", providerReference)
	}
	fmt.Fprintf(w, "status: %s\n", response.Status)
	if response.StatusCode != "" {
		fmt.Fprintf(w, "status_code: %s\n", response.StatusCode)
	}
	if response.StatusMessage != "" {
		fmt.Fprintf(w, "status_message: %s\n", response.StatusMessage)
	}
	if response.TransactionID != "" {
		fmt.Fprintf(w, "transaction_id: %s\n", response.TransactionID)
	}
	if response.OrderID != "" {
		fmt.Fprintf(w, "order_id: %s\n", response.OrderID)
	}
	if response.PaymentType != "" {
		fmt.Fprintf(w, "payment_type: %s\n", response.PaymentType)
	}
	if response.TransactionStatus != "" {
		fmt.Fprintf(w, "transaction_status: %s\n", response.TransactionStatus)
	}
	if response.FraudStatus != "" {
		fmt.Fprintf(w, "fraud_status: %s\n", response.FraudStatus)
	}
	if response.VANumber != "" {
		fmt.Fprintf(w, "va_number: %s\n", response.VANumber)
	}
	if response.ExpiryTime != "" {
		fmt.Fprintf(w, "expiry_time: %s\n", response.ExpiryTime)
	}
	if response.RedirectURL != "" {
		fmt.Fprintf(w, "redirect_url: %s\n", response.RedirectURL)
	}
}

func printPaymentRefund(w io.Writer, providerCode string, result paymentsvc.RefundResult) {
	fmt.Fprintln(w, "payment refund")
	fmt.Fprintf(w, "provider: %s\n", providerCode)
	fmt.Fprintf(w, "reference: %s\n", result.Reference)
	if result.ProviderReference != "" {
		fmt.Fprintf(w, "provider_reference: %s\n", result.ProviderReference)
	}
	if result.RefundReference != "" {
		fmt.Fprintf(w, "refund_reference: %s\n", result.RefundReference)
	}
	fmt.Fprintf(w, "status: %s\n", result.Response.Status)
	if result.Response.PaymentRequestID != "" {
		fmt.Fprintf(w, "payment_request_id: %s\n", result.Response.PaymentRequestID)
	}
	if result.Response.PaymentSessionID != "" {
		fmt.Fprintf(w, "payment_session_id: %s\n", result.Response.PaymentSessionID)
	}
}

func printPaymentReconcile(w io.Writer, providerCode string, result paymentsvc.ReconcileResult) {
	fmt.Fprintln(w, "payment reconcile")
	fmt.Fprintf(w, "provider: %s\n", providerCode)
	fmt.Fprintf(w, "reference: %s\n", result.Reference)
	if result.ProviderReference != "" {
		fmt.Fprintf(w, "provider_reference: %s\n", result.ProviderReference)
	}
	fmt.Fprintf(w, "local_status: %s\n", result.LocalStatus)
	fmt.Fprintf(w, "provider_status: %s\n", result.ProviderStatus)
	fmt.Fprintf(w, "matched: %t\n", result.Matched)
	fmt.Fprintf(w, "updated: %t\n", result.Updated)
	if result.Response.StatusCode != "" {
		fmt.Fprintf(w, "status_code: %s\n", result.Response.StatusCode)
	}
	if result.Response.StatusMessage != "" {
		fmt.Fprintf(w, "status_message: %s\n", result.Response.StatusMessage)
	}
	if result.Response.TransactionID != "" {
		fmt.Fprintf(w, "transaction_id: %s\n", result.Response.TransactionID)
	}
	if result.Response.OrderID != "" {
		fmt.Fprintf(w, "order_id: %s\n", result.Response.OrderID)
	}
	if result.Response.PaymentType != "" {
		fmt.Fprintf(w, "payment_type: %s\n", result.Response.PaymentType)
	}
	if result.Response.TransactionStatus != "" {
		fmt.Fprintf(w, "transaction_status: %s\n", result.Response.TransactionStatus)
	}
}
