package nodeagent

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/op/go-logging"
)

// Uninstall cleans host artifacts without needing a live panel or pairing token.
// The installer owns confirmation, service shutdown and deletion of agent files.
func Uninstall(path string) error {
	if filepath.Clean(path) != "/opt/vpn-ui-node/agent.json" || filepath.Clean(config.GetDBFolderPath()) != "/opt/vpn-ui-node" || filepath.Clean(config.GetBinFolderPath()) != "/opt/vpn-ui-node/bin" {
		return errors.New("uninstall must target the dedicated /opt/vpn-ui-node installation")
	}
	for _, p := range []string{"/opt/vpn-ui-node", "/opt/vpn-ui-node/bin", "/opt/vpn-ui-node/agent.json", "/opt/vpn-ui-node/vpn-ui.db"} {
		if info, e := os.Lstat(p); e == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink: %s", p)
		}
	}
	if exec.Command("systemctl", "is-active", "--quiet", "vpn-ui-node").Run() == nil {
		return errors.New("stop vpn-ui-node before uninstalling")
	}
	if exec.Command("systemctl", "cat", "vpn-ui").Run() == nil {
		return errors.New("central panel service exists; refusing shared-host cleanup")
	}
	if _, e := os.Stat("/opt/vpn-ui/vpn-ui.db"); e == nil {
		return errors.New("central panel database exists; refusing shared-host cleanup")
	}
	if _, e := os.Stat(config.GetDBPath()); os.IsNotExist(e) {
		return nil
	} else if e != nil {
		return e
	}
	os.Setenv("VPNUI_NODE_AGENT", "1")
	logger.InitLogger(logging.INFO)
	if e := database.InitDB(config.GetDBPath()); e != nil {
		return e
	}
	report, e := service.CleanupNodeHost()
	if report != nil {
		for _, step := range report.Steps {
			fmt.Printf("%s: %s\n", step.Name, step.Msg)
		}
		for _, kept := range report.Kept {
			fmt.Printf("Kept: %s\n", kept)
		}
	}
	return e
}
