package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/pendig/rute-bayar/internal/config"
	"github.com/pendig/rute-bayar/internal/storage/sqlite"
)

func dbCommand(ctx context.Context, w io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("db command requires a subcommand")
	}

	switch args[0] {
	case "migrate":
		cfg := config.Load()
		store, err := sqlite.Open(ctx, cfg.DBPath)
		if err != nil {
			return err
		}
		defer store.Close()

		if err := store.Migrate(ctx); err != nil {
			return err
		}

		fmt.Fprintf(w, "database migrated: %s\n", cfg.DBPath)
		return nil
	default:
		return fmt.Errorf("unknown db subcommand %q", args[0])
	}
}
