package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
)

const groupsMigrationKey = "clientGroupsMigrationV1"

type GroupService struct{}
type GroupView struct {
	model.ClientGroup
	InboundIDs []int `json:"inboundIds"`
	Members    int64 `json:"members"`
}

func GroupsMigrated(tx *gorm.DB) bool {
	if tx == nil {
		return false
	}
	var marker model.Setting
	return tx.Where("key = ?", groupsMigrationKey).First(&marker).Error == nil && marker.Value == "1"
}

// Migrate applies the versioned compatibility and imported-group migrations.
// Existing customer memberships and credentials are never rewritten.
// Each migration commits with its marker; later inbounds stay ungrouped.
func (s *GroupService) Migrate() error {
	if err := s.migrateV1(); err != nil {
		return err
	}
	return s.migrateImportedGroupsAndGrants()
}

func (s *GroupService) migrateV1() error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if GroupsMigrated(tx) {
			return nil
		}
		if !(&AccountService{}).AccountsMigrated() {
			return errors.New("accounts migration must complete before groups migration")
		}
		var inbounds []model.Inbound
		if err := tx.Order("id").Find(&inbounds).Error; err != nil {
			return err
		}
		byInbound := map[int]int{}
		byTag := map[string]int{}
		for _, ib := range inbounds {
			name := strings.TrimSpace(ib.Remark)
			if name == "" {
				name = ib.Tag
			}
			if name == "" {
				name = "Inbound"
			}
			group := model.ClientGroup{Name: fmt.Sprintf("%s (#%d)", name, ib.Id)}
			if err := tx.Create(&group).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.GroupInbound{GroupID: group.ID, InboundID: ib.Id}).Error; err != nil {
				return err
			}
			byInbound[ib.Id] = group.ID
			byTag[ib.Tag] = group.ID
		}
		var memberships []model.AccountInbound
		if err := tx.Find(&memberships).Error; err != nil {
			return err
		}
		for _, m := range memberships {
			if gid := byInbound[m.InboundId]; gid > 0 {
				if err := tx.Create(&model.AccountGroup{AccountID: m.AccountId, GroupID: gid}).Error; err != nil {
					return err
				}
			}
		}
		// Translate imported role IDs to the new per-inbound groups. Preserve every
		// other access-policy field and the distinction between null and an empty set.
		var resources []model.PasarGuardResource
		if err := tx.Where("kind IN ?", []string{"inbounds", "inbounds_groups_association"}).Find(&resources).Error; err != nil {
			return err
		}
		tags := map[string]string{}
		associations := map[int][]string{}
		for _, res := range resources {
			var row map[string]string
			if err := json.Unmarshal([]byte(res.Data), &row); err != nil {
				return err
			}
			if res.Kind == "inbounds" {
				tags[row["id"]] = row["tag"]
			} else {
				gid, _ := strconv.Atoi(row["group_id"])
				associations[gid] = append(associations[gid], row["inbound_id"])
			}
		}
		var roles []model.RepresentativeRole
		if err := tx.Find(&roles).Error; err != nil {
			return err
		}
		for _, role := range roles {
			var access map[string]json.RawMessage
			if role.Access == "" {
				role.Access = "{}"
			}
			if err := json.Unmarshal([]byte(role.Access), &access); err != nil {
				return err
			}
			var allowed *[]int
			if err := json.Unmarshal(access["allowed_group_ids"], &allowed); err != nil && len(access["allowed_group_ids"]) > 0 {
				return err
			}
			if allowed == nil {
				continue
			}
			translated := []int{}
			seen := map[int]bool{}
			for _, gid := range *allowed {
				for _, sid := range associations[gid] {
					if id := byTag[tags[sid]]; id > 0 && !seen[id] {
						translated = append(translated, id)
						seen[id] = true
					}
				}
			}
			if access == nil {
				access = map[string]json.RawMessage{}
			}
			encoded, _ := json.Marshal(translated)
			access["allowed_group_ids"] = encoded
			encoded, _ = json.Marshal(access)
			if err := tx.Model(&role).Update("access", string(encoded)).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.Setting{Key: groupsMigrationKey, Value: "1"}).Error
	})
}

func roleGroupIDs(tx *gorm.DB, user *model.User) (*[]int, error) {
	if user == nil {
		return nil, errors.New("not found")
	}
	if !user.IsRepresentativeAccount() || user.RepresentativeRoleID <= 0 {
		return nil, nil
	}
	var role model.RepresentativeRole
	if err := tx.First(&role, user.RepresentativeRoleID).Error; err != nil {
		return nil, err
	}
	var access struct {
		Allowed *[]int `json:"allowed_group_ids"`
	}
	if role.Access == "" {
		role.Access = "{}"
	}
	if err := json.Unmarshal([]byte(role.Access), &access); err != nil {
		return nil, err
	}
	return access.Allowed, nil
}
func (s *GroupService) List(user *model.User, management bool) ([]GroupView, error) {
	rows := []GroupView{}
	if user == nil || !user.Enable {
		return rows, errors.New("not found")
	}
	if management && (user.IsRepresentativeAccount() || !user.Can(model.PermEditInbound)) {
		return rows, errors.New("not allowed")
	}
	visible, err := (&AccountService{}).AssignableInboundsFor(user)
	if err != nil {
		return nil, err
	}
	scope := map[int]bool{}
	for _, ib := range visible {
		scope[ib.InboundId] = true
	}
	var groups []model.ClientGroup
	tx := database.GetDB()
	if err := tx.Order("id").Find(&groups).Error; err != nil {
		return nil, err
	}
	allowed, err := roleGroupIDs(tx, user)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		ids := []int{}
		if err := tx.Model(&model.GroupInbound{}).Where("group_id = ?", g.ID).Order("inbound_id").Pluck("inbound_id", &ids).Error; err != nil {
			return nil, err
		}
		ok := user.IsSuperAdmin || len(ids) > 0
		for _, id := range ids {
			if !scope[id] {
				ok = false
			}
		}
		if !management && allowed != nil {
			found := false
			for _, id := range *allowed {
				if id == g.ID {
					found = true
				}
			}
			ok = ok && found
		}
		if !management && g.Disabled {
			ok = false
		}
		if user.IsRepresentativeAccount() && user.GroupAccess {
			var n int64
			if err := tx.Model(&model.RepresentativeGroupAccess{}).Where("user_id = ? AND group_id = ?", user.Id, g.ID).Count(&n).Error; err != nil {
				return nil, err
			}
			ok = ok && n > 0
		}
		if !ok {
			continue
		}
		count := int64(0)
		// Representatives must not learn other sellers' customer counts.
		if management {
			if err := tx.Model(&model.AccountGroup{}).Where("group_id = ?", g.ID).Count(&count).Error; err != nil {
				return nil, err
			}
		}
		rows = append(rows, GroupView{ClientGroup: g, InboundIDs: ids, Members: count})
	}
	return rows, nil
}
func (s *GroupService) Resolve(user *model.User, groupIDs []int) ([]int, error) {
	groups, err := s.List(user, false)
	if err != nil {
		return nil, err
	}
	byID := map[int]GroupView{}
	for _, g := range groups {
		byID[g.ID] = g
	}
	seen := map[int]bool{}
	ids := []int{}
	for _, id := range groupIDs {
		g, ok := byID[id]
		if !ok {
			return nil, errors.New("group is outside your allowed groups or inbound access")
		}
		for _, ib := range g.InboundIDs {
			if !seen[ib] {
				ids = append(ids, ib)
				seen[ib] = true
			}
		}
	}
	sort.Ints(ids)
	return ids, nil
}
func groupUnion(tx *gorm.DB, accountID int) ([]int, error) {
	ids := []int{}
	err := tx.Model(&model.GroupInbound{}).Select("DISTINCT group_inbounds.inbound_id").Joins("JOIN account_groups ag ON ag.group_id = group_inbounds.group_id").Joins("JOIN client_groups cg ON cg.id = group_inbounds.group_id AND cg.disabled = 0").Where("ag.account_id = ?", accountID).Order("group_inbounds.inbound_id").Pluck("group_inbounds.inbound_id", &ids).Error
	return ids, err
}

// projectGroups preserves existing AccountInbound rows (including disabled flags,
// per-protocol credentials, slots and traffic). Only new memberships are admitted.
func projectGroups(tx *gorm.DB, accountID int) ([]int, error) {
	ids, err := groupUnion(tx, accountID)
	if err != nil {
		return nil, err
	}
	var account model.Account
	if err := tx.First(&account, accountID).Error; err != nil {
		return nil, err
	}
	var inbounds []*model.Inbound
	if len(ids) > 0 {
		if err := tx.Where("id IN ?", ids).Find(&inbounds).Error; err != nil {
			return nil, err
		}
	}
	if err := (&AccountService{}).ValidateMembershipSet(inbounds); err != nil {
		return nil, err
	}
	for _, ib := range inbounds {
		if err := admitAccountOnInbound(ib, account.Email); err != nil {
			return nil, err
		}
	}
	accounts := &AccountService{}
	if err := accounts.SetMemberships(tx, accountID, ids); err != nil {
		return nil, err
	}
	return accounts.ProjectAccount(tx, accountID)
}
func (s *GroupService) Save(user *model.User, id int, name string, inboundIDs []int, disabled ...bool) (*GroupView, []int, error) {
	if user == nil || user.IsRepresentativeAccount() || !user.Can(model.PermEditInbound) {
		return nil, nil, errors.New("not allowed")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return nil, nil, errors.New("group name is required (maximum 200 bytes)")
	}
	visible, err := (&AccountService{}).AssignableInboundsFor(user)
	if err != nil {
		return nil, nil, err
	}
	scope := map[int]bool{}
	for _, ib := range visible {
		scope[ib.InboundId] = true
	}
	for _, ib := range inboundIDs {
		if !scope[ib] {
			return nil, nil, errors.New("not found")
		}
	}
	if id > 0 {
		rows, err := s.List(user, true)
		if err != nil {
			return nil, nil, err
		}
		found := false
		for _, g := range rows {
			if g.ID == id {
				found = true
			}
		}
		if !found {
			return nil, nil, errors.New("not found")
		}
	}
	if len(inboundIDs) == 0 && !user.IsSuperAdmin {
		return nil, nil, errors.New("only a super administrator can create or empty an unserved group")
	}
	group := model.ClientGroup{ID: id, Name: name}
	touched := []int{}
	err = database.GetDB().Transaction(func(tx *gorm.DB) error {
		oldDisabled := false
		if id > 0 {
			if err := tx.First(&group, id).Error; err != nil {
				return err
			}
			oldDisabled = group.Disabled
			group.Name = name
		}
		if len(disabled) > 0 {
			group.Disabled = disabled[0]
		}
		if id == 0 {
			if err := tx.Create(&group).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&group).Updates(map[string]any{"name": name, "disabled": group.Disabled}).Error; err != nil {
				return err
			}
		}
		if id > 0 {
			previous := []int{}
			if err := tx.Model(&model.GroupInbound{}).Where("group_id = ?", group.ID).Pluck("inbound_id", &previous).Error; err != nil {
				return err
			}
			wanted := map[int]bool{}
			for _, ib := range inboundIDs {
				wanted[ib] = true
			}
			same := len(previous) == len(wanted)
			for _, ib := range previous {
				if !wanted[ib] {
					same = false
				}
			}
			if same && oldDisabled == group.Disabled {
				return nil
			} // Renaming does not rewrite customer settings.
		}
		if err := tx.Where("group_id = ?", group.ID).Delete(&model.GroupInbound{}).Error; err != nil {
			return err
		}
		seen := map[int]bool{}
		for _, ib := range inboundIDs {
			if seen[ib] {
				continue
			}
			seen[ib] = true
			if err := tx.Create(&model.GroupInbound{GroupID: group.ID, InboundID: ib}).Error; err != nil {
				return err
			}
		}
		ids := []int{}
		if err := tx.Model(&model.AccountGroup{}).Where("group_id = ?", group.ID).Order("account_id").Pluck("account_id", &ids).Error; err != nil {
			return err
		}
		// Project sequentially in the transaction, so capacity checks see earlier
		// members and a full pool rolls back the definition and every affected user.
		for _, aid := range ids {
			changed, err := projectGroups(tx, aid)
			if err != nil {
				return err
			}
			touched = unionGroupIDs(touched, changed)
		}
		return nil
	})
	return &GroupView{ClientGroup: group, InboundIDs: inboundIDs}, touched, err
}
func unionGroupIDs(a, b []int) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, ids := range [][]int{a, b} {
		for _, id := range ids {
			if !seen[id] {
				out = append(out, id)
				seen[id] = true
			}
		}
	}
	return out
}
func (s *GroupService) Delete(user *model.User, id int) error {
	groups, err := s.List(user, true)
	if err != nil {
		return err
	}
	found := false
	for _, g := range groups {
		if g.ID == id {
			found = true
		}
	}
	if !found {
		return errors.New("not found")
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.AccountGroup{}).Where("group_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("group has users; reassign them before deleting it")
		}
		if err := tx.Model(&model.RepresentativeGroupAccess{}).Where("group_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("group is assigned to representatives; reassign them before deleting it")
		}
		var roles []model.RepresentativeRole
		if err := tx.Find(&roles).Error; err != nil {
			return err
		}
		for _, r := range roles {
			var p struct {
				IDs []int `json:"allowed_group_ids"`
			}
			if r.Access == "" {
				r.Access = "{}"
			}
			if err := json.Unmarshal([]byte(r.Access), &p); err != nil {
				return err
			}
			for _, gid := range p.IDs {
				if gid == id {
					return errors.New("group is used by a representative role")
				}
			}
		}
		if err := tx.Where("group_id = ?", id).Delete(&model.GroupInbound{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.ClientGroup{}, id).Error
	})
}

// Apply writes account-to-group links and their inbound union in one transaction.
// Invisible existing groups are retained for delegated editors.
func (s *GroupService) Apply(user *model.User, email string, anchor int, groupIDs []int) ([]int, error) {
	if _, err := s.Resolve(user, groupIDs); err != nil {
		return nil, err
	}
	choices, err := s.List(user, false)
	if err != nil {
		return nil, err
	}
	editable := map[int]bool{}
	for _, g := range choices {
		editable[g.ID] = true
	}
	// The email is an account identifier, not proof of ownership. Check it before
	// touching the anchor or accepting any new group associations.
	if !user.IsSuperAdmin {
		if user.IsRepresentativeAccount() {
			owns, err := (&ResellerService{}).OwnsClientEmail(email, user.Id)
			if err != nil {
				return nil, err
			}
			if !owns {
				return nil, errors.New("not found")
			}
		} else {
			current, err := (&AccountService{}).InboundIdsForEmail(email)
			if err != nil {
				return nil, err
			}
			visible, err := (&AccountService{}).AssignableInboundsFor(user)
			if err != nil {
				return nil, err
			}
			hasAccess := false
			for _, id := range current {
				for _, ib := range visible {
					if ib.InboundId == id {
						hasAccess = true
					}
				}
			}
			if !hasAccess && anchor > 0 {
				for _, ib := range visible {
					if ib.InboundId != anchor {
						continue
					}
					var inbound model.Inbound
					if err := database.GetDB().First(&inbound, anchor).Error; err != nil {
						return nil, err
					}
					clients, err := (&InboundService{}).GetClients(&inbound)
					if err != nil {
						return nil, err
					}
					for _, client := range clients {
						if sameEmail(client.Email, email) {
							hasAccess = true
						}
					}
				}
			}
			if !hasAccess {
				return nil, errors.New("not found")
			}
		}
	}
	touched := []int{}
	err = database.GetDB().Transaction(func(tx *gorm.DB) error {
		accounts := &AccountService{}
		if anchor > 0 {
			if err := accounts.SyncInboundAccounts(tx, anchor); err != nil {
				return err
			}
		}
		account, err := accounts.GetAccountByEmailTx(tx, email)
		if err != nil {
			return err
		}
		if account == nil {
			return errors.New("account not found")
		}
		var existing []model.AccountGroup
		if err := tx.Where("account_id = ?", account.Id).Find(&existing).Error; err != nil {
			return err
		}
		keep := map[int]bool{}
		for _, id := range groupIDs {
			keep[id] = true
		}
		for _, g := range existing {
			if !editable[g.GroupID] {
				keep[g.GroupID] = true
			}
		}
		if err := tx.Where("account_id = ?", account.Id).Delete(&model.AccountGroup{}).Error; err != nil {
			return err
		}
		for id := range keep {
			if err := tx.Create(&model.AccountGroup{AccountID: account.Id, GroupID: id}).Error; err != nil {
				return err
			}
		}
		touched, err = projectGroups(tx, account.Id)
		return err
	})
	return touched, err
}

// PreflightCreate checks the entire batch against every selected inbound before
// the anchor write or representative billing. In-memory slots account for all
// users in the request, including single-account IKEv2 and sparse IP pools.
func (s *GroupService) PreflightCreate(inboundIDs []int, emails []string) error {
	for _, id := range inboundIDs {
		var inbound model.Inbound
		if err := database.GetDB().First(&inbound, id).Error; err != nil {
			return err
		}
		var settings map[string]json.RawMessage
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			return err
		}
		for _, email := range emails {
			clients, err := (&InboundService{}).GetClients(&inbound)
			if err != nil {
				return err
			}
			if err := admitAccountOnInbound(&inbound, email); err != nil {
				return err
			}
			found := false
			for _, c := range clients {
				if sameEmail(c.Email, email) {
					found = true
				}
			}
			if found {
				continue
			}
			slot := slotsForNewAccounts(clients, 1)[0]
			clients = append(clients, model.Client{Email: email, Slot: &slot})
			raw, err := json.Marshal(clients)
			if err != nil {
				return err
			}
			settings["clients"] = raw
			raw, err = json.Marshal(settings)
			if err != nil {
				return err
			}
			inbound.Settings = string(raw)
		}
	}
	return nil
}
