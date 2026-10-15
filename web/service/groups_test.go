package service

import (
	"encoding/json"
	"fmt"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"reflect"
	"testing"
)

func groupRoot(t *testing.T) *model.User {
	t.Helper()
	var u model.User
	if err := database.GetDB().Where("is_super_admin = ?", true).First(&u).Error; err != nil {
		t.Fatal(err)
	}
	return &u
}
func mustGroups(t *testing.T) []GroupView {
	t.Helper()
	rows, err := (&GroupService{}).List(groupRoot(t), true)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
func groupIDsFor(t *testing.T, accountID int) []int {
	t.Helper()
	ids := []int{}
	if err := database.GetDB().Model(&model.AccountGroup{}).Where("account_id = ?", accountID).Order("group_id").Pluck("group_id", &ids).Error; err != nil {
		t.Fatal(err)
	}
	return ids
}
func TestGroupsMigrationPreservesEveryExistingMembership(t *testing.T) {
	accounts := newAccountsDB(t)
	a := seedInboundWithClients(t, model.VLESS, 43101, []map[string]any{{"email": "a", "id": "11111111-1111-4111-8111-111111111111", "enable": true, "subId": "same-sub", "totalGB": 12345, "expiryTime": 2000000000000}, {"email": "b", "id": "22222222-2222-4222-8222-222222222222", "enable": true}})
	b := seedInboundWithClients(t, model.Trojan, 43102, []map[string]any{{"email": "a", "password": "unchanged-password", "enable": true, "subId": "same-sub", "totalGB": 12345, "expiryTime": 2000000000000}})
	seedInboundWithClients(t, model.VMESS, 43103, []map[string]any{})
	accounts.MigrationAccounts()
	if _, err := accounts.SetMembershipEnable("a", b.Id, false); err != nil {
		t.Fatal(err)
	}
	before := snapshotUntouchedTables(t)
	beforeAccounts := accountsInDB(t)
	beforeMemberships := membershipsInDB(t)
	gs := &GroupService{}
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	assertAdditiveOnly(t, before)
	if !reflect.DeepEqual(beforeAccounts, accountsInDB(t)) || !reflect.DeepEqual(beforeMemberships, membershipsInDB(t)) {
		t.Fatal("migration rewrote an account or membership")
	}
	groups := mustGroups(t)
	if _, _, err := gs.Save(groupRoot(t), groups[0].ID, "Renamed compatibility group", groups[0].InboundIDs); err != nil {
		t.Fatal(err)
	}
	assertAdditiveOnly(t, before)
	if !reflect.DeepEqual(beforeMemberships, membershipsInDB(t)) {
		t.Fatal("renaming group rewrote existing membership")
	}
	if len(groups) != 3 {
		t.Fatal("expected one group per inbound", groups)
	}
	for _, m := range beforeMemberships {
		ids := groupIDsFor(t, m.AccountId)
		found := false
		for _, g := range groups {
			if reflect.DeepEqual(g.InboundIDs, []int{m.InboundId}) {
				for _, id := range ids {
					if id == g.ID {
						found = true
					}
				}
			}
		}
		if !found {
			t.Fatal("existing inbound was not mapped to its matching group", m)
		}
	}
	aa := noInboundAccount(t, accounts, "a")
	bb := noInboundAccount(t, accounts, "b")
	if len(groupIDsFor(t, aa.Id)) != 2 || len(groupIDsFor(t, bb.Id)) != 1 {
		t.Fatal("multi-inbound users did not receive exact groups")
	}
	later := seedInboundWithClients(t, model.Shadowsocks, 43104, []map[string]any{})
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	if len(mustGroups(t)) != 3 {
		t.Fatal("new inbound was automatically grouped on restart")
	}
	for _, g := range mustGroups(t) {
		if reflect.DeepEqual(g.InboundIDs, []int{later.Id}) {
			t.Fatal("new inbound must be manually grouped")
		}
	}
	if _, err := accounts.ApplyMembershipsFrom("b", 0, []int{a.Id, later.Id}, []int{a.Id}, true); err == nil {
		t.Fatal("direct inbound assignment bypassed group authority")
	}
}
func TestGroupsDynamicUnionAndEmptyGroupPreserveAccounts(t *testing.T) {
	accounts := newAccountsDB(t)
	a := seedInboundWithClients(t, model.VLESS, 43201, []map[string]any{{"email": "a", "id": "11111111-1111-4111-8111-111111111111", "enable": true, "subId": "same-sub"}, {"email": "b", "id": "22222222-2222-4222-8222-222222222222", "enable": true}})
	b := seedInboundWithClients(t, model.Trojan, 43202, []map[string]any{})
	accounts.MigrationAccounts()
	gs := &GroupService{}
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	root := groupRoot(t)
	g := mustGroups(t)[0]
	if err := database.GetDB().Model(&model.Account{}).Where("1=1").Update("password", "preserved-password").Error; err != nil {
		t.Fatal(err)
	}
	before := accountsInDB(t)
	_, changed, err := gs.Save(root, g.ID, "Shared", []int{a.Id, b.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 2 && len(changed) != 1 {
		t.Fatal("no projection changes", changed)
	}
	for _, email := range []string{"a", "b"} {
		ids, err := accounts.InboundIdsForEmail(email)
		if err != nil || !reflect.DeepEqual(ids, []int{a.Id, b.Id}) {
			t.Fatal("group not applied to all members", email, ids, err)
		}
	}
	overlap, _, err := gs.Save(root, 0, "Overlap", []int{b.Id})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Apply(root, "a", 0, []int{g.ID, overlap.ID}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := gs.Save(root, g.ID, "Shared", nil); err != nil {
		t.Fatal(err)
	}
	ids, _ := accounts.InboundIdsForEmail("a")
	if !reflect.DeepEqual(ids, []int{b.Id}) {
		t.Fatal("overlap membership removed", ids)
	}
	ids, _ = accounts.InboundIdsForEmail("b")
	if len(ids) != 0 {
		t.Fatal("removed inbound remained", ids)
	}
	for _, old := range before {
		stored := noInboundAccount(t, accounts, old.Email)
		if !reflect.DeepEqual(*stored, old) {
			t.Fatal("group edits changed customer credentials, quota, expiry or subscription")
		}
	}
	if err := gs.Delete(root, g.ID); err == nil {
		t.Fatal("deleted a group with users")
	}
	if _, _, err := gs.Save(root, g.ID, "Shared", []int{a.Id}); err != nil {
		t.Fatal(err)
	}
	ids, _ = accounts.InboundIdsForEmail("b")
	if !reflect.DeepEqual(ids, []int{a.Id}) {
		t.Fatal("empty group lost its existing users", ids)
	}
}
func TestGroupsMutationRollsBackAllUsersOnAdmissionFailure(t *testing.T) {
	accounts := newAccountsDB(t)
	a := seedInboundWithClients(t, model.VLESS, 43301, []map[string]any{{"email": "a", "id": "11111111-1111-4111-8111-111111111111", "enable": true}, {"email": "b", "id": "22222222-2222-4222-8222-222222222222", "enable": true}})
	accounts.MigrationAccounts()
	gs := &GroupService{}
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	b := seedInboundWithClients(t, model.Trojan, 43302, []map[string]any{})
	before := snapshotUntouchedTables(t)
	links := membershipsInDB(t)
	// Failure after at least one account has been projected exercises rollback of
	// both JSON settings and the membership rows, not merely preflight rejection.
	if err := database.GetDB().Exec(fmt.Sprintf(`CREATE TRIGGER reject_second_group_member BEFORE INSERT ON account_inbounds WHEN NEW.inbound_id=%d AND NEW.account_id=(SELECT id FROM accounts WHERE email='b') BEGIN SELECT RAISE(ABORT,'injected capacity failure'); END`, b.Id)).Error; err != nil {
		t.Fatal(err)
	}
	g := mustGroups(t)[0]
	if _, _, err := gs.Save(groupRoot(t), g.ID, "must rollback", []int{a.Id, b.Id}); err == nil {
		t.Fatal("expected failure")
	}
	assertAdditiveOnly(t, before)
	if !reflect.DeepEqual(links, membershipsInDB(t)) {
		t.Fatal("membership change was not rolled back")
	}
	if got := mustGroups(t)[0]; got.Name != g.Name || !reflect.DeepEqual(got.InboundIDs, g.InboundIDs) {
		t.Fatal("group definition was not rolled back", got)
	}
}
func TestGroupsMigrationRollsBackAndTranslatesImportedRoleAccess(t *testing.T) {
	accounts := newAccountsDB(t)
	a := seedInboundWithClients(t, model.VLESS, 43401, []map[string]any{{"email": "a", "id": "11111111-1111-4111-8111-111111111111", "enable": true}})
	accounts.MigrationAccounts()
	role := model.RepresentativeRole{Name: "Imported", Access: `{"allowed_group_ids":[17],"require_template":false,"allowed_template_ids":[4]}`}
	db := database.GetDB()
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.PasarGuardResource{{Kind: "inbounds", SourceID: 8, Data: fmt.Sprintf(`{"id":"8","tag":%q}`, a.Tag)}, {Kind: "inbounds_groups_association", SourceID: 9, Data: `{"group_id":"17","inbound_id":"8"}`}}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_group_migration BEFORE INSERT ON account_groups BEGIN SELECT RAISE(ABORT,'injected failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	gs := &GroupService{}
	if err := gs.Migrate(); err == nil {
		t.Fatal("expected failure")
	}
	if GroupsMigrated(db) || len(mustGroups(t)) != 0 {
		t.Fatal("partial migration committed")
	}
	db.Exec(`DROP TRIGGER reject_group_migration`)
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	var migrated model.RepresentativeRole
	if err := db.First(&migrated, role.ID).Error; err != nil {
		t.Fatal(err)
	}
	var access struct {
		Groups    []int `json:"allowed_group_ids"`
		Templates []int `json:"allowed_template_ids"`
	}
	if err := json.Unmarshal([]byte(migrated.Access), &access); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(access.Groups, []int{mustGroups(t)[0].ID}) || !reflect.DeepEqual(access.Templates, []int{4}) {
		t.Fatal("incorrect native role mapping", migrated.Access)
	}
	if err := enforceRoleInboundAccess(&migrated, []int{a.Id}); err != nil {
		t.Fatal(err)
	}
}
func TestGroupsFreshInstallationAndActualSingleAccountAdmission(t *testing.T) {
	accounts := newAccountsDB(t)
	accounts.MigrationAccounts()
	gs := &GroupService{}
	if err := gs.Migrate(); err != nil {
		t.Fatal("fresh panel cannot start", err)
	}
	if len(mustGroups(t)) != 0 {
		t.Fatal("fresh panel has compatibility groups")
	}
	a := seedInboundWithClients(t, model.VLESS, 43501, []map[string]any{{"email": "a", "id": "11111111-1111-4111-8111-111111111111", "enable": true}, {"email": "b", "id": "22222222-2222-4222-8222-222222222222", "enable": true}})
	if err := accounts.SyncInboundAccounts(database.GetDB(), a.Id); err != nil {
		t.Fatal(err)
	}
	g, _, err := gs.Save(groupRoot(t), 0, "Customers", []int{a.Id})
	if err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{"a", "b"} {
		if _, err := gs.Apply(groupRoot(t), email, 0, []int{g.ID}); err != nil {
			t.Fatal(err)
		}
	}
	ike := seedInboundWithClients(t, model.IKEV2, 43502, []map[string]any{})
	if err := database.GetDB().Model(ike).Update("settings", `{"authMode":"psk","psk":"secret","clients":[]}`).Error; err != nil {
		t.Fatal(err)
	}
	before := snapshotUntouchedTables(t)
	members := membershipsInDB(t)
	if err := gs.PreflightCreate([]int{ike.Id}, []string{"new-one", "new-two"}); err == nil {
		t.Fatal("batch preflight accepted two IKEv2 PSK users")
	}
	assertAdditiveOnly(t, before)
	if _, _, err := gs.Save(groupRoot(t), g.ID, "Must fail", []int{a.Id, ike.Id}); err == nil {
		t.Fatal("single-account IKEv2 accepted two group members")
	}
	assertAdditiveOnly(t, before)
	if !reflect.DeepEqual(members, membershipsInDB(t)) {
		t.Fatal("actual admission failure left partial memberships")
	}
}
func TestGroupsRepresentativeSelectionRequiresRoleAndInboundGrants(t *testing.T) {
	accounts := newAccountsDB(t)
	a := seedInboundWithClients(t, model.VLESS, 43601, []map[string]any{})
	b := seedInboundWithClients(t, model.Trojan, 43602, []map[string]any{})
	accounts.MigrationAccounts()
	gs := &GroupService{}
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	groups := mustGroups(t)
	role := model.RepresentativeRole{Name: "Scoped", Access: fmt.Sprintf(`{"allowed_group_ids":[%d]}`, groups[0].ID)}
	db := database.GetDB()
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	rep := model.User{Username: "scoped-rep", Enable: true, IsReseller: true, RepresentativeRoleID: role.ID}
	if err := db.Create(&rep).Error; err != nil {
		t.Fatal(err)
	}
	grants := []model.InboundAccess{{UserId: rep.Id, InboundId: a.Id}, {UserId: rep.Id, InboundId: b.Id}}
	if err := db.Create(&grants).Error; err != nil {
		t.Fatal(err)
	}
	choices, err := gs.List(&rep, false)
	if err != nil || len(choices) != 1 || choices[0].ID != groups[0].ID {
		t.Fatal("role scope ignored", choices, err)
	}
	if _, err := gs.Resolve(&rep, []int{groups[1].ID}); err == nil {
		t.Fatal("inbound grant bypassed group restriction")
	}
	if err := db.Model(&role).Update("access", `{"allowed_group_ids":null}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("user_id = ? AND inbound_id = ?", rep.Id, b.Id).Delete(&model.InboundAccess{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Resolve(&rep, []int{groups[1].ID}); err == nil {
		t.Fatal("unrestricted group policy bypassed inbound grant")
	}
	if _, err := gs.Resolve(&rep, []int{groups[0].ID}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&role).Update("access", `{"allowed_group_ids":[]}`).Error; err != nil {
		t.Fatal(err)
	}
	if choices, err := gs.List(&rep, false); err != nil || len(choices) != 0 {
		t.Fatal("empty role allowlist should deny every group", choices, err)
	}
	owner := *groupRoot(t)
	owner.RepresentativeRoleID = role.ID
	if choices, err := gs.List(&owner, false); err != nil || len(choices) != 2 {
		t.Fatal("imported role ID restricted the panel owner", choices, err)
	}

}
