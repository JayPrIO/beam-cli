package sync

import "testing"

func state(hash string) FileState { return FileState{SHA256: hash} }

func actionOf(t *testing.T, plan Plan, path string) Action {
	t.Helper()
	for _, e := range plan {
		if e.Path == path {
			return e.Action
		}
	}
	t.Fatalf("no plan entry for %q", path)
	return -1
}

func TestDiffPush(t *testing.T) {
	local := Manifest{
		"new.txt":       state("h1"),
		"changed.txt":   state("h2new"),
		"unchanged.txt": state("h3"),
	}
	lastSynced := Manifest{
		"changed.txt":   state("h2old"),
		"unchanged.txt": state("h3"),
	}

	plan := DiffPush(local, lastSynced)
	if got := actionOf(t, plan, "new.txt"); got != ActionAdd {
		t.Errorf("new.txt = %v, want add", got)
	}
	if got := actionOf(t, plan, "changed.txt"); got != ActionUpdate {
		t.Errorf("changed.txt = %v, want update", got)
	}
	if got := actionOf(t, plan, "unchanged.txt"); got != ActionSkip {
		t.Errorf("unchanged.txt = %v, want skip", got)
	}
}

func TestDiffPullConflict(t *testing.T) {
	lastSynced := Manifest{"app.js": state("base")}
	vaultNow := Manifest{"app.js": state("remote-change")}
	localNow := Manifest{"app.js": state("local-change")}

	plan := DiffPull(vaultNow, localNow, lastSynced)
	if got := actionOf(t, plan, "app.js"); got != ActionConflict {
		t.Errorf("app.js = %v, want conflict (changed on both sides)", got)
	}
}

func TestDiffPullUpdateWhenOnlyRemoteChanged(t *testing.T) {
	lastSynced := Manifest{"app.js": state("base")}
	vaultNow := Manifest{"app.js": state("remote-change")}
	localNow := Manifest{"app.js": state("base")} // unchanged locally

	plan := DiffPull(vaultNow, localNow, lastSynced)
	if got := actionOf(t, plan, "app.js"); got != ActionUpdate {
		t.Errorf("app.js = %v, want update (only remote changed)", got)
	}
}

func TestDiffPullAddForNewLocalFile(t *testing.T) {
	// Never synced before, doesn't exist locally yet: a plain add, not
	// a conflict, even though it's technically "new" on both the
	// lastSynced-absence and local-absence axes.
	vaultNow := Manifest{"new-from-vault.txt": state("h1")}
	plan := DiffPull(vaultNow, Manifest{}, Manifest{})
	if got := actionOf(t, plan, "new-from-vault.txt"); got != ActionAdd {
		t.Errorf("new-from-vault.txt = %v, want add", got)
	}
}

func TestDiffPullSkipWhenNothingChanged(t *testing.T) {
	lastSynced := Manifest{"app.js": state("same")}
	vaultNow := Manifest{"app.js": state("same")}
	localNow := Manifest{"app.js": state("same")}

	plan := DiffPull(vaultNow, localNow, lastSynced)
	if got := actionOf(t, plan, "app.js"); got != ActionSkip {
		t.Errorf("app.js = %v, want skip", got)
	}
}
