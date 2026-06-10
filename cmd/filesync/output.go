package main

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/justinIs/filesync_g/internal/config"
	"github.com/justinIs/filesync_g/internal/scan"
	"github.com/justinIs/filesync_g/internal/track"
)

// hashDisplayLen is how many leading hex chars of a hash to show — enough to
// eyeball, short enough to keep table rows readable.
const hashDisplayLen = 12

// shortHash truncates a hash for display; empty hashes render as a dash.
func shortHash(h string) string {
	if h == "" {
		return "—"
	}
	if len(h) > hashDisplayLen {
		return h[:hashDisplayLen]
	}
	return h
}

// formatTime renders a timestamp without the monotonic-clock noise of %v.
func formatTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

// printConfig prints the loaded config, hiding internal fields via String().
func printConfig(w io.Writer, cfg *config.Config) {
	if _, err := fmt.Fprintf(w, "config: %s\n", cfg); err != nil {
		fmt.Fprintf(os.Stderr, "printConfig: error formatting: %v", err)
	}
}

// printEntries prints the scanned files as an aligned table.
func printEntries(out io.Writer, entries []scan.Entry) {
	if len(entries) == 0 {
		if _, err := fmt.Fprintln(out, "\nNo files found."); err != nil {
			fmt.Fprintf(os.Stderr, "printEntries: error formatting %v", err)
		}
		return
	}
	if _, err := fmt.Fprintf(out, "\nScanned files (%d):\n", len(entries)); err != nil {
		fmt.Fprintf(os.Stderr, "printEntries: error formatting %v", err)
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "PATH\tSIZE\tMODIFIED"); err != nil {
		fmt.Fprintf(os.Stderr, "printEntries: error formatting %v", err)
	}
	for _, e := range entries {
		if _, err := fmt.Fprintf(w, "%s\t%d\t%s\n", e.RelPath, e.Size, formatTime(e.ModTime)); err != nil {
			fmt.Fprintf(os.Stderr, "printEntries: error formatting %v", err)
		}
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "printEntries: error formatting %v", err)
	}
}

// printResults prints the change set as a single labeled table.
func printResults(out io.Writer, r track.CheckEntriesResult) {
	total := len(r.Updates) + len(r.Refreshes) + len(r.Deletes)
	if total == 0 {
		if _, err := fmt.Fprintln(out, "\nNo changes."); err != nil {
			fmt.Fprintf(os.Stderr, "printResults: error formatting: %v", err)
		}
		return
	}
	if _, err := fmt.Fprintf(out, "\nChanges (%d):\n", total); err != nil {
		fmt.Fprintf(os.Stderr, "printResults: error formatting: %v", err)
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "CHANGE\tPATH\tSIZE\tMODIFIED\tHASH"); err != nil {
		fmt.Fprintf(os.Stderr, "printResults: error formatting: %v", err)
	}
	writeChangeRows(w, "update", r.Updates)
	writeChangeRows(w, "refresh", r.Refreshes)
	writeChangeRows(w, "delete", r.Deletes)

	if err := w.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "printResults: error formatting: %v", err)
	}
	if _, err := fmt.Fprintf(out, "existing: %d\nuntouched: %d\n", r.ExistingCount, r.UntouchedCount); err != nil {
		fmt.Fprintf(os.Stderr, "printResults: error formatting: %v", err)
	}
}

// writeChangeRows writes one tab-separated row per file under the given label.
func writeChangeRows(w io.Writer, change string, files []track.ManifestFileInfo) {
	for _, fi := range files {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
			change, fi.RelPath, fi.Size, formatTime(fi.ModTime), shortHash(fi.Hash)); err != nil {
			fmt.Fprintf(os.Stderr, "writeChangeRows: error formatting: %v", err)
		}
	}
}

// printSummary prints a one-line tally of the run.
func printSummary(out io.Writer, scanned int, r track.CheckEntriesResult) {
	if _, err := fmt.Fprintf(out, "\nfilesync: %d files scanned — %d updates, %d refreshes, %d deletes\n",
		scanned, len(r.Updates), len(r.Refreshes), len(r.Deletes)); err != nil {
		fmt.Fprintf(os.Stderr, "printSummary: error formatting: %v", err)
	}
}
