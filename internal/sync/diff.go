package sync

// Action classifies what a sync operation would do with a given path.
type Action int

const (
	ActionAdd Action = iota
	ActionUpdate
	ActionSkip     // unchanged, nothing to do
	ActionConflict // changed on both sides since last known sync (pull only)
)

func (a Action) String() string {
	switch a {
	case ActionAdd:
		return "add"
	case ActionUpdate:
		return "update"
	case ActionSkip:
		return "skip"
	case ActionConflict:
		return "conflict"
	default:
		return "unknown"
	}
}

// PlanEntry is one path's classification in a Plan.
type PlanEntry struct {
	Path   string
	Action Action
}

// Plan is the full set of per-path decisions a push or pull would make.
// --dry-run computes and prints a Plan without touching any file.
type Plan []PlanEntry

func (p Plan) Counts() map[Action]int {
	counts := make(map[Action]int)
	for _, e := range p {
		counts[e.Action]++
	}
	return counts
}

// changed reports whether b differs from a (different content, per
// hash — size/mtime alone are what trigger a hash check upstream in
// BuildManifest, so by the time two FileStates are compared here,
// SHA256 is the authoritative signal).
func changed(a, b FileState) bool {
	return a.SHA256 != b.SHA256
}

// DiffPush compares the local working-tree manifest against the vault's
// last-known-synced manifest to decide what push would add/update.
// Push never produces conflicts: it is a one-directional overwrite of
// the vault with the local tree's current state.
func DiffPush(local, vaultLastSynced Manifest) Plan {
	plan := make(Plan, 0, len(local))
	for path, localState := range local {
		remoteState, existed := vaultLastSynced[path]
		switch {
		case !existed:
			plan = append(plan, PlanEntry{Path: path, Action: ActionAdd})
		case changed(remoteState, localState):
			plan = append(plan, PlanEntry{Path: path, Action: ActionUpdate})
		default:
			plan = append(plan, PlanEntry{Path: path, Action: ActionSkip})
		}
	}
	return plan
}

// DiffPull compares the vault's current manifest, the local working
// tree's current manifest, and the last-known-synced manifest (the
// common ancestor of both) to classify each path as add/update/skip/
// conflict. A conflict is a path that changed in the vault AND changed
// locally relative to lastSynced — beam does not attempt to merge, so
// these are surfaced for the user to resolve manually.
func DiffPull(vaultNow, localNow, lastSynced Manifest) Plan {
	plan := make(Plan, 0, len(vaultNow))
	for path, remoteState := range vaultNow {
		syncedState, wasSynced := lastSynced[path]
		localState, existsLocally := localNow[path]

		remoteChanged := !wasSynced || changed(syncedState, remoteState)
		localChanged := existsLocally && wasSynced && changed(syncedState, localState)
		// A local file that was never part of a prior sync is not a
		// "change" for conflict purposes — it simply doesn't exist
		// locally yet from pull's point of view, so it's a plain add.

		switch {
		case remoteChanged && localChanged:
			plan = append(plan, PlanEntry{Path: path, Action: ActionConflict})
		case !existsLocally:
			plan = append(plan, PlanEntry{Path: path, Action: ActionAdd})
		case remoteChanged:
			plan = append(plan, PlanEntry{Path: path, Action: ActionUpdate})
		default:
			plan = append(plan, PlanEntry{Path: path, Action: ActionSkip})
		}
	}
	return plan
}
