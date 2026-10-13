// Package nodeagent runs the outbound-only agent and the panel's shared data plane.
package nodeagent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v2/backend"
	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/corebundle"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/nodewire"
	"github.com/mhsanaei/3x-ui/v2/web"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/op/go-logging"
	"gorm.io/gorm"
)

type Config struct {
	PanelURL string `json:"panelUrl"`
	Token    string `json:"token"`
	NodeID   int    `json:"nodeId"`
	CAFile   string `json:"caFile,omitempty"`
}

func ValidatePanelURL(raw string) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("Invalid panel URL")
	}
	ip := net.ParseIP(u.Hostname())
	loopback := ip != nil && ip.IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return "", errors.New("Use HTTPS for node management.")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/"
	return u.String(), nil
}
func Client(c Config) (*http.Client, error) {
	roots, e := x509.SystemCertPool()
	if e != nil {
		roots = x509.NewCertPool()
	}
	if c.CAFile != "" {
		pem, e := os.ReadFile(c.CAFile)
		if e != nil {
			return nil, e
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("Invalid CA file")
		}
	}
	return &http.Client{Timeout: 25 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}}, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("Panel URL redirected; use its exact HTTPS address")
	}}, nil
}
func saveConfig(path string, c Config) error {
	data, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	if e = os.WriteFile(path+".tmp", data, 0600); e != nil {
		return e
	}
	return os.Rename(path+".tmp", path)
}
func exchange(ctx context.Context, client *http.Client, c Config, path string, input, output any) error {
	body, e := json.Marshal(input)
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, c.PanelURL+"node-agent/"+path, bytes.NewReader(body))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	if c.NodeID > 0 {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	response, e := client.Do(req)
	if e != nil {
		return e
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		if response.StatusCode == http.StatusConflict {
			switch code := response.Header.Get("X-VPN-UI-Node-Error"); code {
			case "sequence_gap", "counter_out_of_scope", "counter_regressed", "invalid_report":
				return fmt.Errorf("Panel returned HTTP 409 (%s)", code)
			}
		}
		return fmt.Errorf("Panel returned HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(output)
}
func Enroll(ctx context.Context, path, panel, pair, ca string) error {
	panel, e := ValidatePanelURL(panel)
	if e != nil {
		return e
	}
	if len(pair) != 64 {
		return errors.New("Invalid pairing token")
	}
	// Persist the generated token BEFORE enrollment: a lost HTTP response retries
	// the same credential instead of consuming the single-use code twice.
	c := Config{PanelURL: panel, CAFile: ca}
	if data, e := os.ReadFile(path); e == nil {
		if json.Unmarshal(data, &c) != nil {
			return errors.New("Invalid existing agent config")
		}
		if c.NodeID > 0 {
			if c.PanelURL != panel {
				return errors.New("Agent is paired to another panel")
			}
			fmt.Println("Agent already paired.")
			return nil
		}
		if c.PanelURL != panel {
			return errors.New("Pending enrollment belongs to another panel")
		}
	}
	if c.Token == "" {
		secret := make([]byte, 32)
		if _, e = rand.Read(secret); e != nil {
			return e
		}
		c.Token = hex.EncodeToString(secret)
	}
	if e = saveConfig(path, c); e != nil {
		return e
	}
	client, e := Client(c)
	if e != nil {
		return e
	}
	var reply nodewire.EnrollmentReply
	if e = exchange(ctx, client, c, "enroll", nodewire.Enrollment{PairToken: pair, AgentToken: c.Token, Version: config.GetVersion(), Architecture: runtime.GOARCH}, &reply); e != nil {
		return e
	}
	if reply.NodeID <= 0 {
		return errors.New("Invalid node id")
	}
	c.NodeID = reply.NodeID
	if e = saveConfig(path, c); e != nil {
		return e
	}
	fmt.Printf("Node connected: #%d\n", c.NodeID)
	return nil
}

type Agent struct {
	config           Config
	client           *http.Client
	runtime          *web.Server
	lastSuccess      time.Time
	revision         string
	inventory        string
	lastError        string
	coreRestart      string
	coreRestartForce bool
	provisionRefresh bool
}

func New(path string) (*Agent, error) {
	data, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var c Config
	if e = json.Unmarshal(data, &c); e != nil {
		return nil, e
	}
	panel, e := ValidatePanelURL(c.PanelURL)
	if e != nil {
		return nil, e
	}
	c.PanelURL = panel
	if c.NodeID <= 0 || len(c.Token) != 64 {
		return nil, errors.New("Enroll the node first")
	}
	client, e := Client(c)
	if e != nil {
		return nil, e
	}
	return &Agent{config: c, client: client}, nil
}
func prepareDatabase() error {
	os.Setenv("VPNUI_NODE_AGENT", "1")
	logger.InitLogger(logging.INFO)
	if e := database.InitDB(config.GetDBPath()); e != nil {
		return e
	}
	if e := os.Chmod(config.GetDBPath(), 0600); e != nil {
		return e
	}
	_ = service.SetNodeSetting(database.GetDB(), "nodeRuntimeRunning", "false")
	if backend.Available() {
		if _, e := service.RefreshInstalledDaemons(); e != nil {
			return e
		}
	}
	// An agent may resume only after a fresh authenticated lease. Never start VPNs
	// with cached permissions merely because systemd restarted the process.
	return service.DisableNodeAdmission()
}
func installedCores() map[string]bool {
	set := map[string]bool{}
	if _, e := os.Stat(filepath.Join(config.GetBinFolderPath(), corebundle.XrayBinaryName())); e == nil {
		set["xray"] = true
	}
	for _, c := range (&service.CoreService{}).CoreCatalog() {
		if c.Installed && !c.Builtin {
			set[c.Name] = true
		}
	}
	if service.GetNodeSetting("nodeSSHInstalled") == "true" {
		set["ssh"] = true
	}
	if len(set) > 0 && service.GetNodeSetting("nodeRadiusRemoved") != "true" {
		set["radius"] = true
	}
	return set
}

// A DKMS/compiler log can exceed the central report limit. Retain error tails
// without letting a completed install permanently wedge its durable report.
func boundedProvisionLogs(steps []service.ProvisionStep) []service.ProvisionStep {
	result := append([]service.ProvisionStep(nil), steps...)
	budget := 128 << 10
	for i := len(result) - 1; i >= 0; i-- {
		keep := min(budget, 8<<10)
		if len(result[i].Log) > keep {
			if keep == 0 {
				result[i].Log = ""
			} else {
				result[i].Log = "[earlier output omitted]\n" + result[i].Log[len(result[i].Log)-keep:]
			}
		}
		budget -= min(keep, len(result[i].Log))
	}
	return result
}

func coreReport() json.RawMessage {
	cs := &service.CoreService{}
	installed := []string{}
	for name := range installedCores() {
		installed = append(installed, name)
	}
	sort.Strings(installed)
	statuses := cs.GetCoresStatus()
	actual := installedCores()
	for i := range statuses {
		if !actual[statuses[i].Name] {
			statuses[i].State = service.CoreNotInstalled
		} else if service.NodeCorePaused(statuses[i].Name) {
			statuses[i].State = service.CoreStopped
		}
	}
	provision := cs.ProvisionState()
	provision.Steps = boundedProvisionLogs(provision.Steps)
	report := map[string]any{"installed": installed, "catalog": cs.CoreCatalog(), "cores": statuses, "system": cs.GetSystemStatus(), "provision": provision, "generation": service.GetNodeSetting("nodeCoreGeneration")}
	report["controlGeneration"] = service.GetNodeSetting("nodeControlGeneration")
	report["configGeneration"] = service.GetNodeSetting("nodeConfigGeneration")
	report["configError"] = service.GetNodeSetting("nodeConfigError")
	paused := []string{}
	for _, c := range cs.CoreCatalog() {
		if service.NodeCorePaused(c.Name) {
			paused = append(paused, c.Name)
		}
	}
	report["pausedCores"] = paused
	report["coreActionSupported"] = true
	report["coreActionGeneration"] = service.GetNodeSetting("nodeCoreActionGeneration")
	report["coreActionError"] = service.GetNodeSetting("nodeCoreActionError")
	report["uninstall"] = cs.CoreUninstallStatus()
	report["controlSupported"] = true
	report["runtimeRunning"] = service.GetNodeSetting("nodeRuntimeRunning") == "true"
	var targets any
	_ = json.Unmarshal([]byte(service.GetNodeSetting("nodeConfigTargets")), &targets)
	report["configTargets"] = targets
	data, _ := json.Marshal(report)
	return data
}
func (a *Agent) pendingReport() (nodewire.Report, error) {
	var result nodewire.Report
	e := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var row model.Setting
		if tx.Where("key = ?", "nodePendingReport").First(&row).Error == nil && row.Value != "" {
			return json.Unmarshal([]byte(row.Value), &result)
		}
		var seq model.Setting
		tx.Where("key = ?", "nodeReportSequence").First(&seq)
		number, _ := strconv.ParseInt(seq.Value, 10, 64)
		counters, e := service.ReadNodeCounters(tx)
		if e != nil {
			return e
		}
		result = nodewire.Report{Version: nodewire.Version, Sequence: number + 1, Counters: counters, Online: service.ReadNodeOnline(), CoreReport: coreReport(), LastError: a.lastError}
		raw, e := json.Marshal(result)
		if e != nil {
			return e
		}
		return service.SetNodeSetting(tx, "nodePendingReport", string(raw))
	})
	return result, e
}
func ackReport(sequence int64) error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if e := service.SetNodeSetting(tx, "nodeReportSequence", strconv.FormatInt(sequence, 10)); e != nil {
			return e
		}
		return service.SetNodeSetting(tx, "nodePendingReport", "")
	})
}
func (a *Agent) requestCores(snap *nodewire.Snapshot) error {
	generation := strconv.FormatInt(snap.CoreGeneration, 10)
	if snap.CoreGeneration == 0 || service.GetNodeSetting("nodeCoreGeneration") == generation {
		return nil
	}
	if len(snap.RequestedCores) == 0 {
		return errors.New("Empty core request rejected")
	}
	cs := &service.CoreService{}
	if cs.ProvisionState().Running || cs.CoreUninstallStatus().Running {
		return nil
	}
	// Keep the already-running data plane alive while installing an additional core.
	// Bundles are replaced atomically; admission snapshots still apply during setup.
	// Xray is a dependency of VPN accounting/routing, not a selection that installs
	// every other core. Nothing is extracted before the first central request.
	if _, e := corebundle.ExtractXray(config.GetBinFolderPath()); e != nil {
		return e
	}
	if _, e := corebundle.ExtractGeofiles(config.GetBinFolderPath()); e != nil {
		return e
	}
	installable := []string{}
	for _, c := range snap.RequestedCores {
		if e := service.SetNodeCorePaused(c, false); e != nil {
			return e
		}
		switch c {
		case "xray":
			if a.runtime != nil {
				a.coreRestart = "xray"
				a.coreRestartForce = true
			}
		case "radius":
			if e := service.SetNodeSetting(database.GetDB(), "nodeRadiusRemoved", "false"); e != nil {
				return e
			}
		case "ssh":
			if e := service.SetNodeSetting(database.GetDB(), "nodeSSHInstalled", "true"); e != nil {
				return e
			}
		default:
			installable = append(installable, c)
		}
	}
	if len(installable) > 0 && !cs.StartProvision(installable) {
		return errors.New("Core installation busy")
	}
	a.provisionRefresh = true
	return service.SetNodeSetting(database.GetDB(), "nodeCoreGeneration", generation)
}

// syncWhileProvisioning keeps the running inventory during install/uninstall.
// New cores are activated after prerequisite verification. Quota changes, revoked
// users, explicit node stops and the central lease continue to take effect.
func (a *Agent) syncWhileProvisioning(snap *nodewire.Snapshot) error {
	if !snap.Enable || snap.ControlGeneration != nodeSettingInt("nodeControlGeneration") {
		a.stopRuntime()
	}
	if a.runtime == nil {
		a.lastSuccess = time.Now()
		return nil
	}
	installed := map[string]bool{}
	if err := json.Unmarshal([]byte(a.inventory), &installed); err != nil {
		a.stopRuntime()
		return fmt.Errorf("invalid running core inventory: %w", err)
	}
	if template := service.GetNodeSetting("nodeXrayTemplate"); template != "" {
		snap.Template = template
	}
	for i := range snap.Inbounds {
		if service.NodeCorePaused(service.NodeCoreForProtocol(snap.Inbounds[i].Protocol)) {
			snap.Inbounds[i].Enable = false
		}
	}
	revision, err := service.ApplyNodeSnapshot(snap, installed)
	if err != nil {
		a.stopRuntime()
		return atSyncStage("snapshot", err)
	}
	if revision != a.revision {
		if err := a.runtime.RefreshNodeRuntime(); err != nil {
			a.stopRuntime()
			return atSyncStage("runtime", err)
		}
	}
	a.revision = revision
	a.lastSuccess = time.Now()
	a.lastError = ""
	return nil
}

func (a *Agent) Tick(ctx context.Context) error {
	timeout := 25 * time.Second
	if a.runtime != nil && !a.lastSuccess.IsZero() {
		remaining := time.Until(a.lastSuccess.Add(nodewire.LeaseSeconds * time.Second))
		if remaining <= 0 {
			a.stopRuntime()
		} else if remaining < timeout {
			timeout = remaining
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	report, e := a.pendingReport()
	if e != nil {
		return e
	}
	var snap nodewire.Snapshot
	if e = exchange(ctx, a.client, a.config, strconv.Itoa(a.config.NodeID)+"/sync", report, &snap); e != nil {
		return atSyncStage("exchange", e)
	}
	if snap.NodeID != a.config.NodeID || snap.Version != nodewire.Version {
		return errors.New("Wrong node snapshot")
	}
	if e = ackReport(report.Sequence); e != nil {
		return e
	}
	if e = a.requestCores(&snap); e != nil {
		return atSyncStage("installation", e)
	}
	if (&service.CoreService{}).CoreUninstallStatus().Running {
		return a.syncWhileProvisioning(&snap)
	}
	if (&service.CoreService{}).ProvisionState().Running {
		return a.syncWhileProvisioning(&snap)
	}
	if snap.CoreActionCommand != nil && snap.CoreActionGeneration != nodeSettingInt("nodeCoreActionGeneration") {
		a.applyCoreAction(snap.CoreActionGeneration, *snap.CoreActionCommand)
	}
	if (&service.CoreService{}).CoreUninstallStatus().Running {
		return a.syncWhileProvisioning(&snap)
	}
	if snap.ControlGeneration != nodeSettingInt("nodeControlGeneration") {
		a.stopRuntime()
	}
	if template := service.GetNodeSetting("nodeXrayTemplate"); template != "" {
		snap.Template = template
	}
	for i := range snap.Inbounds {
		if service.NodeCorePaused(service.NodeCoreForProtocol(snap.Inbounds[i].Protocol)) {
			snap.Inbounds[i].Enable = false
		}
	}
	installed := installedCores()
	inventoryRaw, _ := json.Marshal(installed)
	inventory := string(inventoryRaw)
	revision, e := service.ApplyNodeSnapshot(&snap, installed)
	if e != nil {
		return atSyncStage("snapshot", e)
	}
	a.lastSuccess = time.Now()
	if snap.ConfigCommand != nil && snap.ConfigCommand.Inspect && snap.ConfigGeneration != nodeSettingInt("nodeConfigGeneration") {
		a.applyConfigCommand(snap.ConfigGeneration, *snap.ConfigCommand)
	}
	if !snap.Enable {
		a.stopRuntime()
		return service.SetNodeSetting(database.GetDB(), "nodeControlGeneration", strconv.FormatInt(snap.ControlGeneration, 10))
	}
	if len(installed) == 0 {
		if a.runtime != nil {
			a.stopRuntime()
		}
		return service.SetNodeSetting(database.GetDB(), "nodeControlGeneration", strconv.FormatInt(snap.ControlGeneration, 10))
	}
	if a.runtime == nil {
		// Update an already-installed managed Xray when the agent is upgraded.
		// Fresh nodes and nodes where Xray was removed still install nothing.
		if installed["xray"] {
			if _, e = corebundle.ExtractXray(config.GetBinFolderPath()); e != nil {
				return atSyncStage("runtime", e)
			}
		}
		a.runtime = web.NewServer()
		if e = a.runtime.StartNodeRuntime(); e != nil {
			a.stopRuntime()
			return atSyncStage("runtime", e)
		}
		a.coreRestart = ""
	} else if revision != a.revision || inventory != a.inventory || a.provisionRefresh {
		if e = a.runtime.RefreshNodeRuntime(); e != nil {
			return atSyncStage("runtime", e)
		}
	}
	a.provisionRefresh = false
	_ = service.SetNodeSetting(database.GetDB(), "nodeRuntimeRunning", "true")
	if e = service.SetNodeSetting(database.GetDB(), "nodeControlGeneration", strconv.FormatInt(snap.ControlGeneration, 10)); e != nil {
		return e
	}
	if snap.ConfigCommand != nil && snap.ConfigGeneration != nodeSettingInt("nodeConfigGeneration") {
		a.applyConfigCommand(snap.ConfigGeneration, *snap.ConfigCommand)
	}
	if a.coreRestart != "" && a.runtime != nil {
		if e = a.runtime.RefreshNodeCore(a.coreRestart, a.coreRestartForce); e != nil {
			_ = service.SetNodeSetting(database.GetDB(), "nodeCoreActionError", e.Error())
			return e
		}
		a.coreRestart = ""
	}
	a.revision = revision
	a.inventory = inventory
	a.lastError = ""
	return nil
}
func (a *Agent) stopRuntime() {
	_ = service.SetNodeSetting(database.GetDB(), "nodeRuntimeRunning", "false")
	if e := service.DisableNodeAdmission(); e != nil {
		log.Printf("disable node admission: %v", e)
	}
	if a.runtime != nil {
		if e := a.runtime.StopNodeRuntime(); e != nil {
			log.Printf("stop node runtime: %v", e)
		}
		a.runtime = nil
	}
}
func (a *Agent) Run(ctx context.Context) error {
	if e := prepareDatabase(); e != nil {
		return e
	}
	defer a.stopRuntime()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if e := a.Tick(ctx); e != nil {
			// Never print configuration bodies or bearer/pairing credentials.
			a.lastError = publicSyncError(e)
			log.Printf("node synchronization: %v", e)
			if a.lastSuccess.IsZero() || time.Since(a.lastSuccess) >= nodewire.LeaseSeconds*time.Second {
				a.stopRuntime()
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func nodeSettingInt(key string) int64 {
	n, _ := strconv.ParseInt(service.GetNodeSetting(key), 10, 64)
	return n
}
func (a *Agent) applyConfigCommand(generation int64, cmd nodewire.ConfigCommand) {
	cs := &service.CoreService{}
	var err error
	if !cmd.Inspect {
		if cmd.Reset {
			cmd.Text = ""
			if cmd.Core == "xray" {
				cmd.Text = service.DefaultNodeTemplate()
			}
		}
		if cmd.Core == "radius" && cmd.File == "radius.json" {
			err = a.applyRadiusConfig(cmd.Text, cmd.Reset)
		} else if cmd.Core == "xray" {
			err = a.applyXrayConfig(cmd.Text)
		} else {
			var health *service.CoreConfigHealth
			health, err = cs.SaveCoreConfigOverride(cmd.Core, cmd.Inbound, cmd.File, service.CoreConfigModeReplace, cmd.Text)
			if err == nil && health != nil && health.Reverted {
				err = errors.New("Configuration failed; previous configuration restored\n" + health.Logs)
			}
		}
	}
	message := ""
	if err != nil {
		message = err.Error()
		if len(message) > 4096 {
			message = message[:4096]
		}
	}
	_ = service.SetNodeSetting(database.GetDB(), "nodeConfigError", message)
	targets := cs.CoreConfigTargets(cmd.Core)
	if cmd.Core == "xray" {
		template, _ := (&service.SettingService{}).GetXrayConfigTemplate()
		target := map[string]any{"core": "xray", "inbound": 0, "file": "config.json", "label": "Xray template", "current": template}
		// Read the actual runtime file, not another generated candidate: viewing
		// configuration must not run traffic collection or mutate account state.
		if info, err := os.Stat(xray.GetConfigPath()); err == nil && info.Size() <= 384*1024 {
			if raw, err := os.ReadFile(xray.GetConfigPath()); err == nil && json.Valid(raw) {
				target["generated"] = string(raw)
				if encoded, _ := json.Marshal(target); len(encoded) > 700*1024 {
					delete(target, "generated")
				}
			}
		}
		targets = []map[string]any{target}
	}
	if cmd.Core == "radius" {
		secret, _ := (&service.SettingService{}).GetRadiusSecret()
		raw, _ := json.MarshalIndent(map[string]string{"secret": secret}, "", "  ")
		targets = []map[string]any{{"core": "radius", "inbound": 0, "file": "radius.json", "label": "radius.json", "current": string(raw)}}
	}
	raw, _ := json.Marshal(targets)
	if len(raw) > 768*1024 {
		raw = []byte("[]")
		_ = service.SetNodeSetting(database.GetDB(), "nodeConfigError", "Configuration target list exceeds report size limit")
	}
	_ = service.SetNodeSetting(database.GetDB(), "nodeConfigTargets", string(raw))
	_ = service.SetNodeSetting(database.GetDB(), "nodeConfigGeneration", strconv.FormatInt(generation, 10))
}

// Validate the fully generated configuration with this node's own Xray and geo files.
func (a *Agent) applyXrayConfig(text string) (err error) {
	if err = service.ValidateNodeTemplate(text); err != nil {
		return err
	}
	settings := &service.XraySettingService{}
	previous, err := settings.GetXrayConfigTemplate()
	if err != nil {
		return err
	}
	if _, missing := service.EnsureGeofiles(service.ExtGeoRefs([]byte(text))); len(missing) > 0 {
		return fmt.Errorf("Missing node geo files: %s", strings.Join(missing, ", "))
	}
	cfg, err := (&service.XrayService{}).GetXrayConfigWithTemplate(text)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp("", "node-xray-check-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, xray.GetBinaryPath(), "-test", "-c", f.Name()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Xray rejected configuration: %s", string(output))
	}
	if err = settings.SaveXraySetting(text); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = settings.RepairXrayTemplate(previous)
			if a.runtime != nil {
				_ = a.runtime.RefreshNodeRuntime()
			}
		}
	}()
	if a.runtime != nil {
		if err = a.runtime.RefreshNodeRuntime(); err != nil {
			return err
		}
	}
	return service.SetNodeSetting(database.GetDB(), "nodeXrayTemplate", text)
}

func (a *Agent) applyCoreAction(generation int64, cmd nodewire.CoreAction) {
	err := a.executeCoreAction(cmd)
	message := ""
	if err != nil {
		message = err.Error()
		if len(message) > 4096 {
			message = message[:4096]
		}
	}
	_ = service.SetNodeSetting(database.GetDB(), "nodeCoreActionError", message)
	_ = service.SetNodeSetting(database.GetDB(), "nodeCoreActionGeneration", strconv.FormatInt(generation, 10))
}
func (a *Agent) executeCoreAction(cmd nodewire.CoreAction) (err error) {
	cs := &service.CoreService{}
	if !installedCores()[cmd.Core] {
		return errors.New("Core is not installed.")
	}

	if (cmd.Core == "xray" || cmd.Core == "radius" || cmd.Core == "ipsec") && (cmd.Action == "uninstall" || cmd.Action == "stop") {
		radiusDeps := map[string]bool{"l2tp": true, "pptp": true, "openvpn": true, "openconnect": true, "sstp": true, "ikev2": true}
		ipsecDeps := map[string]bool{"l2tp": true, "ikev2": true, "gre": true}
		for core := range installedCores() {
			dependent := cmd.Core == "xray" || (cmd.Core == "radius" && radiusDeps[core]) || (cmd.Core == "ipsec" && ipsecDeps[core])
			if core != cmd.Core && core != "radius" && dependent && ((cmd.Core == "xray" && cmd.Action == "uninstall") || !service.NodeCorePaused(core)) {
				return errors.New("Stop or remove dependent cores first.")
			}
		}
	}

	switch cmd.Action {
	case "stop":
		if a.runtime != nil {
			a.runtime.FlushNodeTraffic()
		}
		if err := service.SetNodeCorePaused(cmd.Core, true); err != nil {
			return err
		}
		if cmd.Core == "ikev2" {
			return nil
		} // Shared charon is reconciled after disabling its inbounds.
		if cmd.Core == "xray" && !(&service.XrayService{}).IsXrayRunning() {
			return nil
		}
		return cs.StopCore(cmd.Core)
	case "start", "restart":
		if err := service.SetNodeCorePaused(cmd.Core, false); err != nil {
			return err
		}
		if cmd.Action == "restart" {
			if a.runtime != nil {
				a.runtime.FlushNodeTraffic()
			}
			if cmd.Core != "xray" || (&service.XrayService{}).IsXrayRunning() {
				if err := cs.StopCore(cmd.Core); err != nil {
					return err
				}
			}
		}
		a.coreRestart = cmd.Core
		a.coreRestartForce = cmd.Action == "restart"
		return nil
	case "uninstall":
		wasPaused := service.NodeCorePaused(cmd.Core)
		defer func() {
			if err != nil {
				_ = service.SetNodeCorePaused(cmd.Core, wasPaused)
			}
		}()
		// Keep the node scheduler and unrelated daemons alive. Pausing admission
		// for this core prevents its supervisor from recreating removed files.
		if a.runtime != nil {
			a.runtime.FlushNodeTraffic()
		}
		if err := service.SetNodeCorePaused(cmd.Core, true); err != nil {
			return err
		}
		switch cmd.Core {
		case "xray":
			if (&service.XrayService{}).IsXrayRunning() {
				if err := cs.StopCore("xray"); err != nil {
					return err
				}
			}
			return os.Remove(xray.GetBinaryPath())
		case "ssh":
			if err := cs.StopCore("ssh"); err != nil {
				return err
			}
			return service.SetNodeSetting(database.GetDB(), "nodeSSHInstalled", "false")
		case "radius":
			if err := cs.StopCore("radius"); err != nil {
				return err
			}
			return service.SetNodeSetting(database.GetDB(), "nodeRadiusRemoved", "true") // Built into the agent; removal disables its listener.
		default:
			started, err := cs.StartCoreUninstall([]string{cmd.Core}, service.InboundsKeep)
			if err != nil {
				return err
			}
			if !started {
				return errors.New("Uninstall busy")
			}
			return nil
		}
	default:
		return errors.New("Unknown core action")
	}
}

func (a *Agent) applyRadiusConfig(text string, reset bool) error {
	var cfg struct {
		Secret string `json:"secret"`
	}
	if reset {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return err
		}
		cfg.Secret = hex.EncodeToString(key)
	} else {
		dec := json.NewDecoder(strings.NewReader(text))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cfg); err != nil {
			return err
		}
	}
	if len(cfg.Secret) < 16 || len(cfg.Secret) > 128 {
		return errors.New("RADIUS secret must contain 16–128 characters")
	}
	for _, r := range cfg.Secret {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return errors.New("RADIUS secret must be alphanumeric, underscore or hyphen")
		}
	}
	// Recreate the runtime on the next snapshot so every daemon receives the same
	// new shared secret, preserving final accounting before the reconfiguration.
	a.stopRuntime()
	return (&service.SettingService{}).SetRadiusSecret(cfg.Secret)
}
