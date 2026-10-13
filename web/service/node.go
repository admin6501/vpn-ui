package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/nodewire"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NodeService struct{}

func nodeSecret() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func NodeTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func nodeInboundTag(id int, tag string) string {
	if id == 0 || strings.HasPrefix(tag, fmt.Sprintf("node-%d-", id)) {
		return tag
	}
	return fmt.Sprintf("node-%d-%s", id, tag)
}
func validNodeAddress(address string) bool {
	if address == "" || strings.ContainsAny(address, "/:?#@ \t\r\n") {
		return net.ParseIP(address) != nil
	}
	if len(address) > 253 {
		return false
	}
	for _, label := range strings.Split(address, ".") {
		if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
func (s *NodeService) Create(name, address string) (*model.RemoteNode, string, error) {
	name = strings.TrimSpace(name)
	address = strings.TrimSpace(address)
	if name == "" || len(name) > 100 || !validNodeAddress(address) {
		return nil, "", fmt.Errorf("Invalid node name/address / نام یا آدرس نود نامعتبر است")
	}
	token, e := nodeSecret()
	if e != nil {
		return nil, "", e
	}
	n := &model.RemoteNode{Name: name, Address: address, Enable: true, PairHash: NodeTokenHash(token), PairExpiresAt: time.Now().Add(15 * time.Minute).UnixMilli(), RequestedCores: "[]", Online: "[]"}
	e = database.GetDB().Create(n).Error
	return n, token, e
}
func (s *NodeService) Enroll(request nodewire.Enrollment) (int, error) {
	if len(request.PairToken) != 64 || len(request.AgentToken) != 64 || (request.Architecture != "amd64" && request.Architecture != "arm64") {
		return 0, errors.New("Invalid enrollment")
	}
	var n model.RemoteNode
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if e := tx.Where("pair_hash = ?", NodeTokenHash(request.PairToken)).First(&n).Error; e != nil {
			return e
		}
		hash := NodeTokenHash(request.AgentToken)
		if n.TokenHash != "" {
			if subtle.ConstantTimeCompare([]byte(hash), []byte(n.TokenHash)) == 1 {
				return nil
			}
			return errors.New("Pairing token already used")
		}
		if !n.Enable || n.PairExpiresAt < time.Now().UnixMilli() {
			return errors.New("Pairing token expired")
		}
		return tx.Model(&n).Updates(map[string]any{"token_hash": hash, "agent_version": request.Version, "architecture": request.Architecture}).Error
	})
	return n.ID, err
}
func (s *NodeService) Authenticate(id int, token string) (*model.RemoteNode, error) {
	var n model.RemoteNode
	if len(token) != 64 {
		return nil, errors.New("Invalid node credential")
	}
	if e := database.GetDB().First(&n, id).Error; e != nil {
		return nil, e
	}
	if n.TokenHash == "" || subtle.ConstantTimeCompare([]byte(NodeTokenHash(token)), []byte(n.TokenHash)) != 1 {
		return nil, errors.New("Invalid node credential")
	}
	return &n, nil
}
func (s *NodeService) Rotate(id int) (string, error) {
	token, e := nodeSecret()
	if e != nil {
		return "", e
	}
	var n model.RemoteNode
	e = database.GetDB().Transaction(func(tx *gorm.DB) error {
		if e := tx.First(&n, id).Error; e != nil {
			return e
		}
		if n.TokenHash != "" {
			return errors.New("Node already paired; update the agent without replacing its database / نود قبلاً متصل شده؛ عامل را بدون حذف دیتابیس به‌روز کنید")
		}
		// A new installation has new cumulative counters; rotation resets only node
		// cursors, never the account's already billed traffic.
		if e := tx.Where("node_id = ?", id).Delete(&model.NodeCounter{}).Error; e != nil {
			return e
		}
		return tx.Model(&n).Updates(map[string]any{"pair_hash": NodeTokenHash(token), "pair_expires_at": time.Now().Add(15 * time.Minute).UnixMilli(), "token_hash": "", "last_sequence": 0, "last_seen": 0, "online": "[]"}).Error
	})
	return token, e
}
func (s *NodeService) Update(id int, name, address string, enable bool) error {
	if strings.TrimSpace(name) == "" || len(name) > 100 || !validNodeAddress(address) {
		return errors.New("Invalid node name/address / نام یا آدرس نود نامعتبر است")
	}
	return database.GetDB().Model(&model.RemoteNode{}).Where("id = ?", id).Updates(map[string]any{"name": strings.TrimSpace(name), "address": address, "enable": enable}).Error
}
func (s *NodeService) Delete(id int) error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		var count int64
		if e := tx.Model(&model.Inbound{}).Where("node_id = ?", id).Count(&count).Error; e != nil {
			return e
		}
		if count > 0 {
			return errors.New("Remove this node's inbounds first / ابتدا ورودی‌های این نود را حذف کنید")
		}
		if e := tx.Where("node_id = ?", id).Delete(&model.NodeCounter{}).Error; e != nil {
			return e
		}
		return tx.Delete(&model.RemoteNode{}, id).Error
	})
}
func (s *NodeService) InstallCores(id int, cores []string) error {
	allowed := map[string]bool{"xray": true, "ssh": true, "radius": true}
	for _, c := range coreCatalog {
		allowed[c.name] = true
	}
	if len(cores) == 0 {
		return errors.New("Select at least one core / حداقل یک هسته انتخاب کنید")
	}
	set := map[string]bool{}
	for _, c := range cores {
		if !allowed[c] {
			return fmt.Errorf("Unknown core %q", c)
		}
		set[c] = true
	}
	cores = nil
	for c := range set {
		cores = append(cores, c)
	}
	sort.Strings(cores)
	b, _ := json.Marshal(cores)
	return database.GetDB().Model(&model.RemoteNode{}).Where("id = ?", id).Updates(map[string]any{"requested_cores": string(b), "core_generation": gorm.Expr("core_generation + 1")}).Error
}
func NodeCoreForProtocol(protocol model.Protocol) string {
	name := protocolCoreName(string(protocol))
	if name == "" {
		if protocol == model.SSH {
			return "ssh"
		}
		return "xray"
	}
	return name
}
func (s *NodeService) ProtocolReady(id int, p model.Protocol) error {
	var n model.RemoteNode
	if e := database.GetDB().First(&n, id).Error; e != nil {
		return e
	}
	if !n.Enable {
		return errors.New("Node disabled / نود غیرفعال است")
	}
	var state struct {
		Installed []string `json:"installed"`
	}
	_ = json.Unmarshal([]byte(n.CoreReport), &state)
	want := NodeCoreForProtocol(p)
	for _, v := range state.Installed {
		if v == want {
			return nil
		}
	}
	return errors.New("Install the required core on this node first / ابتدا هسته موردنیاز را در صفحه این نود نصب کنید")
}
func validateNodeInbound(inbound *model.Inbound) error {
	if inbound.NodeID < 0 {
		return errors.New("Invalid node")
	}
	if inbound.Id != 0 {
		var old model.Inbound
		if e := database.GetDB().First(&old, inbound.Id).Error; e == nil && old.NodeID != inbound.NodeID {
			return errors.New("Create a new inbound to change servers / برای تغییر سرور، ورودی جدید ایجاد کنید")
		}
	}
	if inbound.NodeID == 0 {
		return nil
	}
	var n model.RemoteNode
	if e := database.GetDB().First(&n, inbound.NodeID).Error; e != nil {
		return errors.New("Node not found / نود پیدا نشد")
	}
	return nil
}
func attachNodeAddresses(db *gorm.DB, inbounds []*model.Inbound) error {
	ids := map[int]bool{}
	for _, i := range inbounds {
		if i.NodeID != 0 {
			ids[i.NodeID] = true
		}
	}
	if len(ids) == 0 {
		return nil
	}
	list := make([]int, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	var nodes []model.RemoteNode
	if e := db.Where("id IN ?", list).Find(&nodes).Error; e != nil {
		return e
	}
	byID := map[int]model.RemoteNode{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	for _, i := range inbounds {
		if n, ok := byID[i.NodeID]; ok {
			i.NodeAddress = n.Address
			i.NodeName = n.Name
		}
	}
	return nil
}

// Snapshot contains only this node's inbounds and the accounts serving them.
// No admin password, representative ledger, other location or signing key is sent.
func (s *NodeService) Snapshot(id int) (*nodewire.Snapshot, error) {
	db := database.GetDB()
	var n model.RemoteNode
	if e := db.First(&n, id).Error; e != nil {
		return nil, e
	}
	snap := &nodewire.Snapshot{Version: nodewire.Version, NodeID: id, Enable: n.Enable && !n.Paused, ControlGeneration: n.ControlGeneration, CoreActionGeneration: n.CoreActionGeneration, ConfigGeneration: n.ConfigGeneration, LeaseSeconds: nodewire.LeaseSeconds, CoreGeneration: n.CoreGeneration, Template: defaultNodeTemplate, Files: map[string][]byte{}}
	_ = json.Unmarshal([]byte(n.RequestedCores), &snap.RequestedCores)
	if n.CoreActionCommand != "" {
		_ = json.Unmarshal([]byte(n.CoreActionCommand), &snap.CoreActionCommand)
	}
	if n.ConfigCommand != "" {
		_ = json.Unmarshal([]byte(n.ConfigCommand), &snap.ConfigCommand)
	}
	if !n.Enable {
		return snap, nil
	}
	if e := db.Where("node_id = ?", id).Order("id").Find(&snap.Inbounds).Error; e != nil {
		return nil, e
	}
	ids := make([]int, 0, len(snap.Inbounds))
	for i := range snap.Inbounds {
		in := &snap.Inbounds[i]
		ids = append(ids, in.Id)
		in.ClientStats = nil
		in.NodeAddress = ""
		in.NodeName = ""
		if e := snapshotCertificateFiles(in, snap.Files); e != nil {
			return nil, e
		}
	}
	// Authorize retained meters when issuing their scoped assignment, rather
	// than waiting for the first report (the assignment can be removed meanwhile).
	if len(ids) > 0 {
		cursors := make([]model.NodeCounter, 0, len(ids))
		for _, inboundID := range ids {
			cursors = append(cursors, model.NodeCounter{NodeID: id, Kind: "inbound", InboundID: inboundID})
		}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(cursors, 100).Error; err != nil {
			return nil, err
		}
	}
	if len(ids) == 0 {
		return snap, nil
	}
	if e := db.Where("inbound_id IN ?", ids).Order("account_id,inbound_id").Find(&snap.Memberships).Error; e != nil {
		return nil, e
	}
	var accounts []model.Account
	if e := db.Where("id IN (?)", db.Model(&model.AccountInbound{}).Select("account_id").Where("inbound_id IN ?", ids)).Order("id").Find(&accounts).Error; e != nil {
		return nil, e
	}
	var stats []xray.ClientTraffic
	emails := make([]string, 0, len(accounts))
	for _, a := range accounts {
		emails = append(emails, a.Email)
	}
	if len(emails) > 0 {
		if e := db.Where("email IN ?", emails).Find(&stats).Error; e != nil {
			return nil, e
		}
	}
	byEmail := map[string]xray.ClientTraffic{}
	for _, t := range stats {
		byEmail[accountKey(t.Email)] = t
	}
	policyByAccount := map[int][]*model.Inbound{}
	byID := map[int]*model.Inbound{}
	for i := range snap.Inbounds {
		byID[snap.Inbounds[i].Id] = &snap.Inbounds[i]
	}
	for _, m := range snap.Memberships {
		policyByAccount[m.AccountId] = append(policyByAccount[m.AccountId], byID[m.InboundId])
	}
	var acknowledged []model.NodeCounter
	if e := db.Where("node_id = ? AND kind = ?", id, "account").Find(&acknowledged).Error; e != nil {
		return nil, e
	}
	reported := map[int]int64{}
	for _, c := range acknowledged {
		reported[c.AccountID] = c.Up + c.Down
	}
	now := time.Now().UnixMilli()
	for _, a := range accounts {
		t, exists := byEmail[accountKey(a.Email)]
		if exists && a.ExpiryTime < 0 && t.ExpiryTime > 0 {
			a.ExpiryTime = t.ExpiryTime
		}
		remaining := int64(-1)
		if a.TotalGB > 0 {
			remaining = a.TotalGB - t.Up - t.Down
			if remaining < 0 {
				remaining = 0
			}
			if policy := nodeBillingPolicy(policyByAccount[a.Id]); policy != nil {
				remaining = int64(float64(remaining) / policy.TrafficMultiplier)
			}
		}
		allowed := exists && a.Enable && t.Enable && !a.RepresentativeBlocked && (a.ExpiryTime <= 0 || a.ExpiryTime > now) && (remaining != 0)
		snap.Accounts = append(snap.Accounts, nodewire.Account{Account: a, ProtocolCredentials: a.ProtocolCredentials, Remaining: remaining, ReportedUsage: reported[a.Id], Allowed: allowed})
	}
	cursors := []model.NodeCounter{}
	for _, a := range snap.Accounts {
		cursors = append(cursors, model.NodeCounter{NodeID: id, Kind: "account", AccountID: a.Account.Id})
	}
	for _, m := range snap.Memberships {
		cursors = append(cursors, model.NodeCounter{NodeID: id, Kind: "membership", AccountID: m.AccountId, InboundID: m.InboundId})
	}
	if len(cursors) > 0 {
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(cursors, 100).Error; err != nil {
			return nil, err
		}
	}
	return snap, nil
}

// Only certificate fields may copy files; arbitrary paths embedded in unrelated
// settings never become a file-reading API for the agent.
func snapshotCertificateFiles(inbound *model.Inbound, files map[string][]byte) error {
	for _, p := range []*string{&inbound.Settings, &inbound.StreamSettings} {
		if *p == "" {
			continue
		}
		var obj any
		if e := json.Unmarshal([]byte(*p), &obj); e != nil {
			return e
		}
		var walk func(any) error
		walk = func(value any) error {
			switch v := value.(type) {
			case map[string]any:
				for k, item := range v {
					if k == "certificateFile" || k == "keyFile" || k == "caCertFile" || k == "serverCertFile" || k == "serverKeyFile" || k == "tlsCryptFile" {
						path, ok := item.(string)
						if !ok || path == "" {
							continue
						}
						data, e := os.ReadFile(path)
						if e != nil {
							return fmt.Errorf("Inbound #%d certificate file unavailable", inbound.Id)
						}
						if len(data) > 4<<20 {
							return errors.New("Certificate file too large")
						}
						hash := sha256.Sum256(data)
						name := hex.EncodeToString(hash[:])
						files[name] = data
						v[k] = "nodefile:" + name
					} else if e := walk(item); e != nil {
						return e
					}
				}
			case []any:
				for _, item := range v {
					if e := walk(item); e != nil {
						return e
					}
				}
			}
			return nil
		}
		if e := walk(obj); e != nil {
			return e
		}
		data, _ := json.Marshal(obj)
		*p = string(data)
	}
	return nil
}

func (s *NodeService) AcceptReport(id int, report nodewire.Report) error {
	if report.Version != nodewire.Version || report.Sequence <= 0 || len(report.Counters) > 100000 || len(report.Online) > 100000 || len(report.CoreReport) > 1<<20 || len(report.LastError) > 1024 {
		return errors.New("Invalid node report")
	}
	needRestart := false
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var n model.RemoteNode
		if e := tx.First(&n, id).Error; e != nil {
			return e
		}
		if report.Sequence <= n.LastSequence {
			return nil
		}
		if report.Sequence != n.LastSequence+1 {
			return &nodeReportConflict{code: "sequence_gap", message: "Unexpected report sequence; preserve the agent state"}
		}
		var inbounds []model.Inbound
		if e := tx.Where("node_id = ?", id).Find(&inbounds).Error; e != nil {
			return e
		}
		byInbound := map[int]*model.Inbound{}
		ids := []int{}
		for i := range inbounds {
			byInbound[inbounds[i].Id] = &inbounds[i]
			ids = append(ids, inbounds[i].Id)
		}
		var memberships []model.AccountInbound
		if len(ids) > 0 {
			if e := tx.Where("inbound_id IN ?", ids).Find(&memberships).Error; e != nil {
				return e
			}
		}
		allowed := map[int]bool{}
		pairs := map[string]bool{}
		membersByAccount := map[int][]*model.Inbound{}
		for _, m := range memberships {
			allowed[m.AccountId] = true
			pairs[fmt.Sprintf("%d:%d", m.AccountId, m.InboundId)] = true
			membersByAccount[m.AccountId] = append(membersByAccount[m.AccountId], byInbound[m.InboundId])
		}
		seen := map[string]bool{}
		now := time.Now().UnixMilli()
		for _, record := range report.Counters {
			if record.Up < 0 || record.Down < 0 || record.Up > math.MaxInt64/2048 || record.Down > math.MaxInt64/2048 {
				return errors.New("Invalid counter value")
			}
			if record.Kind != "account" && record.Kind != "membership" && record.Kind != "inbound" {
				return errors.New("Unknown counter kind")
			}
			if record.Kind == "account" && (record.AccountID <= 0 || record.InboundID != 0) || record.Kind == "membership" && (record.AccountID <= 0 || record.InboundID <= 0) || record.Kind == "inbound" && (record.AccountID != 0 || record.InboundID <= 0) {
				return errors.New("Invalid counter identity")
			}
			key := fmt.Sprintf("%s:%d:%d", record.Kind, record.AccountID, record.InboundID)
			if seen[key] {
				return errors.New("Duplicate counter")
			}
			seen[key] = true
			cursor := model.NodeCounter{NodeID: id, Kind: record.Kind, AccountID: record.AccountID, InboundID: record.InboundID}
			e := tx.Where("node_id = ? AND kind = ? AND account_id = ? AND inbound_id = ?", id, record.Kind, record.AccountID, record.InboundID).First(&cursor).Error
			known := e == nil
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			scoped := false
			switch record.Kind {
			case "account":
				scoped = allowed[record.AccountID]
			case "membership":
				scoped = pairs[fmt.Sprintf("%d:%d", record.AccountID, record.InboundID)]
			case "inbound":
				scoped = byInbound[record.InboundID] != nil
			}
			// Removed assignments may finish reporting their final retained counters;
			// only an already accepted identity can use that path.
			if !scoped && !known {
				// Legacy inactive tombstones can have no accepted cursor. A zero
				// record carries no bill: ignore it without authorizing its ID.
				// A later positive foreign counter must still be rejected.
				if record.Up == 0 && record.Down == 0 {
					continue
				}
				return &nodeReportConflict{code: "counter_out_of_scope", message: "Counter is outside this node's assignments"}
			}
			if record.Up < cursor.Up || record.Down < cursor.Down {
				return &nodeReportConflict{code: "counter_regressed", message: "Counter moved backwards; re-pair a reinstalled node"}
			}
			up, down := record.Up-cursor.Up, record.Down-cursor.Down
			if record.Kind == "account" && (up > 0 || down > 0) {
				var a model.Account
				e := tx.First(&a, record.AccountID).Error
				if e == nil {
					var t xray.ClientTraffic
					if e := tx.Where("email = ?", a.Email).First(&t).Error; e != nil {
						return errors.New("Missing central account traffic; repair accounting first")
					}
					if t.ExpiryTime < 0 {
						a.ExpiryTime = now - t.ExpiryTime
						t.ExpiryTime = a.ExpiryTime
						if e := tx.Model(&a).Update("expiry_time", a.ExpiryTime).Error; e != nil {
							return e
						}
						if e := tx.Model(&t).Update("expiry_time", t.ExpiryTime).Error; e != nil {
							return e
						}
						if _, e := (&AccountService{}).ProjectAccount(tx, a.Id); e != nil {
							return e
						}
						needRestart = true
					}
					policy := nodeBillingPolicy(membersByAccount[a.Id])
					up, down = multiplyDelta(policy, t.Up+t.Down, up, down)
					if e := tx.Model(&t).Updates(map[string]any{"up": gorm.Expr("up + ?", up), "down": gorm.Expr("down + ?", down), "all_time": gorm.Expr("all_time + ?", record.Up-cursor.Up+record.Down-cursor.Down), "last_online": now}).Error; e != nil {
						return e
					}
					if t.Total > 0 && t.Up+t.Down+up+down >= t.Total {
						needRestart = true
						if e := tx.Model(&t).Update("enable", false).Error; e != nil {
							return e
						}
					}
				} else if !errors.Is(e, gorm.ErrRecordNotFound) {
					return e
				}
			}
			if scoped && record.Kind == "membership" {
				if e := tx.Model(&model.AccountInbound{}).Where("account_id = ? AND inbound_id = ?", record.AccountID, record.InboundID).Updates(map[string]any{"up": gorm.Expr("up + ?", up), "down": gorm.Expr("down + ?", down), "all_time": gorm.Expr("all_time + ?", up+down)}).Error; e != nil {
					return e
				}
			}
			if scoped && record.Kind == "inbound" {
				if e := tx.Model(&model.Inbound{}).Where("id = ?", record.InboundID).Updates(map[string]any{"up": gorm.Expr("up + ?", up), "down": gorm.Expr("down + ?", down), "all_time": gorm.Expr("all_time + ?", up+down)}).Error; e != nil {
					return e
				}
			}
			cursor.Up = record.Up
			cursor.Down = record.Down
			if known {
				if e := tx.Model(&model.NodeCounter{}).Where("node_id = ? AND kind = ? AND account_id = ? AND inbound_id = ?", id, record.Kind, record.AccountID, record.InboundID).Updates(map[string]any{"up": cursor.Up, "down": cursor.Down}).Error; e != nil {
					return e
				}
			} else if e := tx.Create(&cursor).Error; e != nil {
				return e
			}
		}
		changed, e := reconcileRepresentatives(tx)
		if e != nil {
			return e
		}
		needRestart = needRestart || changed
		online := []nodewire.Online{}
		for _, o := range report.Online {
			if allowed[o.AccountID] && (o.InboundID == 0 || pairs[fmt.Sprintf("%d:%d", o.AccountID, o.InboundID)]) {
				online = append(online, o)
			}
		}
		raw, _ := json.Marshal(online)
		return tx.Model(&n).Updates(map[string]any{"last_sequence": report.Sequence, "last_seen": now, "online": string(raw), "core_report": string(report.CoreReport), "last_error": report.LastError}).Error
	})
	if err == nil && needRestart {
		(&XrayService{}).SetToNeedRestart()
	}
	return err
}
func nodeBillingPolicy(inbounds []*model.Inbound) *model.Inbound {
	var best *model.Inbound
	for _, i := range inbounds {
		if i != nil && i.Enable && i.TrafficMultiplierEnable && validMultiplier(i.TrafficMultiplier) && (best == nil || i.TrafficMultiplier > best.TrafficMultiplier) {
			best = i
		}
	}
	return best
}
func nodeOnline(db *gorm.DB) ([]string, []string) {
	if db == nil || !db.Migrator().HasTable(&model.RemoteNode{}) {
		return nil, nil
	}
	var nodes []model.RemoteNode
	if e := db.Where("enable = ? AND paused = ? AND last_seen > ?", true, false, time.Now().Add(-35*time.Second).UnixMilli()).Find(&nodes).Error; e != nil {
		return nil, nil
	}
	var entries []nodewire.Online
	var ids []int
	for _, n := range nodes {
		var online []nodewire.Online
		if json.Unmarshal([]byte(n.Online), &online) != nil {
			continue
		}
		entries = append(entries, online...)
		for _, o := range online {
			ids = append(ids, o.AccountID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var accounts []model.Account
	if db.Select("id", "email").Where("id IN ?", ids).Find(&accounts).Error != nil {
		return nil, nil
	}
	emails := map[int]string{}
	for _, a := range accounts {
		emails[a.Id] = a.Email
	}
	clients := []string{}
	members := []string{}
	for _, o := range entries {
		if email := emails[o.AccountID]; email != "" {
			clients = append(clients, email)
			members = append(members, onlineMembershipKey(o.InboundID, email))
		}
	}
	return clients, members
}

//go:embed config.json
var defaultNodeTemplate string

// Control keeps the management agent alive while changing VPN runtime state.
func (s *NodeService) Control(id int, action string) error {
	var n model.RemoteNode
	if err := database.GetDB().First(&n, id).Error; err != nil {
		return err
	}
	switch action {
	case "start", "restart", "reconnect":
		n.Paused = false
		n.Enable = true
	case "stop":
		n.Paused = true
	default:
		return errors.New("Unknown node action")
	}
	return database.GetDB().Model(&n).Updates(map[string]any{"enable": n.Enable, "paused": n.Paused, "control_generation": gorm.Expr("control_generation + 1")}).Error
}
func (s *NodeService) Configure(id int, cmd nodewire.ConfigCommand) error {
	var n model.RemoteNode
	if err := database.GetDB().First(&n, id).Error; err != nil {
		return err
	}
	if len(cmd.Text) > 512*1024 {
		return errors.New("Configuration exceeds 512 KiB")
	}
	valid := cmd.Core == "xray"
	for _, c := range (&CoreService{}).CoreCatalog() {
		if c.Name == cmd.Core {
			valid = true
		}
	}
	if !valid {
		return errors.New("Unknown core")
	}
	if !cmd.Inspect && (n.Paused || !n.Enable) {
		return errors.New("Start the node before applying configuration / ابتدا نود را روشن کنید")
	}
	if !cmd.Inspect && cmd.File == "inbound.json" {
		var in model.Inbound
		if err := database.GetDB().Where("id = ? AND node_id = ?", cmd.Inbound, id).First(&in).Error; err != nil {
			return err
		}
		if NodeCoreForProtocol(in.Protocol) != cmd.Core {
			return errors.New("Inbound does not belong to this core")
		}
		if cmd.Reset {
			return errors.New("Edit inbound settings directly; no replacement file to reset")
		}
		var body nodeInboundConfiguration
		dec := json.NewDecoder(strings.NewReader(cmd.Text))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			return err
		}
		var settings, original map[string]any
		if err := json.Unmarshal(body.Settings, &settings); err != nil || settings == nil {
			return errors.New("Settings must be a JSON object")
		}
		_ = json.Unmarshal([]byte(in.Settings), &original)
		// Credentials and account limits remain centrally managed rather than copied
		// from a node's derived, quota-adjusted client projection.
		delete(settings, "clients")
		if clients, ok := original["clients"]; ok {
			settings["clients"] = clients
		}
		raw, _ := json.Marshal(settings)
		in.Settings = string(raw)
		if len(body.StreamSettings) > 0 {
			var stream map[string]any
			if err := json.Unmarshal(body.StreamSettings, &stream); err != nil || stream == nil {
				return errors.New("streamSettings must be an object")
			}
			in.StreamSettings = string(body.StreamSettings)
		}
		in.Listen = body.Listen
		in.Port = body.Port
		if _, _, err := (&InboundService{}).UpdateInbound(&in); err != nil {
			return err
		}
		cmd.Inspect = true
		cmd.Text = ""
	}

	if !cmd.Inspect && !cmd.Reset && cmd.Core == "xray" {
		if err := ValidateNodeTemplate(cmd.Text); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	return database.GetDB().Model(&n).Updates(map[string]any{"config_command": string(raw), "config_generation": gorm.Expr("config_generation + 1")}).Error
}

func DefaultNodeTemplate() string { return defaultNodeTemplate }

// Node templates must retain the API and counters used for centralized quotas.
func ValidateNodeTemplate(text string) error {
	var cfg, base map[string]any
	if err := json.Unmarshal([]byte(text), &cfg); err != nil {
		return err
	}
	_ = json.Unmarshal([]byte(defaultNodeTemplate), &base)
	equal := func(a, b any) bool { x, _ := json.Marshal(a); y, _ := json.Marshal(b); return string(x) == string(y) }
	if !equal(cfg["api"], base["api"]) || !equal(cfg["stats"], base["stats"]) {
		return errors.New("Preserve the node API and statistics configuration / تنظیمات API و آمار نود باید حفظ شوند")
	}
	var api bool
	inbounds, _ := cfg["inbounds"].([]any)
	for _, entry := range inbounds {
		m, _ := entry.(map[string]any)
		if m["tag"] == "api" {
			api = equal(entry, base["inbounds"].([]any)[0])
		}
	}
	policy, _ := cfg["policy"].(map[string]any)
	levels, _ := policy["levels"].(map[string]any)
	level, _ := levels["0"].(map[string]any)
	system, _ := policy["system"].(map[string]any)
	routing, _ := cfg["routing"].(map[string]any)
	rules, _ := routing["rules"].([]any)
	baseRouting := base["routing"].(map[string]any)
	var apiRoute bool
	for _, rule := range rules {
		if equal(rule, baseRouting["rules"].([]any)[0]) {
			apiRoute = true
		}
	}
	if !api || !apiRoute || level["statsUserUplink"] != true || level["statsUserDownlink"] != true || system["statsInboundUplink"] != true || system["statsInboundDownlink"] != true {
		return errors.New("Preserve API inbound/routing and traffic counters / ورودی مدیریتی و شمارنده‌های مصرف باید حفظ شوند")
	}
	return (&XraySettingService{}).CheckXrayConfig(text)
}

func (s *NodeService) CoreAction(id int, cmd nodewire.CoreAction) error {
	var n model.RemoteNode
	if err := database.GetDB().First(&n, id).Error; err != nil {
		return err
	}
	switch cmd.Action {
	case "start", "stop", "restart", "uninstall":
	default:
		return errors.New("Unknown core action")
	}
	valid := false
	for _, c := range (&CoreService{}).CoreCatalog() {
		if c.Name == cmd.Core {
			valid = true
		}
	}
	if !valid {
		return errors.New("Unknown core")
	}
	if (cmd.Action == "start" || cmd.Action == "restart") && (n.Paused || !n.Enable) {
		return errors.New("Start the node first / ابتدا نود را روشن کنید")
	}
	raw, _ := json.Marshal(cmd)
	return database.GetDB().Model(&n).Updates(map[string]any{"core_action_command": string(raw), "core_action_generation": gorm.Expr("core_action_generation + 1")}).Error
}

type nodeInboundConfiguration struct {
	Listen         string          `json:"listen"`
	Port           int             `json:"port"`
	Settings       json.RawMessage `json:"settings"`
	StreamSettings json.RawMessage `json:"streamSettings"`
}

func NodeInboundConfigurationTargets(id int, core string) []map[string]any {
	var inbounds []model.Inbound
	database.GetDB().Where("node_id = ?", id).Order("id").Find(&inbounds)
	out := []map[string]any{}
	for _, in := range inbounds {
		if NodeCoreForProtocol(in.Protocol) != core {
			continue
		}
		var settings map[string]any
		_ = json.Unmarshal([]byte(in.Settings), &settings)
		delete(settings, "clients")
		raw, _ := json.Marshal(settings)
		stream := json.RawMessage(in.StreamSettings)
		if !json.Valid(stream) {
			stream = json.RawMessage(`{}`)
		}
		current, _ := json.MarshalIndent(nodeInboundConfiguration{Listen: in.Listen, Port: in.Port, Settings: raw, StreamSettings: stream}, "", "  ")
		out = append(out, map[string]any{"core": core, "inbound": in.Id, "file": "inbound.json", "label": fmt.Sprintf("inbound-%d.json", in.Id), "remark": in.Remark, "current": string(current), "central": true})
	}
	return out
}
