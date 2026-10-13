package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/nodewire"
)

func tagRepairSnapshot(inbounds ...model.Inbound) *nodewire.Snapshot {
	return &nodewire.Snapshot{Version: nodewire.Version, NodeID: 1, LeaseSeconds: nodewire.LeaseSeconds, Enable: true, Template: defaultNodeTemplate, Inbounds: inbounds}
}
func tagRepairInbound(id int, tag string) model.Inbound {
	return model.Inbound{Id: id, Tag: tag, Protocol: model.AWG, Port: 51820 + id, Enable: true, Settings: `{"clients":[]}`, StreamSettings: `{}`}
}

func TestNodeTagReuseRetainsRetiredMetersAndCentralTag(t *testing.T) {
	newAccountsDB(t)
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	db := database.GetDB()
	retired := tagRepairInbound(10, "node-1-amnezia")
	retired.Up, retired.Down, retired.AllTime = 17, 23, 40
	if err := db.Create(&retired).Error; err != nil {
		t.Fatal(err)
	}
	replacement := tagRepairInbound(20, retired.Tag)
	// Reproduce the failing plain upsert before invoking the repair path.
	if err := db.Save(&replacement).Error; err == nil {
		t.Fatal("fixture did not reproduce the unique tag conflict")
	}
	snap := tagRepairSnapshot(replacement)
	if _, err := ApplyNodeSnapshot(snap, map[string]bool{"awg": true, "xray": true}); err != nil {
		t.Fatal(err)
	}
	var old, current model.Inbound
	db.First(&old, retired.Id)
	db.First(&current, replacement.Id)
	if old.Enable || old.Tag == retired.Tag || old.Up != 17 || old.Down != 23 || old.AllTime != 40 {
		t.Fatal("retired identity/meter was lost", old.Id, old.Up, old.Down, old.AllTime)
	}
	if current.Tag != replacement.Tag || !current.Enable {
		t.Fatal("central routing tag changed or replacement stayed disabled")
	}
	archivedTag := old.Tag
	db.Model(&current).Updates(map[string]any{"up": 71, "all_time": 71})
	if _, err := ApplyNodeSnapshot(snap, map[string]bool{"awg": true, "xray": true}); err != nil {
		t.Fatal(err)
	}
	db.First(&old, retired.Id)
	db.First(&current, replacement.Id)
	if old.Tag != archivedTag || current.Up != 71 || current.AllTime != 71 {
		t.Fatal("repeated sync renamed tombstone or reset current meter")
	}
}

func TestNodeTagSwapIsTransactional(t *testing.T) {
	newAccountsDB(t)
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	db := database.GetDB()
	first, second := tagRepairInbound(1, "first"), tagRepairInbound(2, "second")
	first.Up, second.Up = 11, 22
	db.Create(&first)
	db.Create(&second)
	one, two := first, second
	one.Tag, two.Tag = second.Tag, first.Tag
	if _, err := ApplyNodeSnapshot(tagRepairSnapshot(one, two), map[string]bool{"awg": true}); err != nil {
		t.Fatal(err)
	}
	var a, b model.Inbound
	db.First(&a, first.Id)
	db.First(&b, second.Id)
	if a.Tag != "second" || b.Tag != "first" || a.Up != 11 || b.Up != 22 {
		t.Fatal("swapped tags lost identities/meters")
	}
}

func TestNodeDuplicateIncomingTagsDoNotChangeAdmission(t *testing.T) {
	newAccountsDB(t)
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	original := tagRepairInbound(1, "original")
	database.GetDB().Create(&original)
	for _, entries := range [][]model.Inbound{{tagRepairInbound(2, "same"), tagRepairInbound(3, "same")}, {tagRepairInbound(2, "a"), tagRepairInbound(2, "b")}, {tagRepairInbound(0, "new")}} {
		if _, err := ApplyNodeSnapshot(tagRepairSnapshot(entries...), map[string]bool{"awg": true}); err == nil {
			t.Fatal("invalid snapshot identities accepted")
		}
		var kept model.Inbound
		database.GetDB().First(&kept, original.Id)
		if !kept.Enable || kept.Tag != original.Tag {
			t.Fatal("invalid snapshot changed live admission")
		}
	}
}

func TestNodeTagReservationRollsBackWithInvalidProjection(t *testing.T) {
	newAccountsDB(t)
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	db := database.GetDB()
	original := tagRepairInbound(1, "shared")
	original.Up = 99
	db.Create(&original)
	broken := tagRepairInbound(2, "shared")
	broken.Settings = "not-json"
	snapshot := tagRepairSnapshot(broken)
	snapshot.Accounts = []nodewire.Account{{Account: model.Account{Id: 1, Email: "test@example.com"}, Allowed: true, Remaining: 100}}
	snapshot.Memberships = []model.AccountInbound{{AccountId: 1, InboundId: broken.Id}}
	if _, err := ApplyNodeSnapshot(snapshot, map[string]bool{"awg": true}); err == nil {
		t.Fatal("invalid projection accepted")
	}
	var kept model.Inbound
	db.First(&kept, original.Id)
	if kept.Tag != original.Tag || kept.Up != 99 || !kept.Enable {
		t.Fatal("failed transaction leaked tag/archive changes")
	}
	var count int64
	db.Model(&model.Inbound{}).Where("id = ?", broken.Id).Count(&count)
	if count != 0 {
		t.Fatal("failed snapshot left replacement behind")
	}
}
