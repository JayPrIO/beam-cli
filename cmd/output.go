package cmd

import (
	"fmt"
	"sort"

	"github.com/jayprio/beam-cli/internal/errs"
	"github.com/jayprio/beam-cli/internal/sync"
)

// printPlan prints a Plan's summary counts, and (with --verbose or
// --dry-run) every non-skip action per path.
func printPlan(plan sync.Plan, dryRun bool) {
	counts := plan.Counts()
	prefix := ""
	if dryRun {
		prefix = "[dry-run] "
	}
	fmt.Printf("%splan: %d add, %d update, %d skip, %d conflict\n",
		prefix, counts[sync.ActionAdd], counts[sync.ActionUpdate], counts[sync.ActionSkip], counts[sync.ActionConflict])

	if !verbose && !dryRun {
		return
	}

	entries := make([]sync.PlanEntry, 0, len(plan))
	for _, e := range plan {
		if e.Action != sync.ActionSkip {
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	for _, e := range entries {
		fmt.Printf("%s  %-8s %s\n", prefix, e.Action, e.Path)
	}
}

func printFileErrors(r *errs.Report) {
	for _, fe := range r.Errors {
		fmt.Printf("  error  %-20s %s: %v\n", fe.Op, fe.Path, fe.Err)
	}
}

func printConflicts(r *errs.Report) {
	for _, c := range r.Conflicts {
		fmt.Printf("  conflict  %s\n", c.Path)
	}
}
