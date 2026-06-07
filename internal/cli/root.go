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
		return reconcileCommand(ctx, stdout, stderr, args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `Rute Bayar

Usage:
  rutebayar onboard
  rutebayar onboard xendit --secret-key <key>
  rutebayar onboard midtrans --server-key <key> --client-key <key> --merchant-id <id>
  rutebayar onboard doku --client-id <id> --secret-key <key>
  rutebayar provider list
  rutebayar provider accounts
  rutebayar provider test
  rutebayar pay create --provider midtrans --method bank_transfer --bank bca
  rutebayar pay create --provider xendit --method payment_link --reference rb-0001 --amount 15000
  rutebayar pay create --provider doku --method checkout --reference rb-0001 --amount 15000
  rutebayar pay status --provider midtrans --reference rb-0001
  rutebayar pay refund --provider xendit --reference rb-0001
	rutebayar webhook serve --addr :8080 --mode webhook|api|all
	rutebayar webhook replay --event-id <id>
	rutebayar webhook forward list
	rutebayar webhook forward add
	rutebayar webhook forward update
	rutebayar webhook forward remove
	rutebayar webhook forward attempts list
	rutebayar webhook forward attempts show <attempt-id>
	rutebayar webhook forward attempts retry <attempt-id>
  rutebayar db migrate
  rutebayar reconcile --provider midtrans --reference rb-0001
  rutebayar version`)
}
