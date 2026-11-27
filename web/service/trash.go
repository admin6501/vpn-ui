package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	TrashClient  = "client"
	TrashManager = "manager"
)

var ErrTrashConflict = errors.New("trash restore conflicts with current panel data")
var ErrTrashClientPlan = errors.New("missing or invalid restore quota or validity")

type TrashService struct{}

type TrashClientPlan struct {
	QuotaBytes   *int64 `json:"quotaBytes"`
	ValidityDays *int64 `json:"validityDays"`
}

func (p *TrashClientPlan) valid() bool {
	const maxQuotaBytes = 1<<53 - 1 // Exact in the panel's JSON client settings.
	return p != nil && p.QuotaBytes != nil && *p.QuotaBytes >= 0 && *p.QuotaBytes <= maxQuotaBytes &&
		p.ValidityDays != nil && *p.ValidityDays >= 0 && *p.ValidityDays <= maxRenewalDuration/bulkMsPerDay
}

// Preserve database columns, including secrets that the API models intentionally
// omit from JSON. Table and column identifiers below are always chosen by the server.
type trashRow map[string]any

type trashReference struct {
	ID       int
	Protocol model.Protocol
}

type trashClientSnapshot struct {
	Account     trashRow
	Memberships []trashRow
	Groups      []trashRow
	Traffic     []trashRow
	Ledger      []trashRow
	Payment     []trashRow
	Inbounds    []trashReference
}

type trashSnapshot struct {
	User                   trashRow
	InboundGrants          []trashRow
	AdminGroups            []trashRow
	RepresentativeGroups   []trashRow
	Profile                []trashRow
	ReadPositions          []trashRow
	Clients                []trashClientSnapshot
	OwnedInbounds          []trashReference
	ManagedRepresentatives []int
	HandedOverClients      []trashRow
}

func trashRows(tx *gorm.DB, table, where string, args ...any) ([]trashRow, error) {
	rows, err := tx.Table(table).Where(where, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var result []trashRow
	values, pointers := make([]any, len(columns)), make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		row := make(trashRow, len(columns))
		for i, column := range columns {
			value := values[i]
			if data, ok := value.([]byte); ok {
				value = string(data)
			}
			row[column] = value
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r trashRow) integer(key string) int64 {
	switch v := r[key].(type) {
	case int64:
		return v
	case json.Number:
		n, _ := v.Int64()
		return n
	}
	return 0
}

func (r trashRow) string(key string) string {
	s, _ := r[key].(string)
	return s
}

func insertTrashRows(tx *gorm.DB, table string, rows ...trashRow) error {
	for _, row := range rows {
		for column, value := range row {
			if number, ok := value.(json.Number); ok {
				n, err := number.Int64()
				if err != nil {
					return err
				}
				row[column] = n
			}
		}
		if err := tx.Session(&gorm.Session{Logger: gormlogger.Discard}).Table(table).Create(map[string]any(row)).Error; err != nil {
			return err
		}
	}
	return nil
}

func saveTrash(tx *gorm.DB, owner int, kind, name string, snapshot trashSnapshot) error {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return tx.Session(&gorm.Session{Logger: gormlogger.Discard}).Create(&model.PanelTrash{OwnerID: owner, Kind: kind, Name: name,
		ClientCount: len(snapshot.Clients), DeletedAt: time.Now().UnixMilli(), Payload: string(payload)}).Error
}

func trashClientOwner(tx *gorm.DB, id int) (int, error) {
	var owner int
	err := tx.Table("accounts a").Where("a.id = ?", id).Select(database.ActivityOwnerSQL).Scan(&owner).Error
	return owner, err
}

func snapshotTrashClient(tx *gorm.DB, id int) (trashClientSnapshot, error) {
	rows, err := trashRows(tx, "accounts", "id = ?", id)
	if err != nil {
		return trashClientSnapshot{}, err
	}
	if len(rows) != 1 {
		return trashClientSnapshot{}, gorm.ErrRecordNotFound
	}
	snapshot := trashClientSnapshot{Account: rows[0]}
	email := accountKey(snapshot.Account.string("email"))
	for _, part := range []struct {
		table, where string
		value        any
		dst          *[]trashRow
	}{
		{"account_inbounds", "account_id = ?", id, &snapshot.Memberships},
		{"account_groups", "account_id = ?", id, &snapshot.Groups},
		{"client_traffics", "LOWER(TRIM(email)) = ?", email, &snapshot.Traffic},
		{"reseller_clients", "LOWER(TRIM(email)) = ?", email, &snapshot.Ledger},
		{"panel_payment_reminders", "account_id = ?", id, &snapshot.Payment},
	} {
		*part.dst, err = trashRows(tx, part.table, part.where, part.value)
		if err != nil {
			return trashClientSnapshot{}, err
		}
	}
	for _, row := range snapshot.Memberships {
		inboundID := int(row.integer("inbound_id"))
		ref := trashReference{ID: inboundID}
		if err := tx.Model(&model.Inbound{}).Where("id = ?", inboundID).Select("protocol").Scan(&ref.Protocol).Error; err != nil {
			return trashClientSnapshot{}, err
		}
		snapshot.Inbounds = append(snapshot.Inbounds, ref)
	}
	return snapshot, nil
}

func (s *trashClientSnapshot) clearAllowance() error {
	for _, column := range []string{"total_gb", "expiry_time", "renewal_duration", "last_renewed_at", "imported_last_reset_at"} {
		s.Account[column] = int64(0)
	}
	s.Account["last_renewal_automatic"] = false
	s.Payment = nil
	for i, row := range s.Memberships {
		for _, column := range []string{"up", "down", "all_time"} {
			row[column] = int64(0)
		}
		if extra := row.string("extra"); extra != "" {
			var entry map[string]json.RawMessage
			if err := json.Unmarshal([]byte(extra), &entry); err != nil {
				return err
			}
			for _, key := range []string{"totalGB", "expiryTime", "renewalDuration", "lastRenewedAt", "importedLastResetAt", "up", "down", "allTime", "lastOnline"} {
				delete(entry, key)
			}
			rendered, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			s.Memberships[i]["extra"] = string(rendered)
		}
	}
	for _, row := range s.Traffic {
		for _, column := range []string{"total", "expiry_time", "up", "down", "all_time", "last_online"} {
			row[column] = int64(0)
		}
	}
	for _, row := range s.Ledger {
		row["charged_bytes"], row["all_time_base"] = int64(0), int64(0)
	}
	return nil
}

func archiveTrashClient(tx *gorm.DB, id int) error {
	owner, err := trashClientOwner(tx, id)
	if err != nil || owner == 0 {
		return err
	}
	client, err := snapshotTrashClient(tx, id)
	if err != nil {
		return err
	}
	if err := client.clearAllowance(); err != nil {
		return err
	}
	return saveTrash(tx, owner, TrashClient, client.Account.string("email"), trashSnapshot{Clients: []trashClientSnapshot{client}})
}

func archiveLastTrashClients(tx *gorm.DB, inboundID int, candidates []int) error {
	if len(candidates) == 0 {
		return nil
	}
	var ids []int
	if err := tx.Model(&model.Account{}).Where("id IN ?", candidates).
		Where("NOT EXISTS (SELECT 1 FROM account_inbounds WHERE account_id = accounts.id AND inbound_id <> ?)", inboundID).
		Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if err := archiveTrashClient(tx, id); err != nil {
			return err
		}
	}
	return nil
}

// Bulk operations can remove several memberships at once. Capture an account
// before dropping any of them, and only when the last live entry has been removed.
func (s *InboundService) finishTrashRemovals(tx *gorm.DB, removed map[string][]int) error {
	accounts := &AccountService{}
	for email, inboundIDs := range removed {
		remaining, err := s.inboundIdsServingEmails(tx, []string{email})
		if err != nil {
			return err
		}
		account, err := accounts.GetAccountByEmailTx(tx, email)
		if err != nil {
			return err
		}
		if account != nil {
			if len(remaining) == 0 {
				if err := archiveTrashClient(tx, account.Id); err != nil {
					return err
				}
				if _, err := removeTrashClient(tx, *account); err != nil {
					return err
				}
				continue
			}
			if err := tx.Where("account_id = ? AND inbound_id IN ?", account.Id, inboundIDs).
				Delete(&model.AccountInbound{}).Error; err != nil {
				return err
			}
		}
		if len(remaining) != 0 {
			if err := tx.Model(&xray.ClientTraffic{}).Where("LOWER(TRIM(email)) = ? AND inbound_id IN ?", email, inboundIDs).
				UpdateColumn("inbound_id", remaining[0]).Error; err != nil {
				return err
			}
			continue
		}
		if err := tx.Where("LOWER(TRIM(email)) = ?", email).Delete(&xray.ClientTraffic{}).Error; err != nil {
			return err
		}
		if err := tx.Where("LOWER(TRIM(client_email)) = ?", email).Delete(&model.InboundClientIps{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func removeTrashClient(tx *gorm.DB, account model.Account) ([]int, error) {
	if err := tx.Where("account_id = ?", account.Id).Delete(&model.AccountInbound{}).Error; err != nil {
		return nil, err
	}
	touched, err := (&AccountService{}).ProjectAccount(tx, account.Id)
	if err != nil {
		return nil, err
	}
	email := accountKey(account.Email)
	for _, part := range []struct {
		table, where string
		value        any
	}{
		{"account_groups", "account_id = ?", account.Id},
		{"client_traffics", "LOWER(TRIM(email)) = ?", email},
		{"inbound_client_ips", "LOWER(TRIM(client_email)) = ?", email},
	} {
		if err := tx.Table(part.table).Where(part.where, part.value).Delete(map[string]any{}).Error; err != nil {
			return nil, err
		}
	}
	return touched, tx.Delete(&account).Error
}

func settleTrashClient(tx *gorm.DB, email string, traffic []xray.ClientTraffic) error {
	key := accountKey(email)
	var owner model.ResellerClient
	err := tx.Where("LOWER(TRIM(email)) = ?", key).First(&owner).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var profile model.ResellerProfile
	if err := tx.Where("user_id = ?", owner.UserId).First(&profile).Error; err != nil {
		return err
	}
	if len(traffic) == 0 && profile.ConsumptionBased {
		return errors.New("cannot retain representative consumption without a traffic snapshot")
	}
	if len(traffic) != 0 {
		consumed := max(traffic[0].AllTime-owner.AllTimeBase, 0)
		if profile.ConsumptionBased {
			if err := tx.Model(&model.ResellerProfile{}).Where("user_id = ?", owner.UserId).
				Update("usage_base", gorm.Expr("usage_base + ?", consumed)).Error; err != nil {
				return err
			}
		} else if err := addSpent(tx, owner.UserId, -max(owner.ChargedBytes-consumed, 0)); err != nil {
			return err
		}
	}
	return tx.Delete(&owner).Error
}

func (TrashService) DeleteClient(user *model.User, email string) ([]int, error) {
	if user == nil || !user.Can(model.PermDeleteClient) {
		return nil, gorm.ErrRecordNotFound
	}
	var touched []int
	err := database.SnapshotTransaction(database.GetDB(), func(tx *gorm.DB) error {
		var account model.Account
		if err := tx.Where("LOWER(TRIM(email)) = ?", accountKey(email)).First(&account).Error; err != nil {
			return err
		}
		owner, err := trashClientOwner(tx, account.Id)
		if err != nil {
			return err
		}
		ids, err := servingInboundIds(tx, account.Email)
		if err != nil {
			return err
		}
		if user.IsReseller && owner != user.Id || len(ids) == 0 && !user.IsSuperAdmin && owner != user.Id {
			return gorm.ErrRecordNotFound
		}
		if err := checkTrashInboundAccess(tx, user, ids); err != nil {
			return err
		}
		if err := archiveTrashClient(tx, account.Id); err != nil {
			return err
		}
		var traffic []xray.ClientTraffic
		if err := tx.Select("all_time").Where("LOWER(TRIM(email)) = ?", accountKey(account.Email)).Limit(1).Find(&traffic).Error; err != nil {
			return err
		}
		touched, err = removeTrashClient(tx, account)
		if err != nil {
			return err
		}
		return settleTrashClient(tx, account.Email, traffic)
	})
	return touched, err
}

func checkTrashInboundAccess(tx *gorm.DB, user *model.User, ids []int) error {
	if len(ids) == 0 || user.IsSuperAdmin {
		return nil
	}
	var count int64
	if err := effectiveInboundAccess(tx).Where("user_id = ? AND inbound_id IN ?", user.Id, ids).Distinct("inbound_id").Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(ids) {
		return ErrTrashConflict
	}
	return nil
}

func trashScope(db *gorm.DB, user *model.User, kind string) *gorm.DB {
	q := db.Model(&model.PanelTrash{}).Where("kind = ?", kind)
	switch {
	case user != nil && kind == TrashManager && user.IsPanelOwner:
		return q
	case user != nil && kind == TrashClient:
		return q.Where("owner_id = ?", user.Id)
	default:
		return q.Where("1 = 0")
	}
}

func (TrashService) List(user *model.User, kind string, before int64) ([]model.PanelTrash, error) {
	rows := []model.PanelTrash{}
	q := trashScope(database.GetDB(), user, kind)
	if before > 0 {
		q = q.Where("id < ?", before)
	}
	err := q.Select("id, kind, name, client_count, deleted_at").Order("id DESC").Limit(ActivityPageSize + 1).Find(&rows).Error
	return rows, err
}

func (TrashService) Purge(user *model.User, kind string, id int64) error {
	q := trashScope(database.GetDB(), user, kind)
	if id > 0 {
		q = q.Where("id = ?", id)
	}
	return q.Delete(&model.PanelTrash{}).Error
}

func (TrashService) Restore(user *model.User, kind string, id int64, plan *TrashClientPlan) ([]int, error) {
	if user == nil || id <= 0 || (kind == TrashClient && !user.Can(model.PermCreateClient)) {
		return nil, gorm.ErrRecordNotFound
	}
	if kind == TrashClient && !plan.valid() {
		return nil, ErrTrashClientPlan
	}
	var touched []int
	err := database.SnapshotTransaction(database.GetDB(), func(tx *gorm.DB) error {
		touched = nil
		var row model.PanelTrash
		if err := trashScope(tx, user, kind).First(&row, id).Error; err != nil {
			return err
		}
		var snapshot trashSnapshot
		decoder := json.NewDecoder(strings.NewReader(row.Payload))
		decoder.UseNumber()
		if err := decoder.Decode(&snapshot); err != nil {
			return err
		}
		if kind == TrashManager {
			if err := restoreTrashManager(tx, user, &snapshot); err != nil {
				return err
			}
		}
		for _, client := range snapshot.Clients {
			if kind == TrashClient {
				ids := make([]int, 0, len(client.Inbounds))
				for _, ref := range client.Inbounds {
					ids = append(ids, ref.ID)
				}
				if err := checkTrashInboundAccess(tx, user, ids); err != nil {
					return err
				}
				if err := checkTrashGroupAccess(tx, user, client.Groups); err != nil {
					return err
				}
				if err := client.applyRestorePlan(tx, user, *plan); err != nil {
					return err
				}
			}
			ids, err := restoreTrashClient(tx, client)
			if err != nil {
				return err
			}
			touched = append(touched, ids...)
		}
		return tx.Delete(&row).Error
	})
	return touched, err
}

func (s *trashClientSnapshot) applyRestorePlan(tx *gorm.DB, user *model.User, plan TrashClientPlan) error {
	now := time.Now().UnixMilli()
	quota := *plan.QuotaBytes
	duration, expiry := *plan.ValidityDays*bulkMsPerDay, int64(0)
	if duration > 0 {
		expiry = now + duration
	}
	representative := user.IsRepresentativeAccount()
	if len(s.Ledger) > 1 || (len(s.Ledger) == 1 && (!representative || s.Ledger[0].integer("user_id") != int64(user.Id))) {
		return ErrTrashConflict
	}
	if representative {
		profile, err := (&ResellerService{}).profileForDB(tx, user.Id)
		if err != nil {
			return err
		}
		quote, err := Quote(QuoteInput{
			Profile: *profile, Create: true, RequestedExpiry: &expiry, NewTotal: quota, NowMillis: now,
		})
		if err != nil {
			return err
		}
		if err := enforceRepresentativeUserCount(tx, user.Id); err != nil {
			return err
		}
		if err := addSpent(tx, user.Id, quote.DeltaSpent); err != nil {
			return err
		}
		if quote.ForceExpiry {
			expiry, duration = quote.ExpiryTime, quote.ExpiryTime-now
		}
		if len(s.Ledger) == 0 {
			inboundID := 0
			if len(s.Inbounds) != 0 {
				inboundID = s.Inbounds[0].ID
			}
			s.Ledger = []trashRow{{"email": s.Account.string("email"), "inbound_id": inboundID, "user_id": user.Id}}
		}
		s.Ledger[0]["charged_bytes"], s.Ledger[0]["all_time_base"] = quote.NewCharged, int64(0)
	}
	s.Account["total_gb"], s.Account["expiry_time"], s.Account["renewal_duration"] = quota, expiry, duration
	for _, row := range s.Traffic {
		row["total"], row["expiry_time"] = quota, expiry
	}
	return nil
}

func checkTrashGroupAccess(tx *gorm.DB, user *model.User, groups []trashRow) error {
	if !user.GroupAccess || user.IsSuperAdmin || len(groups) == 0 {
		return nil
	}
	allowed, err := (&GroupService{}).list(tx, user, false)
	if err != nil {
		return err
	}
	for _, row := range groups {
		id := int(row.integer("group_id"))
		if !slices.ContainsFunc(allowed, func(group GroupView) bool { return group.ID == id }) {
			return ErrTrashConflict
		}
	}
	return nil
}

func trashRequireRows(tx *gorm.DB, table, condition string, args ...any) error {
	var count int64
	if err := tx.Table(table).Where(condition, args...).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrTrashConflict
	}
	return nil
}

func restoreTrashClient(tx *gorm.DB, c trashClientSnapshot) ([]int, error) {
	id, email := c.Account.integer("id"), c.Account.string("email")
	var n int64
	if err := tx.Table("accounts").Where("LOWER(TRIM(email)) = ? OR id = ? OR (sub_id <> '' AND sub_id = ?)", accountKey(email), id, c.Account.string("sub_id")).Count(&n).Error; err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrTrashConflict
	}
	serving, err := servingInboundIds(tx, email)
	if err != nil {
		return nil, err
	}
	if len(serving) > 0 {
		return nil, ErrTrashConflict
	}
	for _, row := range c.Groups {
		if err := trashRequireRows(tx, "client_groups", "id = ?", row.integer("group_id")); err != nil {
			return nil, err
		}
	}
	for _, ref := range c.Inbounds {
		if err := trashRequireRows(tx, "inbounds", "id = ? AND protocol = ?", ref.ID, ref.Protocol); err != nil {
			return nil, err
		}
	}
	if err := insertTrashRows(tx, "accounts", c.Account); err != nil {
		return nil, err
	}
	var account model.Account
	if err := tx.First(&account, id).Error; err != nil {
		return nil, err
	}
	if err := insertTrashRows(tx, "account_groups", c.Groups...); err != nil {
		return nil, err
	}
	if len(c.Groups) != 0 {
		current, err := groupUnion(tx, int(id))
		if err != nil {
			return nil, err
		}
		previous := make([]int, 0, len(c.Inbounds))
		for _, ref := range c.Inbounds {
			previous = append(previous, ref.ID)
		}
		slices.Sort(previous)
		if !slices.Equal(current, previous) {
			return nil, ErrTrashConflict
		}
	}
	// A pool slot or a protocol credential may have been issued to a new client
	// since the deletion. Never reassign a live client's address or credential.
	for _, row := range c.Memberships {
		var inbound model.Inbound
		if err := tx.First(&inbound, row.integer("inbound_id")).Error; err != nil {
			return nil, err
		}
		if err := admitAccountOnInbound(&inbound, account.Email); err != nil {
			return nil, ErrTrashConflict
		}
		clients, valid := parseSettingsClients(inbound.Settings)
		if !valid {
			return nil, ErrTrashConflict
		}
		member := model.AccountInbound{Extra: row.string("extra"), Flow: row.string("flow")}
		if row["slot"] != nil {
			slot := int(row.integer("slot"))
			if capacity, bounded := maxVpnAccounts(&inbound); bounded && (slot < 0 || slot >= capacity) {
				return nil, ErrTrashConflict
			}
			member.Slot = &slot
		}
		entry := renderClientEntry(&account, &member, &inbound, nil)
		key := clientIdentityKey(inbound.Protocol)
		credential, _ := entry[key].(string)
		for i, existing := range clients {
			current, _ := existing[key].(string)
			if (member.Slot != nil && slotPoolProtocol(inbound.Protocol) && entrySlotOr(existing, i) == *member.Slot) ||
				(credential != "" && current == credential) {
				return nil, ErrTrashConflict
			}
		}
	}
	for table, rows := range map[string][]trashRow{
		"account_inbounds": c.Memberships, "reseller_clients": c.Ledger,
		"client_traffics": c.Traffic, "panel_payment_reminders": c.Payment,
	} {
		if err := insertTrashRows(tx, table, rows...); err != nil {
			return nil, fmt.Errorf("restore %s: %w", table, err)
		}
	}
	if len(c.Traffic) == 0 {
		inboundID := 0
		if len(c.Inbounds) > 0 {
			inboundID = c.Inbounds[0].ID
		}
		if err := insertTrashRows(tx, "client_traffics", trashRow{"inbound_id": inboundID, "email": email,
			"enable": account.Enable, "total": account.TotalGB, "expiry_time": account.ExpiryTime, "reset": account.Reset}); err != nil {
			return nil, err
		}
	}
	return (&AccountService{}).ProjectAccount(tx, int(id))
}
