package service

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/nodewire"
	"gorm.io/gorm"
)

func TestNodeBatchProjectionPreservesCredentialsAdmissionAndRemovedMemberships(t *testing.T) {
	newAccountsDB(t)
	db := database.GetDB()
	in := model.Inbound{Id: 1, Tag: "node-projection", Protocol: model.VLESS, Settings: `{"clients":[{"email":"user-0","id":"old-id","custom":"keep"},{"email":"user-2","id":"retired-id"}],"customSetting":true}`}
	if err := db.Create(&in).Error; err != nil {
		t.Fatal(err)
	}
	var requested []nodewire.Account
	for i := 0; i < 30; i++ {
		a := model.Account{Id: i + 1, Email: fmt.Sprintf("user-%d", i), Enable: i != 1, TotalGB: 1000, ProtocolCredentials: fmt.Sprintf(`{"vless":{"id":"00000000-0000-0000-0000-%012d"}}`, i+1)}
		if err := db.Create(&a).Error; err != nil {
			t.Fatal(err)
		}
		requested = append(requested, nodewire.Account{Account: a})
		if i == 2 {
			continue
		} // A formerly assigned account must disappear.
		enabled := i != 3
		if err := db.Create(&model.AccountInbound{AccountId: a.Id, InboundId: in.Id, Enable: &enabled}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range requested {
		if _, err := (&AccountService{}).ProjectAccount(db, a.Account.Id); err != nil {
			t.Fatal(err)
		}
	}
	var expected model.Inbound
	if err := db.First(&expected, in.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&in).Update("settings", in.Settings).Error; err != nil {
		t.Fatal(err)
	}
	writes := 0
	if err := db.Callback().Update().After("gorm:update").Register("test_node_projection_writes", func(tx *gorm.DB) {
		if tx.Statement.Table == "inbounds" {
			writes++
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Callback().Update().Remove("test_node_projection_writes") })
	if err := projectNodeAccounts(db, requested); err != nil {
		t.Fatal(err)
	}
	var actual model.Inbound
	if err := db.First(&actual, in.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expected.Settings, actual.Settings) {
		t.Fatal("batch projection changed client credentials, admission, custom fields or removal semantics")
	}
	if writes != 1 {
		t.Fatalf("one inbound with 30 clients required %d writes; want one", writes)
	}
}

func TestNodeIPsecSelectionAndInventorySkipPreflight(t *testing.T) {
	svc, n, _ := nodeFixture(t)
	if err := svc.InstallCores(n.ID, []string{"ipsec"}); err != nil {
		t.Fatal(err)
	}
	ss := &SettingService{}
	if err := ss.SetProvisionedProtocols([]string{"l2tp"}); err != nil {
		t.Fatal(err)
	}
	probed := withStubConflictProbe(t)
	installed := (&CoreService{}).InstalledNodeCores()
	if !installed["l2tp"] || !installed["ipsec"] || len(*probed) != 0 {
		t.Fatal("cheap inventory lost shared IPsec or invoked preflight", installed, *probed)
	}
}
