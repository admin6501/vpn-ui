package service

import (
	"encoding/json"
	"errors"
	"sort"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
)

// effectiveInboundAccess is shared by page, object and broadcast authorization.
// Legacy grants remain for compatibility, but cannot authorize a representative
// once GroupAccess has been enabled. Native administrators use group grants,
// with a temporary creator grant for new inbounds awaiting their first group.
func effectiveInboundAccess(tx *gorm.DB) *gorm.DB {
	const representative = `u.is_panel_owner = 0 AND (u.is_reseller = 1 OR (u.representative_role_id > 0 AND u.admin_role_id = 0))`
	const grouped = `u.group_access = 1 AND u.is_super_admin = 0`
	sql := `SELECT ia.id, ia.user_id, ia.inbound_id FROM inbound_accesses ia JOIN users u ON u.id=ia.user_id WHERE NOT (` + grouped + `) UNION
 SELECT 0 AS id, rg.user_id, gi.inbound_id FROM representative_group_accesses rg JOIN users u ON u.id=rg.user_id JOIN group_inbounds gi ON gi.group_id=rg.group_id JOIN client_groups cg ON cg.id=rg.group_id WHERE cg.disabled=0 AND ` + grouped + ` AND ` + representative + ` UNION
 SELECT 0 AS id, ag.user_id, gi.inbound_id FROM admin_group_accesses ag JOIN users u ON u.id=ag.user_id JOIN group_inbounds gi ON gi.group_id=ag.group_id WHERE ` + grouped + ` AND NOT (` + representative + `) UNION
 SELECT 0 AS id, ia.user_id, ia.inbound_id FROM inbound_accesses ia JOIN users u ON u.id=ia.user_id JOIN inbounds ib ON ib.id=ia.inbound_id AND ib.user_id=u.id WHERE ia.pending_group=1 AND NOT EXISTS (SELECT 1 FROM group_inbounds gi WHERE gi.inbound_id=ia.inbound_id) AND ` + grouped + ` AND NOT (` + representative + `)`

	return tx.Table("(" + sql + ") AS inbound_accesses")
}

func representativeGroupIDs(tx *gorm.DB, id int) ([]int, error) {
	ids := []int{}
	err := tx.Model(&model.RepresentativeGroupAccess{}).Where("user_id = ?", id).Order("group_id").Pluck("group_id", &ids).Error
	return ids, err
}
func setRepresentativeGroups(tx *gorm.DB, id int, ids []int) error {
	if err := tx.Where("user_id = ?", id).Delete(&model.RepresentativeGroupAccess{}).Error; err != nil {
		return err
	}
	for _, gid := range uniqueGroupIDs(ids) {
		if err := tx.Create(&model.RepresentativeGroupAccess{UserID: id, GroupID: gid}).Error; err != nil {
			return err
		}
	}
	return tx.Model(&model.User{}).Where("id = ?", id).Update("group_access", true).Error
}
func uniqueGroupIDs(ids []int) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, id := range ids {
		if !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	sort.Ints(out)
	return out
}
func groupInboundIDs(tx *gorm.DB, ids []int) ([]int, error) {
	out := []int{}
	if len(ids) == 0 {
		return out, nil
	}
	err := tx.Model(&model.GroupInbound{}).Where("group_id IN ?", ids).Distinct().Order("inbound_id").Pluck("inbound_id", &out).Error
	return out, err
}
func enforceRoleGroupAccess(role *model.RepresentativeRole, ids []int) error {
	var access struct {
		Allowed         *[]int `json:"allowed_group_ids"`
		RequireTemplate bool   `json:"require_template"`
	}
	if role.Access == "" {
		role.Access = "{}"
	}
	if err := json.Unmarshal([]byte(role.Access), &access); err != nil {
		return errors.New("invalid role access policy")
	}
	if access.RequireTemplate {
		return errors.New("template-required roles need the PasarGuard template adapter")
	}
	if access.Allowed == nil {
		return nil
	}
	allowed := map[int]bool{}
	for _, id := range *access.Allowed {
		allowed[id] = true
	}
	for _, id := range ids {
		if !allowed[id] {
			return errors.New("group is outside the role's allowed groups")
		}
	}
	return nil
}

// validateGroupSpec uses nil to preserve grants for metadata-only requests. A
// supplied empty set revokes all groups. Historical out-of-policy grants may be
// retained; only new grants (or every grant on role change) require role approval.
func (s *ResellerService) validateGroupSpec(caller, existing *model.User, spec *ResellerSpec) error {
	if !GroupsMigrated(database.GetDB()) {
		if spec.GroupIDs != nil {
			return errors.New("groups migration must complete before assigning groups")
		}
		return nil
	}
	if len(spec.InboundIds) > 0 {
		return errors.New("assign groups to representatives instead of inbounds")
	}
	if spec.GroupIDs == nil {
		if existing == nil {
			empty := []int{}
			spec.GroupIDs = &empty
		} else if spec.RepresentativeRoleID > 0 && spec.RepresentativeRoleID != existing.RepresentativeRoleID {
			ids, err := representativeGroupIDs(database.GetDB(), existing.Id)
			if err != nil {
				return err
			}
			spec.GroupIDs = &ids
		} else {
			return nil
		}
	}
	ids := uniqueGroupIDs(*spec.GroupIDs)
	existingIDs := []int{}
	var err error
	if existing != nil {
		existingIDs, err = representativeGroupIDs(database.GetDB(), existing.Id)
		if err != nil {
			return err
		}
	}
	old := map[int]bool{}
	for _, id := range existingIDs {
		old[id] = true
	}
	added := []int{}
	for _, id := range ids {
		if id <= 0 {
			return errors.New("invalid group")
		}
		if !old[id] {
			added = append(added, id)
		}
	}
	choices, err := (&GroupService{}).List(caller, false)
	if err != nil {
		return err
	}
	allowed := map[int]bool{}
	for _, g := range choices {
		allowed[g.ID] = true
	}
	for _, id := range added {
		if !allowed[id] {
			return errors.New("group is outside your allowed groups")
		}
	}
	roleID := spec.RepresentativeRoleID
	if roleID == 0 && existing != nil {
		roleID = existing.RepresentativeRoleID
	}
	changed := existing != nil && roleID != existing.RepresentativeRoleID
	if roleID > 0 && (existing == nil || changed || len(added) > 0) {
		var role model.RepresentativeRole
		if err := database.GetDB().First(&role, roleID).Error; err != nil {
			return err
		}
		checked := added
		if existing == nil || changed {
			checked = ids
		}
		if err := enforceRoleGroupAccess(&role, checked); err != nil {
			return err
		}
	}
	var n int64
	if len(ids) > 0 {
		if err := database.GetDB().Model(&model.ClientGroup{}).Where("id IN ?", ids).Count(&n).Error; err != nil {
			return err
		}
		if int(n) != len(ids) {
			return errors.New("group not found")
		}
	}
	spec.GroupIDs = &ids
	return nil
}
