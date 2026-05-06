package cli

import (
	"fmt"
	"io"

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
