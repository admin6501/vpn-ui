package service

import (
	"encoding/json"
	"github.com/mhsanaei/3x-ui/v2/database"
	"os"
	"path/filepath"
	"testing"
)

func TestSSLSyncCandidateIsNotAnUnrelatedLegacyDomain(t *testing.T) {
	t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
	home := t.TempDir()
	for _, domain := range []string{"panel.example.test", "other.example.test"} {
		dir := filepath.Join(home, domain)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, domain+".conf"), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	candidate := SSLAdoptable{HasAcmeState: true, Info: &SSLCertInfo{Identifiers: []string{"panel.example.test"}, KeyMatchesLeaf: true}}
	remaining := sslLegacyDomainsNotManagedAt(home, []SSLAdoptable{candidate})
	if len(remaining) != 1 || remaining[0] != "other.example.test" {
		t.Fatalf("candidate incorrectly blocks its own adoption: %v", remaining)
	}
	candidate.Info.KeyMatchesLeaf = false
	if got := sslLegacyDomainsNotManagedAt(home, []SSLAdoptable{candidate}); len(got) != 2 {
		t.Fatalf("invalid pair must not authorize removing renewal: %v", got)
	}
}

func TestSSLStatusRetainsExternalServingCertificateAfterMigration(t *testing.T) {
	newAccountsDB(t)
	root := t.TempDir()
	cert := makeTestCert(t, testCertOpts{sans: []string{"panel.example.test"}, chain: true, issuerCN: "Test CA"})
	certPath, keyPath := filepath.Join(root, "fullchain.pem"), filepath.Join(root, "privkey.pem")
	if err := os.WriteFile(certPath, cert.certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, cert.keyPEM, 0600); err != nil {
		t.Fatal(err)
	}
	settings := &SettingService{}
	settings.SetCertFile(certPath)
	settings.SetKeyFile(keyPath)
	settings.SetSubCertFile(certPath)
	settings.SetSubKeyFile(keyPath)
	status, err := (&SSLService{}).Status("")
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Adoptable) != 1 || !status.Adoptable[0].UsedByPanel || !status.Adoptable[0].UsedBySub {
		t.Fatalf("serving certificate disappeared: %+v", status.Adoptable)
	}
	unchanged, _ := settings.GetCertFile()
	if unchanged != certPath {
		t.Fatal("status changed the live certificate path")
	}
}

func TestOverviewRequiredCoresIgnoreRemoteAndDisabledInbounds(t *testing.T) {
	newAccountsDB(t)
	local := seedInboundWithClients(t, "openvpn", 47401, nil)
	remote := seedInboundWithClients(t, "ikev2", 47402, nil)
	disabled := seedInboundWithClients(t, "pptp", 47403, nil)
	db := database.GetDB()
	if err := db.Model(remote).Update("node_id", 7).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(disabled).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	counts := localEnabledCoreInbounds()
	if counts["openvpn"] != 1 || counts["xray"] != 1 || counts["ikev2"] != 0 || counts["pptp"] != 0 {
		t.Fatalf("host requirements leaked remote/disabled services: %v, local=%d", counts, local.Id)
	}
}

func TestOverviewCentralXrayTagsExcludeRemoteNodes(t *testing.T) {
	newAccountsDB(t)
	local := seedInboundWithClients(t, "vless", 47501, nil)
	remote := seedInboundWithClients(t, "trojan", 47502, nil)
	if err := database.GetDB().Model(remote).Update("node_id", 7).Error; err != nil {
		t.Fatal(err)
	}
	raw, err := (&InboundService{}).GetInboundTags()
	if err != nil {
		t.Fatal(err)
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0] != local.Tag {
		t.Fatalf("central routing leaked remote node tags: %v", tags)
	}
}
