package nodeagent

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/nodewire"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func TestNodeManagementRequiresVerifiedTLS(t *testing.T) {
	for _, raw := range []string{"http://example.com/", "https://user:pass@example.com", "https://example.com/?token=secret", "file:///tmp/a", "https://example.com/#a"} {
		if _, err := ValidatePanelURL(raw); err == nil {
			t.Fatalf("accepted unsafe URL: %s", raw)
		}
	}
	if got, err := ValidatePanelURL("https://example.com/panel"); err != nil || got != "https://example.com/panel/" {
		t.Fatal(got, err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(nodewire.EnrollmentReply{NodeID: 1})
	}))
	defer server.Close()
	c := Config{PanelURL: server.URL + "/"}
	client, err := Client(c)
	if err != nil {
		t.Fatal(err)
	}
	var out nodewire.EnrollmentReply
	if exchange(context.Background(), client, c, "enroll", nil, &out) == nil {
		t.Fatal("untrusted certificate accepted")
	}
	ca := filepath.Join(t.TempDir(), "ca.pem")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	c.CAFile = ca
	client, err = Client(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = exchange(context.Background(), client, c, "enroll", nil, &out); err != nil || out.NodeID != 1 {
		t.Fatal(err, out)
	}
}
func TestNodeEnrollmentRetriesSameCredentialAfterLostResponse(t *testing.T) {
	var token string
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in nodewire.Enrollment
		json.NewDecoder(r.Body).Decode(&in)
		requests++
		if token == "" {
			token = in.AgentToken
		} else if token != in.AgentToken {
			t.Error("retry changed credential")
		}
		if requests == 1 {
			w.WriteHeader(200)
			w.Write([]byte("{"))
			return
		}
		json.NewEncoder(w).Encode(nodewire.EnrollmentReply{NodeID: 7})
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "agent.json")
	pair := strings.Repeat("a", 64)
	if Enroll(context.Background(), path, server.URL, pair, "") == nil {
		t.Fatal("partial response accepted")
	}
	if err := Enroll(context.Background(), path, server.URL, pair, ""); err != nil {
		t.Fatal(err)
	}
	a, err := New(path)
	if err != nil || a.config.NodeID != 7 || a.config.Token != token {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("credential file is public")
	}
	if err = Enroll(context.Background(), path, server.URL, pair, ""); err != nil || requests != 2 {
		t.Fatal("paired installer is not idempotent", err)
	}
}
func TestNodeDurablePendingReportSurvivesRestartWithoutCoreInstall(t *testing.T) {
	t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	db.Create(&model.Account{Id: 1, Email: "meter@example.com", Enable: true})
	db.Create(&xray.ClientTraffic{Email: "meter@example.com", Up: 100, Down: 200, Enable: true})
	a := &Agent{}
	first, err := a.pendingReport()
	if err != nil {
		t.Fatal(err)
	}
	db.Model(&xray.ClientTraffic{}).Where("email = ?", "meter@example.com").Update("up", 150)
	restarted := &Agent{}
	retry, err := restarted.pendingReport()
	if err != nil {
		t.Fatal(err)
	}
	raw1, _ := json.Marshal(first)
	raw2, _ := json.Marshal(retry)
	if string(raw1) != string(raw2) {
		t.Fatal("pending report changed after restart")
	}
	if err = ackReport(first.Sequence); err != nil {
		t.Fatal(err)
	}
	next, err := restarted.pendingReport()
	if err != nil || next.Sequence != first.Sequence+1 {
		t.Fatal(err, next.Sequence)
	}
	if len(installedCores()) != 0 {
		t.Fatal("fresh agent installed a core")
	}
	if err = a.requestCores(&nodewire.Snapshot{}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(config.GetBinFolderPath(), "xray-linux-amd64")); err == nil {
		t.Fatal("implicit Xray extraction")
	}
	if service.GetNodeSetting("nodeCoreGeneration") != "" {
		t.Fatal("implicit core request")
	}
}
