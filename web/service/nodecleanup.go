package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

// CleanupNodeHost runs after systemd has stopped the dedicated agent and its
// process cgroup. It reuses ownership-aware core cleanup rather than the panel
// uninstaller, which also removes the central panel's service and management menu.
func CleanupNodeHost() (*CoreUninstallReport, error) {
	if os.Getenv("VPNUI_NODE_AGENT") != "1" {
		return nil, errors.New("dedicated node mode is required")
	}
	var cs CoreService
	var ss SettingService
	selected := nodeCleanupSelection(ss.GetProvisionedProtocols(), OwnershipReport())
	var count int64
	if err := database.GetDB().Model(&model.Inbound{}).Count(&count).Error; err != nil {
		return nil, err
	}
	report := &CoreUninstallReport{Cores: selected}
	if len(selected) > 0 {
		if err := ss.SetProvisionedProtocols(selected); err != nil {
			return nil, err
		}
		report.Kept = cs.runCoreUninstall(selected, func(step ProvisionStep) { report.Steps = append(report.Steps, step) })
	}
	var err error
	// Undo the node's data plane only when its database actually served a VPN.
	if count > 0 && nodeCleanupHasTunnelCore(selected) {
		if commandExists("nft") {
			if exec.Command("nft", "list", "table", "ip", "vpn").Run() == nil {
				err = exec.Command("nft", "delete", "table", "ip", "vpn").Run()
				report.Steps = append(report.Steps, ProvisionStep{Name: "remove node nftables table", OK: err == nil, Msg: msgOrOK(err)})
			}
		}
		(&NftService{}).CleanupLegacyIptables()
		collapseVpnPolicyRules(true)
		// Delete only our loopback-local route, never flush an operator's table.
		if commandExists("ip") {
			out, _ := exec.Command("ip", "route", "show", "table", "100", "type", "local").Output()
			if strings.Contains(string(out), "local default dev lo") || strings.Contains(string(out), "local 0.0.0.0/0 dev lo") {
				err = exec.Command("ip", "route", "del", "local", "0.0.0.0/0", "dev", "lo", "table", "100").Run()
				report.Steps = append(report.Steps, ProvisionStep{Name: "remove node policy route", OK: err == nil, Msg: msgOrOK(err)})
			}
		}
	}
	files := &UninstallReport{}
	releaseOwned(files, "/etc/vpn-ui/vpn.nft", selected)
	report.Kept = append(report.Kept, files.Kept...)
	for _, failure := range files.Errors {
		report.Steps = append(report.Steps, ProvisionStep{Name: "release node host file", OK: false, Msg: failure})
	}
	var failures []string
	for _, step := range report.Steps {
		if !step.OK {
			failures = append(failures, step.Name+": "+step.Msg)
		}
	}
	if len(failures) > 0 {
		return report, fmt.Errorf("node cleanup incomplete: %s", strings.Join(failures, "; "))
	}
	return report, nil
}

func nodeCleanupSelection(recorded []string, owned []OwnedArtifactReport) []string {
	names := append([]string(nil), recorded...)
	for _, artifact := range owned {
		names = append(names, artifact.Cores...)
	}
	return validCoreNames(names)
}
func nodeCleanupHasTunnelCore(names []string) bool {
	for _, name := range names {
		switch name {
		case "l2tp", "pptp", "openvpn", "openconnect", "sstp", "ikev2", "wgc", "awg", "gre":
			return true
		}
	}
	return false
}
