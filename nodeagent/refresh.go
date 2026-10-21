package nodeagent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/mhsanaei/3x-ui/v2/nodewire"
	"github.com/mhsanaei/3x-ui/v2/web/service"
)

// A node's local budget is acknowledged local bytes + the shared remaining
// allowance. Ordinary local downloads change both sides by equal amounts. They
// need a meter report, not a full rewrite of every client and inbound in SQLite.
func snapshotApplyKey(snap *nodewire.Snapshot, installed map[string]bool) string {
	budgets := make([]int64, len(snap.Accounts))
	for i, a := range snap.Accounts {
		budgets[i] = -1
		if a.Remaining >= 0 {
			budgets[i] = a.ReportedUsage + a.Remaining
		}
	}
	raw, _ := json.Marshal(struct {
		Revision  string
		Budgets   []int64
		Installed map[string]bool
		Files     map[string][]byte
	}{service.NodeSnapshotRevision(snap), budgets, installed, snap.Files})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (a *Agent) applySnapshot(snap *nodewire.Snapshot, installed map[string]bool) (string, error) {
	key := snapshotApplyKey(snap, installed)
	if a.runtime != nil && key == a.appliedKey {
		return service.NodeSnapshotRevision(snap), service.UpdateNodeSpeedUsage(snap.Accounts)
	}
	revision, err := service.ApplyNodeSnapshot(snap, installed)
	if err == nil {
		a.appliedKey = key
	}
	return revision, err
}

// Xray routes every protocol's traffic, but a change to one VPN's clients must
// not restart every unrelated VPN daemon on the host.
func snapshotCoreRevisions(snap *nodewire.Snapshot) map[string]string {
	groups := map[string]*nodewire.Snapshot{}
	for _, inbound := range snap.Inbounds {
		core := service.NodeCoreForProtocol(inbound.Protocol)
		if core == "xray" {
			continue
		}
		if groups[core] == nil {
			groups[core] = &nodewire.Snapshot{Enable: snap.Enable}
		}
		groups[core].Inbounds = append(groups[core].Inbounds, inbound)
	}
	result := map[string]string{"xray": service.NodeSnapshotRevision(snap)}
	for core, group := range groups {
		inbounds, accounts := map[int]bool{}, map[int]bool{}
		for _, in := range group.Inbounds {
			inbounds[in.Id] = true
		}
		for _, m := range snap.Memberships {
			if inbounds[m.InboundId] {
				group.Memberships = append(group.Memberships, m)
				accounts[m.AccountId] = true
			}
		}
		for _, a := range snap.Accounts {
			if accounts[a.Account.Id] {
				group.Accounts = append(group.Accounts, a)
			}
		}
		result[core] = service.NodeSnapshotRevision(group)
	}
	return result
}

func changedSnapshotCores(before, after map[string]string) []string {
	set := map[string]bool{}
	for core, revision := range before {
		if after[core] != revision {
			set[core] = true
		}
	}
	for core, revision := range after {
		if before[core] != revision {
			set[core] = true
		}
	}
	var cores []string
	for core := range set {
		if core != "xray" {
			cores = append(cores, core)
		}
	}
	sort.Strings(cores)
	if set["xray"] {
		cores = append(cores, "xray")
	}
	return cores
}

func (a *Agent) refreshSnapshot(snap *nodewire.Snapshot) error {
	next := snapshotCoreRevisions(snap)
	// The first snapshot or a changed installation may need to initialize shared
	// resources too. Later edits reconcile only the affected protocol services.
	if a.coreRevisions == nil || a.provisionRefresh {
		if err := a.runtime.RefreshNodeRuntime(); err != nil {
			return err
		}
	} else {
		for _, core := range changedSnapshotCores(a.coreRevisions, next) {
			if err := a.runtime.RefreshNodeCore(core, false); err != nil {
				return err
			}
		}
	}
	a.coreRevisions = next
	return nil
}
