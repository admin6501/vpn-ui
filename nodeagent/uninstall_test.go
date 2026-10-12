package nodeagent

import (
	"path/filepath"
	"testing"
)

func TestNodeUninstallRejectsOtherInstallations(t *testing.T) {
	t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	for _, path := range []string{"/opt/vpn-ui/agent.json", "/opt/vpn-ui-node/agent.json", filepath.Join(t.TempDir(), "agent.json")} {
		if err := Uninstall(path); err == nil {
			t.Fatal("cleanup accepted a non-dedicated installation")
		}
	}
}
