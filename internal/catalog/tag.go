package catalog

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

// maxInteractiveModifiers caps how many modifiers the interactive tagger
// asks for per hold.
const maxInteractiveModifiers = 2

// primaryTypeKeys maps a single digit keystroke to the primary hold type it
// selects. Digits (not letters) avoid the pinch/pocket "both start with p"
// clash and need no memorization since the legend is printed at every
// prompt.
var primaryTypeKeys = map[byte]string{
	'1': "crimp",
	'2': "sloper",
	'3': "pinch",
	'4': "jug",
	'5': "pocket",
}

// modifierKeys maps a single digit keystroke to a modifier. Any keystroke
// outside this set (including Enter or '0') ends the modifier sub-loop for
// the current hold without being treated as an error or a quit.
var modifierKeys = map[byte]string{
	'1': "sharp",
	'2': "rounded",
	'3': "incut",
	'4': "sloping",
	'5': "small",
	'6': "large",
	'7': "positive",
	'8': "textured",
}

const primaryTypeLegend = "[1]crimp [2]sloper [3]pinch [4]jug [5]pocket [q]uit"
const modifierLegend = "[1]sharp [2]rounded [3]incut [4]sloping [5]small [6]large [7]positive [8]textured [other]done"

// readByte reads one byte from r. 'q', 'Q', and Ctrl-C (0x03, which arrives
// as a plain byte in raw terminal mode since ISIG is disabled) all report
// quit=true, as does a clean EOF (keys exhausted) -- both mean "stop the
// session, not an error".
func readByte(r *bufio.Reader) (b byte, quit bool, err error) {
	b, err = r.ReadByte()
	if errors.Is(err, io.EOF) {
		return 0, true, nil
	}
	if err != nil {
		return 0, false, err
	}
	if b == 'q' || b == 'Q' || b == 0x03 {
		return 0, true, nil
	}
	return b, false, nil
}

// readPrimaryTypeKey blocks until a valid primary-type digit or a quit key
// is read, reprompting on anything else since a primary type is mandatory.
func readPrimaryTypeKey(r *bufio.Reader, log io.Writer) (primary string, quit bool, err error) {
	for {
		b, quit, err := readByte(r)
		if err != nil || quit {
			return "", quit, err
		}
		if pt, ok := primaryTypeKeys[b]; ok {
			return pt, false, nil
		}
		_, _ = fmt.Fprintf(log, "\r\n  invalid key %q, try again: ", string(b))
	}
}

// readModifierKey reads one keystroke. A recognized modifier digit is
// returned as-is; any other non-quit key means "done adding modifiers for
// this hold" (ok=false), not an error.
func readModifierKey(r *bufio.Reader) (modifier string, ok, quit bool, err error) {
	b, quit, err := readByte(r)
	if err != nil || quit {
		return "", false, quit, err
	}
	if m, ok := modifierKeys[b]; ok {
		return m, true, false, nil
	}
	return "", false, false, nil
}

// runTagLoop drives the interactive tagging session against rows (mutated
// in place and also returned), reading one raw keystroke at a time from
// keys. Rows with a non-blank PrimaryType are skipped without prompting, so
// re-running against a partially-tagged seed only asks about what's left.
// save is invoked after every hold -- not just at the end -- so the CSV on
// disk is never more than one hold stale; if a quit key, Ctrl-C, or EOF is
// encountered, the loop stops and returns rows as tagged so far with no
// error.
func runTagLoop(rows []HoldRow, keys io.Reader, log io.Writer, save func([]HoldRow) error) ([]HoldRow, error) {
	SortHoldRows(rows)
	br := bufio.NewReader(keys)

	for i := range rows {
		row := &rows[i]
		if row.PrimaryType != "" {
			continue
		}

		_, _ = fmt.Fprintf(log, "[%s] %s: ", row.GridRef, primaryTypeLegend)
		primary, quit, err := readPrimaryTypeKey(br, log)
		if err != nil {
			return rows, fmt.Errorf("catalog: read primary type for %s: %w", row.GridRef, err)
		}
		if quit {
			return rows, nil
		}
		row.PrimaryType = primary
		_, _ = fmt.Fprintf(log, "%s\r\n", primary)

		var mods []string
		modQuit := false
		for len(mods) < maxInteractiveModifiers {
			_, _ = fmt.Fprintf(log, "  modifier %d/%d %s: ", len(mods)+1, maxInteractiveModifiers, modifierLegend)
			m, ok, quit, err := readModifierKey(br)
			if err != nil {
				return rows, fmt.Errorf("catalog: read modifier for %s: %w", row.GridRef, err)
			}
			if quit {
				modQuit = true
				break
			}
			if !ok {
				_, _ = fmt.Fprint(log, "done\r\n")
				break
			}
			mods = append(mods, m)
			_, _ = fmt.Fprintf(log, "%s\r\n", m)
		}
		row.Modifiers = mods

		if err := save(rows); err != nil {
			return rows, fmt.Errorf("catalog: save after %s: %w", row.GridRef, err)
		}

		if modQuit {
			return rows, nil
		}
	}

	return rows, nil
}

// loadExistingCSV reads a previously-written inventory CSV, if one exists at
// path. A missing file is reported via the returned error satisfying
// os.IsNotExist -- callers treat that as "no prior progress", not a
// failure.
func loadExistingCSV(path string) ([]HoldRow, error) {
	f, err := os.Open(path) //nolint:gosec // path is operator-supplied (CLI flag or derived from a validated board year), not untrusted input
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	return ReadTagsCSV(f)
}

// mergeExistingProgress overlays any non-blank row from existing onto dst by
// grid_ref, so progress from an earlier `holds tag` run that hasn't been
// pushed to the DB yet via `load-tags` is never lost or re-prompted.
func mergeExistingProgress(dst, existing []HoldRow) {
	byRef := make(map[string]HoldRow, len(existing))
	for _, r := range existing {
		if r.PrimaryType != "" {
			byRef[r.GridRef] = r
		}
	}
	for i := range dst {
		if o, ok := byRef[dst[i].GridRef]; ok {
			dst[i].PrimaryType = o.PrimaryType
			dst[i].Modifiers = o.Modifiers
		}
	}
}

// RunInteractiveTag runs one interactive tagging session for holdsetup,
// seeding from the DB inventory (merged with any prior progress already
// written to outPath) and writing the result to outPath after every hold.
// It never writes to the database -- `catalog holds load-tags` remains the
// only path that persists tags to Postgres.
func RunInteractiveTag(ctx context.Context, store *HoldStore, holdsetup int, outPath string) error {
	fd := int(os.Stdin.Fd()) //nolint:gosec // stdin's fd is always a small non-negative int in practice
	if !term.IsTerminal(fd) {
		return errors.New("catalog: holds tag requires an interactive terminal (stdin is not a TTY)")
	}

	rows, err := store.Inventory(ctx, holdsetup)
	if err != nil {
		return fmt.Errorf("catalog: load inventory: %w", err)
	}

	existing, err := loadExistingCSV(outPath)
	switch {
	case err == nil:
		mergeExistingProgress(rows, existing)
	case os.IsNotExist(err):
		// No prior progress at outPath -- start fresh from the DB seed.
	default:
		return fmt.Errorf("catalog: read existing %s: %w", outPath, err)
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("catalog: enter raw terminal mode: %w", err)
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	// A raw terminal has ISIG disabled, so Ctrl+C arrives as a plain byte
	// (see readByte) rather than SIGINT -- but an external SIGTERM/SIGHUP
	// (e.g. `kill`, or the terminal window closing) still kills the
	// process directly. Without this handler that leaves the operator's
	// shell stuck in raw mode until they run `stty sane`.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)
	go func() {
		if _, ok := <-sigCh; ok {
			_ = term.Restore(fd, oldState)
			os.Exit(1)
		}
	}()

	save := func(current []HoldRow) error {
		f, err := os.Create(outPath) //nolint:gosec // outPath is operator-supplied (CLI flag or derived from a validated board year), not untrusted input
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		return WriteInventoryCSV(f, current)
	}

	tagged, err := runTagLoop(rows, os.Stdin, os.Stdout, save)
	if err != nil {
		return err
	}

	done := 0
	for _, r := range tagged {
		if r.PrimaryType != "" {
			done++
		}
	}
	fmt.Printf("\r\n%d / %d tagged, written to %s\r\n", done, len(tagged), outPath)

	return nil
}
