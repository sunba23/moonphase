package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/sunba23/moonphase/internal/catalog"
	"github.com/sunba23/moonphase/internal/config"
	"github.com/sunba23/moonphase/internal/db"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: catalog <ingest> [flags]")
	}

	switch os.Args[1] {
	case "ingest":
		return runIngest(os.Args[2:])
	default:
		return fmt.Errorf("unknown command %q, expected: ingest", os.Args[1])
	}
}

func runIngest(args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	file := fs.String("file", "", "path to a MoonBoard board-edition export JSON file")
	dryRun := fs.Bool("dry-run", false, "parse and validate without writing to the database")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *file == "" {
		return errors.New("catalog ingest: --file is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx := context.Background()

	pool, err := db.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	ingester := catalog.NewIngester(pool)

	summary, err := ingester.IngestFile(ctx, *file, *dryRun)
	printSummary(summary)
	if err != nil {
		return fmt.Errorf("ingest %s: %w", *file, err)
	}

	if len(summary.Errors) > 0 {
		return fmt.Errorf("catalog ingest: %d problem(s) failed", len(summary.Errors))
	}

	return nil
}

func printSummary(s catalog.Summary) {
	fmt.Printf("problems seen:       %d\n", s.ProblemsSeen)
	fmt.Printf("ingested:            %d\n", s.Ingested)
	fmt.Printf("skipped (inactive):  %d\n", s.SkippedInactive)
	fmt.Printf("skipped (deleted):   %d\n", s.SkippedDeleted)
	fmt.Printf("configs written:     %d\n", s.ConfigsWritten)
	fmt.Printf("moves written:       %d\n", s.MovesWritten)
	fmt.Printf("new holds found:     %d\n", s.NewHoldsFound)

	if len(s.Errors) > 0 {
		fmt.Printf("errors:              %d\n", len(s.Errors))
		for _, e := range s.Errors {
			fmt.Printf("  - %v\n", e)
		}
	}
}
