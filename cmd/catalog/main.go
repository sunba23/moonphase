package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

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
		return errors.New("usage: catalog <ingest|holds> [flags]")
	}

	switch os.Args[1] {
	case "ingest":
		return runIngest(os.Args[2:])
	case "holds":
		return runHolds(os.Args[2:])
	default:
		return fmt.Errorf("unknown command %q, expected: ingest, holds", os.Args[1])
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

	pool, cleanup, err := connectDB()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()

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

func runHolds(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: catalog holds <inventory|load-tags|status> [flags]")
	}

	switch args[0] {
	case "inventory":
		return runHoldsInventory(args[1:])
	case "load-tags":
		return runHoldsLoadTags(args[1:])
	case "status":
		return runHoldsStatus(args[1:])
	case "tag":
		return runHoldsTag(args[1:])
	case "recompute-composition":
		return runHoldsRecomputeComposition(args[1:])
	default:
		return fmt.Errorf("unknown holds command %q, expected: inventory, load-tags, status, tag, recompute-composition", args[0])
	}
}

func runHoldsInventory(args []string) error {
	fs := flag.NewFlagSet("holds inventory", flag.ExitOnError)
	board := fs.String("board", "", "board year (2016, 2017, 2019, or 2024)")
	out := fs.String("out", "", "output CSV path (stdout if omitted)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *board == "" {
		return errors.New("catalog holds inventory: --board is required")
	}

	holdsetup, err := catalog.ResolveBoardYear(*board)
	if err != nil {
		return fmt.Errorf("catalog holds inventory: %w", err)
	}

	pool, cleanup, err := connectDB()
	if err != nil {
		return err
	}
	defer cleanup()

	store := catalog.NewHoldStore(pool)

	rows, err := store.Inventory(context.Background(), holdsetup)
	if err != nil {
		return fmt.Errorf("catalog holds inventory: %w", err)
	}

	w := os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return fmt.Errorf("catalog holds inventory: create %s: %w", *out, err)
		}
		defer func() { _ = f.Close() }()
		w = f
	}

	if err := catalog.WriteInventoryCSV(w, rows); err != nil {
		return fmt.Errorf("catalog holds inventory: %w", err)
	}

	return nil
}

func runHoldsLoadTags(args []string) error {
	fs := flag.NewFlagSet("holds load-tags", flag.ExitOnError)
	file := fs.String("file", "", "path to a hand-filled hold-tags CSV")
	board := fs.String("board", "", "board year (2016, 2017, 2019, or 2024)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *file == "" {
		return errors.New("catalog holds load-tags: --file is required")
	}
	if *board == "" {
		return errors.New("catalog holds load-tags: --board is required")
	}

	holdsetup, err := catalog.ResolveBoardYear(*board)
	if err != nil {
		return fmt.Errorf("catalog holds load-tags: %w", err)
	}

	f, err := os.Open(*file) //nolint:gosec // path is an operator-supplied CLI flag, not untrusted input
	if err != nil {
		return fmt.Errorf("catalog holds load-tags: open %s: %w", *file, err)
	}
	defer func() { _ = f.Close() }()

	rows, err := catalog.ReadTagsCSV(f)
	if err != nil {
		return fmt.Errorf("catalog holds load-tags: %w", err)
	}

	for _, r := range rows {
		if len(r.Modifiers) > catalog.MaxHoldModifiers {
			fmt.Fprintf(os.Stderr, "warning: %s has %d modifiers (max recommended %d)\n", r.GridRef, len(r.Modifiers), catalog.MaxHoldModifiers)
		}
	}

	pool, cleanup, err := connectDB()
	if err != nil {
		return err
	}
	defer cleanup()

	store := catalog.NewHoldStore(pool)

	if err := store.ApplyTags(context.Background(), holdsetup, rows); err != nil {
		return fmt.Errorf("catalog holds load-tags: %w", err)
	}

	tagged := 0
	for _, r := range rows {
		if r.PrimaryType != "" {
			tagged++
		}
	}
	fmt.Printf("applied tags: %d\n", tagged)

	return nil
}

func runHoldsStatus(args []string) error {
	fs := flag.NewFlagSet("holds status", flag.ExitOnError)
	board := fs.String("board", "", "board year (optional filter; 2016, 2017, 2019, or 2024)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var boardFilter *int
	if *board != "" {
		holdsetup, err := catalog.ResolveBoardYear(*board)
		if err != nil {
			return fmt.Errorf("catalog holds status: %w", err)
		}
		boardFilter = &holdsetup
	}

	pool, cleanup, err := connectDB()
	if err != nil {
		return err
	}
	defer cleanup()

	store := catalog.NewHoldStore(pool)

	statuses, err := store.Status(context.Background(), boardFilter)
	if err != nil {
		return fmt.Errorf("catalog holds status: %w", err)
	}

	for _, s := range statuses {
		fmt.Printf("%-4s %-14s: %d / %d tagged\n", catalog.BoardYears[s.Holdsetup], catalog.KnownBoards[s.Holdsetup], s.Tagged, s.Total)
	}

	return nil
}

func runHoldsTag(args []string) error {
	fs := flag.NewFlagSet("holds tag", flag.ExitOnError)
	board := fs.String("board", "", "board year (2016, 2017, 2019, or 2024)")
	out := fs.String("out", "", "output CSV path (default migrations/seed/holds/<year>.csv)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *board == "" {
		return errors.New("catalog holds tag: --board is required")
	}

	holdsetup, err := catalog.ResolveBoardYear(*board)
	if err != nil {
		return fmt.Errorf("catalog holds tag: %w", err)
	}

	outPath := *out
	if outPath == "" {
		outPath = filepath.Join("migrations", "seed", "holds", *board+".csv")
	}

	pool, cleanup, err := connectDB()
	if err != nil {
		return err
	}
	defer cleanup()

	store := catalog.NewHoldStore(pool)

	if err := catalog.RunInteractiveTag(context.Background(), store, holdsetup, outPath); err != nil {
		return fmt.Errorf("catalog holds tag: %w", err)
	}

	return nil
}

func runHoldsRecomputeComposition(args []string) error {
	fs := flag.NewFlagSet("holds recompute-composition", flag.ExitOnError)
	board := fs.String("board", "", "board year (2016, 2017, 2019, or 2024)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *board == "" {
		return errors.New("catalog holds recompute-composition: --board is required")
	}

	holdsetup, err := catalog.ResolveBoardYear(*board)
	if err != nil {
		return fmt.Errorf("catalog holds recompute-composition: %w", err)
	}

	pool, cleanup, err := connectDB()
	if err != nil {
		return err
	}
	defer cleanup()

	n, err := catalog.RecomputeHoldTypesForBoard(context.Background(), pool, holdsetup)
	if err != nil {
		return fmt.Errorf("catalog holds recompute-composition: %w", err)
	}

	fmt.Printf("recomputed composition for %d problems on board %s\n", n, *board)

	return nil
}

// connectDB loads config and opens a pool the way every catalog subcommand
// needs it. Callers must call cleanup (which closes the pool) when done.
func connectDB() (*pgxpool.Pool, func(), error) {
	_ = godotenv.Load() // no-op if .env doesn't exist, e.g. in production where vars are injected directly

	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}

	pool, err := db.New(context.Background(), cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to database: %w", err)
	}

	return pool, func() { pool.Close() }, nil
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
