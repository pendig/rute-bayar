package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/pendig/rute-bayar/internal/build"
)

func Execute(args []string) error {
	return ExecuteWithIO(context.Background(), args, os.Stdout, os.Stderr)
}

func ExecuteWithIO(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printHelp(stdout)
		return nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		printHelp(stdout)
		return nil
	case "version", "--version":
		fmt.Fprintf(stdout, "%s %s\n", build.Name, build.Version)
		return nil
	case "onboard":
		return onboard(ctx, stdout, stderr, args[1:])
	case "provider":
		return providerCommand(ctx, stdout, args[1:])
	case "pay":
		return payCommand(ctx, stdout, stderr, args[1:])
	case "webhook":
		return webhookCommand(ctx, stdout, stderr, args[1:])
	case "db":
		return dbCommand(ctx, stdout, args[1:])
	case "reconcile":
		fmt.Fprintln(stdout, "reconcile command scaffold is ready; implementation comes next.")
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `Rute Bayar

Usage:
  rute-bayar onboard
  rute-bayar onboard xendit --secret-key <key>
  rute-bayar onboard midtrans --server-key <key> --client-key <key> --merchant-id <id>
  rute-bayar provider list
  rute-bayar provider accounts
  rute-bayar provider test
  rute-bayar pay create --provider midtrans --method bank_transfer --bank bca
  rute-bayar pay create --provider xendit --method payment_link --reference rb-0001 --amount 15000
  rute-bayar pay status --provider midtrans --reference rb-0001
  rute-bayar pay refund
	rute-bayar webhook serve --addr :8080
	rute-bayar webhook forward list
	rute-bayar webhook forward add
	rute-bayar webhook forward update
	rute-bayar webhook forward remove
  rute-bayar db migrate
  rute-bayar reconcile
  rute-bayar version`)
}
