package service

import (
	"errors"
	"fmt"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
)

const adminGroupsMigrationKey = "administratorGroupsMigrationV3"
const groupManagementPermissions = model.PermAccessGroups | model.PermCreateGroup | model.PermEditGroup | model.PermDeleteGroup

func AdminGroupsMigrated(tx *gorm.DB) bool {
	var marker model.Setting
	return tx.Where("key = ?", adminGroupsMigrationKey).First(&marker).Error == nil && marker.Value == "1"
}
func adminGroupIDs(tx *gorm.DB, id int) ([]int, error) {
	ids := []int{}
	err := tx.Model(&model.AdminGroupAccess{}).Where("user_id = ?", id).Order("group_id").Pluck("group_id", &ids).Error
	return ids, err
}
func setAdminGroups(tx *gorm.DB, id int, groups []int) error {
	groups = uniqueGroupIDs(groups)
	for _, gid := range groups {
		if gid <= 0 {
			return errors.New("invalid group")
		}
	}
	var n int64
	if len(groups) > 0 {
		if err := tx.Model(&model.ClientGroup{}).Where("id IN ?", groups).Count(&n).Error; err != nil {
			return err
		}
		if int(n) != len(groups) {
			return errors.New("group not found")
		}
	}
	if err := tx.Where("user_id = ?", id).Delete(&model.AdminGroupAccess{}).Error; err != nil {
		return err
	}
	for _, gid := range groups {
		if err := tx.Create(&model.AdminGroupAccess{UserID: id, GroupID: gid}).Error; err != nil {
			return err
		}
	}
	return tx.Model(&model.User{}).Where("id = ?", id).Update("group_access", true).Error
}
func writeAdminAccess(tx *gorm.DB, id int, spec AdminSpec, creating bool) error {
	if !AdminGroupsMigrated(tx) {
		if spec.GroupIDs != nil {
			return errors.New("administrator groups migration must complete first")
		}
		ids := spec.InboundIds
		if spec.IsSuperAdmin {
			ids = nil
		}
		return setAdminInboundAccess(tx, id, ids)
	}
	if len(spec.InboundIds) > 0 {
		return errors.New("assign groups to administrators instead of individual inbounds")
	}
	if spec.GroupIDs == nil && !creating {
		return nil
	}
	ids := []int{}
	if spec.GroupIDs != nil && !spec.IsSuperAdmin {
		ids = *spec.GroupIDs
	}
	return setAdminGroups(tx, id, ids)
}

// The protected owner can manage administrators even after self-demotion. The
// administrator-grant checklist follows that authority, not their operational scope.
func (s *AdminService) AssignableGroups(actor *model.User) ([]GroupView, error) {
	if !actor.CanManageAdmins() {
		return nil, ErrAdminNotFound
	}
	var groups []model.ClientGroup
	tx := database.GetDB()
	if err := tx.Order("id").Find(&groups).Error; err != nil {
		return nil, err
	}
	out := []GroupView{}
	for _, g := range groups {
		ids, err := groupInboundIDs(tx, []int{g.ID})
		if err != nil {
			return nil, err
		}
		out = append(out, GroupView{ClientGroup: g, InboundIDs: ids})
	}
	return out, nil
}

// singletonGroup preserves an exact legacy grant without widening it to another
// inbound in a larger group. It never rewrites a customer or an existing group.
func singletonGroup(tx *gorm.DB, inboundID int) (int, error) {
	var g model.ClientGroup
	err := tx.Where("pasar_guard_id IS NULL AND disabled = 0 AND id IN (SELECT group_id FROM group_inbounds GROUP BY group_id HAVING COUNT(*)=1 AND MIN(inbound_id)=?)", inboundID).Order("id").First(&g).Error
	if err == nil {
		return g.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	var ib model.Inbound
	if err := tx.First(&ib, inboundID).Error; err != nil {
		return 0, err
	}
	name := ib.Remark
	if name == "" {
		name = ib.Tag
	}
	g = model.ClientGroup{Name: fmt.Sprintf("%s (#%d)", name, inboundID)}
	if err := tx.Create(&g).Error; err != nil {
		return 0, err
	}
	if err := tx.Create(&model.GroupInbound{GroupID: g.ID, InboundID: inboundID}).Error; err != nil {
		return 0, err
	}
	return g.ID, nil
}
func (s *GroupService) migrateAdminGroups() error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if AdminGroupsMigrated(tx) {
			return nil
		}
		// Existing editInbound holders could already perform every group operation.
		// Preserve that capability once, then all future edits use independent bits.
		if err := tx.Model(&model.AdminRole{}).Where("(permissions & ?) != 0", model.PermEditInbound).Update("permissions", gorm.Expr("permissions | ?", groupManagementPermissions)).Error; err != nil {
			return err
		}
		var users []model.User
		if err := tx.Order("id").Find(&users).Error; err != nil {
			return err
		}
		for _, u := range users {
			if u.IsRepresentativeAccount() {
				continue
			}
			if u.Permissions.Has(model.PermEditInbound) {
				if err := tx.Model(&u).Update("permissions", u.Permissions|groupManagementPermissions).Error; err != nil {
					return err
				}
			}
			if u.GroupAccess {
				continue
			}
			old := []int{}
			if err := tx.Table("inbound_accesses ia").Joins("JOIN inbounds ib ON ib.id=ia.inbound_id").Where("ia.user_id = ?", u.Id).Order("ia.inbound_id").Pluck("ia.inbound_id", &old).Error; err != nil {
				return err
			}
			scope := map[int]bool{}
			for _, id := range old {
				scope[id] = true
			}
			var groups []model.ClientGroup
			if err := tx.Order("id").Find(&groups).Error; err != nil {
				return err
			}
			grants := []int{}
			covered := map[int]bool{}
			for _, g := range groups {
				ids, err := groupInboundIDs(tx, []int{g.ID})
				if err != nil {
					return err
				}
				fits := len(ids) > 0
				for _, id := range ids {
					fits = fits && scope[id]
				}
				if fits {
					grants = append(grants, g.ID)
					for _, id := range ids {
						covered[id] = true
					}
				}
			}
			for _, id := range old {
				if !covered[id] {
					gid, err := singletonGroup(tx, id)
					if err != nil {
						return err
					}
					grants = append(grants, gid)
				}
			}
			if err := setAdminGroups(tx, u.Id, grants); err != nil {
				return err
			}
		}
		return tx.Create(&model.Setting{Key: adminGroupsMigrationKey, Value: "1"}).Error
	})
}
