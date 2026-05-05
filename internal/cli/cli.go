package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pena-digital/rute-bayar/internal/build"
	"github.com/pena-digital/rute-bayar/internal/daemon"
	"github.com/pena-digital/rute-bayar/internal/forwarding"
	"github.com/pena-digital/rute-bayar/internal/storage/memory"
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
		return onboard(stdout)
	case "provider":
		return providerCommand(stdout, args[1:])
	case "pay":
		return payCommand(stdout, args[1:])
	case "webhook":
		return webhookCommand(ctx, stdout, stderr, args[1:])
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
  rute-bayar provider list
  rute-bayar provider test
  rute-bayar pay create
  rute-bayar pay status
  rute-bayar pay refund
  rute-bayar webhook serve --addr :8080
  rute-bayar webhook forward list
  rute-bayar webhook forward add
  rute-bayar reconcile
  rute-bayar version`)
}

func onboard(w io.Writer) error {
	fmt.Fprintln(w, "onboarding wizard scaffold is ready.")
	fmt.Fprintln(w, "Next implementation: collect provider credentials, validate them, and persist to SQLite.")
	return nil
}

func providerCommand(w io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("provider command requires a subcommand")
	}
	switch args[0] {
	case "list":
		fmt.Fprintln(w, "midtrans")
		fmt.Fprintln(w, "xendit")
		return nil
	case "test":
		fmt.Fprintln(w, "provider test scaffold is ready.")
		return nil
	default:
		return fmt.Errorf("unknown provider subcommand %q", args[0])
	}
}

func payCommand(w io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("pay command requires a subcommand")
	}
	switch args[0] {
	case "create", "status", "refund":
		fmt.Fprintf(w, "pay %s scaffold is ready.\n", args[0])
		return nil
	default:
		return fmt.Errorf("unknown pay subcommand %q", args[0])
	}
}

func webhookCommand(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("webhook command requires a subcommand")
	}

	switch args[0] {
	case "serve":
		fs := flag.NewFlagSet("webhook serve", flag.ContinueOnError)
		fs.SetOutput(stderr)
		addr := fs.String("addr", ":8080", "daemon listen address")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		store := memory.NewForwardingStore()
		srv := daemon.NewServer(*addr, forwarding.NewService(store))
		fmt.Fprintf(stdout, "Rute Bayar webhook daemon listening on %s\n", *addr)
		return srv.ListenAndServe()
	case "replay":
		fmt.Fprintln(stdout, "webhook replay scaffold is ready.")
		return nil
	case "forward":
		return webhookForwardCommand(ctx, stdout, args[1:])
	default:
		return fmt.Errorf("unknown webhook subcommand %q", args[0])
	}
}

func webhookForwardCommand(_ context.Context, w io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("webhook forward command requires a subcommand")
	}

	switch args[0] {
	case "list":
		fmt.Fprintln(w, "no forwarding targets configured yet")
	case "add":
		fmt.Fprintln(w, "webhook forwarding target add scaffold is ready.")
	case "update":
		fmt.Fprintln(w, "webhook forwarding target update scaffold is ready.")
	case "remove":
		fmt.Fprintln(w, "webhook forwarding target remove scaffold is ready.")
	default:
		return fmt.Errorf("unknown webhook forward subcommand %q", strings.Join(args, " "))
	}
	return nil
}

