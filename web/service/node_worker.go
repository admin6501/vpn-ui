package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/nodewire"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
)

// SetNodeSetting is private agent state in its own database, never the central DB.
func SetNodeSetting(tx *gorm.DB, key, value string) error {
	var row model.Setting
	e := tx.Where("key = ?", key).First(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return tx.Create(&model.Setting{Key: key, Value: value}).Error
	}
	if e != nil {
		return e
	}
	if row.Value == value {
		return nil
	}
	return tx.Model(&row).Update("value", value).Error
}
func GetNodeSetting(key string) string {
	var row model.Setting
	if database.GetDB().Where("key = ?", key).First(&row).Error != nil {
		return ""
	}
	return row.Value
}

// ApplyNodeSnapshot never imports administrators or resets local meter counters.
// Keeping those cumulative counters lets delivery survive network loss and restart.
func ApplyNodeSnapshot(snap *nodewire.Snapshot, installed map[string]bool) (string, error) {
	if snap.Version != nodewire.Version || snap.NodeID <= 0 || snap.LeaseSeconds != nodewire.LeaseSeconds {
		return "", errors.New("Incompatible node protocol")
	}
	if err := validateNodeInboundIdentities(snap.Inbounds); err != nil {
		return "", err
	}
	if len(snap.Files) > 1000 {
		return "", errors.New("Too many certificate files")
	}
	certDir := filepath.Join(config.GetBinFolderPath(), "node-certs")
	if e := os.MkdirAll(certDir, 0700); e != nil {
		return "", e
	}
	replacements := map[string]string{}
	for name, data := range snap.Files {
		sum := sha256.Sum256(data)
		if name != hex.EncodeToString(sum[:]) || len(data) > 4<<20 {
			return "", errors.New("Invalid certificate bundle")
		}
		dest := filepath.Join(certDir, name)
		if e := os.WriteFile(dest+".tmp", data, 0600); e != nil {
			return "", e
		}
		if e := os.Rename(dest+".tmp", dest); e != nil {
			return "", e
		}
		replacements["nodefile:"+name] = dest
	}
	revision := NodeSnapshotRevision(snap)
	e := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := reserveNodeInboundTags(tx, snap.Inbounds); err != nil {
			return err
		}
		if e := tx.Model(&model.Inbound{}).Where("1 = 1").Update("enable", false).Error; e != nil {
			return e
		}
		if e := tx.Model(&model.Account{}).Where("1 = 1").Update("enable", false).Error; e != nil {
			return e
		}
		if e := tx.Model(&xray.ClientTraffic{}).Where("1 = 1").Update("enable", false).Error; e != nil {
			return e
		}
		if e := tx.Model(&model.AccountInbound{}).Where("1 = 1").Update("enable", false).Error; e != nil {
			return e
		}
		for _, source := range snap.Inbounds {
			source.NodeID = 0
			source.NodeAddress = ""
			source.NodeName = ""
			source.UserId = 1
			source.ClientStats = nil
			source.Enable = source.Enable && snap.Enable && installed[NodeCoreForProtocol(source.Protocol)] && !NodeCorePaused(NodeCoreForProtocol(source.Protocol))
			source.TrafficMultiplierEnable = false
			source.TrafficReset = "never"
			source.LastTrafficResetTime = 0
			for from, to := range replacements {
				source.Settings = strings.ReplaceAll(source.Settings, from, to)
				source.StreamSettings = strings.ReplaceAll(source.StreamSettings, from, to)
			}
			if strings.Contains(source.Settings, "nodefile:") || strings.Contains(source.StreamSettings, "nodefile:") {
				return errors.New("Missing node certificate")
			}
			var old model.Inbound
			if e := tx.First(&old, source.Id).Error; e == nil {
				source.Up = old.Up
				source.Down = old.Down
				source.AllTime = old.AllTime
			} else if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			} else {
				source.Up = 0
				source.Down = 0
				source.AllTime = 0
			}
			if e := tx.Save(&source).Error; e != nil {
				return e
			}
		}
		for _, wrapped := range snap.Accounts {
			a := wrapped.Account
			// Account identity is its central ID. Preserve its local meter when
			// the operator renames its email, and retain tombstones when an email
			// is reused by a newly created central account.
			var collision model.Account
			if e := tx.Where("email = ? AND id <> ?", a.Email, a.Id).First(&collision).Error; e == nil {
				archived := fmt.Sprintf("node-archived-%d@invalid", collision.Id)
				if e := tx.Model(&xray.ClientTraffic{}).Where("email = ?", collision.Email).Update("email", archived).Error; e != nil {
					return e
				}
				if e := tx.Model(&collision).Update("email", archived).Error; e != nil {
					return e
				}
			} else if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			var previous model.Account
			if e := tx.First(&previous, a.Id).Error; e == nil && previous.Email != a.Email {
				if e := tx.Model(&xray.ClientTraffic{}).Where("email = ?", previous.Email).Update("email", a.Email).Error; e != nil {
					return e
				}
			} else if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			a.ProtocolCredentials = wrapped.ProtocolCredentials
			a.ImportedResetDays = 0
			a.ImportedResetDisabled = true
			a.Reset = 0
			a.Enable = wrapped.Allowed && snap.Enable
			a.RepresentativeBlocked = false
			var traffic xray.ClientTraffic
			e := tx.Where("email = ?", a.Email).First(&traffic).Error
			if errors.Is(e, gorm.ErrRecordNotFound) {
				traffic = xray.ClientTraffic{Email: a.Email}
			} else if e != nil {
				return e
			}
			traffic.Enable = a.Enable
			traffic.ExpiryTime = a.ExpiryTime
			traffic.Reset = 0
			if wrapped.Remaining < 0 {
				a.TotalGB = 0
			} else {
				a.TotalGB = wrapped.ReportedUsage + wrapped.Remaining
				if a.TotalGB == 0 {
					a.TotalGB = 1
				}
			}
			if a.TotalGB > 0 && traffic.Up+traffic.Down >= a.TotalGB {
				a.Enable = false
				traffic.Enable = false
			}
			traffic.Total = a.TotalGB
			if e := tx.Save(&a).Error; e != nil {
				return e
			}
			if e := tx.Save(&traffic).Error; e != nil {
				return e
			}
		}
		for _, m := range snap.Memberships {
			var old model.AccountInbound
			if e := tx.Where("account_id = ? AND inbound_id = ?", m.AccountId, m.InboundId).First(&old).Error; e == nil {
				m.Up = old.Up
				m.Down = old.Down
				m.AllTime = old.AllTime
			} else if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			} else {
				m.Up = 0
				m.Down = 0
				m.AllTime = 0
			}
			enabled := (m.Enable == nil || *m.Enable) && snap.Enable
			m.Enable = &enabled
			if e := tx.Save(&m).Error; e != nil {
				return e
			}
		}
		// Central period usage controls speed thresholds; local meters are cumulative
		// report cursors and deliberately survive resets. Never zero those meters.
		speedUsageJSON, err := nodeSpeedUsageJSON(snap.Accounts)
		if err != nil {
			return err
		}
		if err := SetNodeSetting(tx, "nodeSpeedUsage", string(speedUsageJSON)); err != nil {
			return err
		}
		// Projection synchronizes protocol-specific credentials, membership state and
		// quota into every daemon's existing clients shape.
		if e := projectNodeAccounts(tx, snap.Accounts); e != nil {
			return fmt.Errorf("node account projection: %w", e)
		}
		for key, value := range map[string]string{"accountsMigratedAt": "node-v1", "membershipUsageMigratedAt": "node-v1", "xrayTemplateConfig": snap.Template, "subEnable": "false", "tgbotEnabled": "false"} {
			if e := SetNodeSetting(tx, key, value); e != nil {
				return e
			}
		}
		return nil
	})
	if e == nil {
		WriteSpeedLimits()
	}
	return revision, e
}

func nodeSpeedUsageJSON(accounts []nodewire.Account) ([]byte, error) {
	speedUsage := make(map[string]int64)
	for _, a := range accounts {
		if a.Usage != nil {
			if *a.Usage < 0 {
				return nil, errors.New("Negative node speed usage")
			}
			speedUsage[a.Account.Email] = *a.Usage
		}
	}
	return json.Marshal(speedUsage)
}

// Quota-only heartbeats still update period usage for speed thresholds even
// when their materialized credentials and admission budget have not changed.
func UpdateNodeSpeedUsage(accounts []nodewire.Account) error {
	raw, err := nodeSpeedUsageJSON(accounts)
	if err != nil {
		return err
	}
	if GetNodeSetting("nodeSpeedUsage") == string(raw) {
		return nil
	}
	if err := SetNodeSetting(database.GetDB(), "nodeSpeedUsage", string(raw)); err != nil {
		return err
	}
	WriteSpeedLimits()
	return nil
}

// NodeSnapshotRevision excludes meter-only changes that never need a daemon reload.
func NodeSnapshotRevision(snap *nodewire.Snapshot) string {
	// Changes to quota balances are consumed directly from SQLite; they must not
	// restart every daemon whenever a user downloads another byte.
	identity := *snap
	identity.Files = nil
	// Management commands and acknowledgments do not change the data plane.
	identity.ControlGeneration = 0
	identity.CoreGeneration = 0
	identity.RequestedCores = nil
	identity.ConfigGeneration = 0
	identity.ConfigCommand = nil
	identity.CoreActionGeneration = 0
	identity.CoreActionCommand = nil
	identity.Inbounds = append([]model.Inbound(nil), snap.Inbounds...)
	identity.Memberships = append([]model.AccountInbound(nil), snap.Memberships...)
	for i := range identity.Memberships {
		identity.Memberships[i].Up = 0
		identity.Memberships[i].Down = 0
		identity.Memberships[i].AllTime = 0
	}
	// Clone the slice before normalizing the hash so the actual snapshot is intact.
	identity.Accounts = append([]nodewire.Account(nil), snap.Accounts...)
	for i := range identity.Accounts {
		identity.Accounts[i].Remaining = 0
		identity.Accounts[i].ReportedUsage = 0
		identity.Accounts[i].Usage = nil
	}
	for i := range identity.Inbounds {
		identity.Inbounds[i].Up = 0
		identity.Inbounds[i].Down = 0
		identity.Inbounds[i].AllTime = 0
	}
	raw, _ := json.Marshal(identity)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

// Project a whole authenticated snapshot in memory. ProjectAccount scans and
// writes inbounds separately for each account; doing that on every heartbeat
// turns a node with hundreds of clients into hundreds of repeated DB scans and
// rewrites of the same inbound. Preserve the same projection/removal semantics
// while loading each table once and writing each inbound at most once.
func projectNodeAccounts(tx *gorm.DB, requested []nodewire.Account) error {
	var accounts []model.Account
	if err := tx.Find(&accounts).Error; err != nil {
		return err
	}
	byID := make(map[int]*model.Account, len(accounts))
	for i := range accounts {
		byID[accounts[i].Id] = &accounts[i]
	}
	var memberships []model.AccountInbound
	if err := tx.Find(&memberships).Error; err != nil {
		return err
	}
	member := make(map[int]map[int]*model.AccountInbound)
	for i := range memberships {
		m := &memberships[i]
		if member[m.AccountId] == nil {
			member[m.AccountId] = make(map[int]*model.AccountInbound)
		}
		member[m.AccountId][m.InboundId] = m
	}
	var inbounds []model.Inbound
	if err := tx.Order("id ASC").Find(&inbounds).Error; err != nil {
		return err
	}
	for i := range inbounds {
		inbound := &inbounds[i]
		before := inbound.Settings
		for _, source := range requested {
			a := byID[source.Account.Id]
			if a == nil {
				return errors.New("Missing node account")
			}
			var err error
			if m := member[a.Id][inbound.Id]; m != nil {
				inbound.Settings, err = projectAccountOntoInbound(inbound, a, m)
			} else if strings.Contains(strings.ToLower(inbound.Settings), accountKey(a.Email)) {
				inbound.Settings, _, err = removeAccountFromInbound(inbound, a.Email)
			}
			if err != nil {
				return err
			}
		}
		if inbound.Settings != before {
			if err := tx.Model(inbound).Update("settings", inbound.Settings).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// A node can remain connected to the control plane while its data plane is
// stopped. Clearing admission before stopping also prevents an asynchronous
// provisioning task from bringing expired cached permissions back up.
func DisableNodeAdmission() error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		for _, m := range []any{&model.Inbound{}, &model.Account{}, &xray.ClientTraffic{}, &model.AccountInbound{}} {
			if e := tx.Model(m).Where("1 = 1").Update("enable", false).Error; e != nil {
				return e
			}
		}
		return nil
	})
}

func ReadNodeCounters(tx *gorm.DB) ([]nodewire.Counter, error) {
	counters := []nodewire.Counter{}
	var accounts []model.Account
	if e := tx.Find(&accounts).Error; e != nil {
		return nil, e
	}
	byEmail := map[string]int{}
	for _, a := range accounts {
		byEmail[accountKey(a.Email)] = a.Id
	}
	var traffics []xray.ClientTraffic
	if e := tx.Find(&traffics).Error; e != nil {
		return nil, e
	}
	for _, t := range traffics {
		if id := byEmail[accountKey(t.Email)]; id > 0 {
			counters = append(counters, nodewire.Counter{Kind: "account", AccountID: id, Up: t.Up, Down: t.Down})
		}
	}
	var memberships []model.AccountInbound
	if e := tx.Find(&memberships).Error; e != nil {
		return nil, e
	}
	for _, m := range memberships {
		counters = append(counters, nodewire.Counter{Kind: "membership", AccountID: m.AccountId, InboundID: m.InboundId, Up: m.Up, Down: m.Down})
	}
	var inbounds []model.Inbound
	if e := tx.Find(&inbounds).Error; e != nil {
		return nil, e
	}
	for _, i := range inbounds {
		counters = append(counters, nodewire.Counter{Kind: "inbound", InboundID: i.Id, Up: i.Up, Down: i.Down})
	}
	return counters, nil
}
func ReadNodeOnline() []nodewire.Online {
	var accounts []model.Account
	database.GetDB().Where("enable = ?", true).Select("id,email").Find(&accounts)
	ids := map[string]int{}
	for _, a := range accounts {
		ids[accountKey(a.Email)] = a.Id
	}
	var active []model.Inbound
	database.GetDB().Where("enable = ?", true).Select("id").Find(&active)
	enabled := map[int]bool{}
	for _, in := range active {
		enabled[in.Id] = true
	}
	var memberships []model.AccountInbound
	database.GetDB().Where("enable = ?", true).Find(&memberships)
	admitted := map[int]bool{}
	for _, m := range memberships {
		if enabled[m.InboundId] {
			admitted[m.AccountId] = true
		}
	}
	result := []nodewire.Online{}
	svc := &InboundService{}
	seen := map[string]bool{}
	for _, pair := range svc.GetOnlineMemberships() {
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			continue
		}
		var inbound int
		_, e := fmt.Sscanf(parts[0], "%d", &inbound)
		if e == nil && enabled[inbound] && ids[accountKey(parts[1])] > 0 {
			result = append(result, nodewire.Online{AccountID: ids[accountKey(parts[1])], InboundID: inbound})
			seen[accountKey(parts[1])] = true
		}
	}
	for _, email := range svc.GetOnlineClients() {
		key := accountKey(email)
		if !seen[key] && ids[key] > 0 && admitted[ids[key]] {
			result = append(result, nodewire.Online{AccountID: ids[key]})
		}
	}
	return result
}

// Agent-only durable stop switches prevent routine jobs from restarting stopped cores.
func NodeCorePaused(core string) bool {
	if os.Getenv("VPNUI_NODE_AGENT") != "1" || database.GetDB() == nil {
		return false
	}
	return GetNodeSetting("nodeCorePaused:"+core) == "true"
}
func SetNodeCorePaused(core string, paused bool) error {
	return SetNodeSetting(database.GetDB(), "nodeCorePaused:"+core, fmt.Sprint(paused))
}
