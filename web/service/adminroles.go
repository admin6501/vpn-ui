package service

import (
	"errors"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
	"strings"
	"unicode/utf8"
)

var (
	ErrProtectedAdmin     = errors.New("only the panel owner may edit the protected owner account")
	ErrAdminRoleAssigned  = errors.New("assign these administrators to another role before deleting this role")
	ErrAdminRoleName      = errors.New("a role name of 1 to 100 characters is required")
	ErrAdminRoleDuplicate = errors.New("this administrator role name is already in use")
)

type AdminRoleView struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	Permissions   []string `json:"permissions"`
	AdminCount    int64    `json:"adminCount"`
	OwnerAssigned bool     `json:"ownerAssigned"`
}

func (s *AdminService) AdminRoles() ([]AdminRoleView, error) {
	var roles []model.AdminRole
	db := database.GetDB()
	if err := db.Order("name,id").Find(&roles).Error; err != nil {
		return nil, err
	}
	type count struct {
		AdminRoleID int
		N           int64
		Owners      int64
	}
	var counts []count
	if err := db.Model(&model.User{}).Select("admin_role_id, COUNT(*) AS n, SUM(CASE WHEN is_panel_owner = 1 THEN 1 ELSE 0 END) AS owners").Group("admin_role_id").Scan(&counts).Error; err != nil {
		return nil, err
	}
	byID := map[int]count{}
	for _, c := range counts {
		byID[c.AdminRoleID] = c
	}
	out := []AdminRoleView{}
	for _, r := range roles {
		c := byID[r.ID]
		out = append(out, AdminRoleView{ID: r.ID, Name: r.Name, Permissions: r.Permissions.Slugs(), AdminCount: c.N, OwnerAssigned: c.Owners > 0})
	}
	return out, nil
}
func (s *AdminService) SaveAdminRole(actor *model.User, id int, name string, permissions model.Permission) error {
	if !actor.CanManageAdmins() {
		return ErrAdminNotFound
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return ErrAdminRoleName
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if id > 0 {
			if err := guardOwnerRole(tx, actor.Id, id); err != nil {
				return err
			}
			var role model.AdminRole
			if err := tx.First(&role, id).Error; err != nil {
				return err
			}
		}
		var n int64
		if err := tx.Model(&model.AdminRole{}).Where("LOWER(TRIM(name)) = ? AND id != ?", strings.ToLower(name), id).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrAdminRoleDuplicate
		}
		// Keep only registered bits, and never permit roles to confer ownership or super-admin status.
		permissions = model.PermissionsFromSlugs(permissions.Slugs())
		if id == 0 {
			return tx.Create(&model.AdminRole{Name: name, Permissions: permissions}).Error
		}
		return tx.Model(&model.AdminRole{}).Where("id = ?", id).Updates(map[string]any{"name": name, "permissions": permissions}).Error
	})
}
func guardOwnerRole(tx *gorm.DB, actorID, roleID int) error {
	var n int64
	if err := tx.Model(&model.User{}).Where("admin_role_id = ? AND is_panel_owner = ? AND id != ?", roleID, true, actorID).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return ErrProtectedAdmin
	}
	return nil
}
func (s *AdminService) DeleteAdminRole(actor *model.User, id int) error {
	if !actor.CanManageAdmins() {
		return ErrAdminNotFound
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := guardOwnerRole(tx, actor.Id, id); err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&model.User{}).Where("admin_role_id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrAdminRoleAssigned
		}
		var role model.AdminRole
		if err := tx.First(&role, id).Error; err != nil {
			return err
		}
		return tx.Delete(&role).Error
	})
}
func resolveAdminRole(tx *gorm.DB, spec AdminSpec, current int) (int, error) {
	if spec.IsSuperAdmin {
		return 0, nil
	}
	id := current
	if spec.AdminRoleID != nil {
		id = *spec.AdminRoleID
	}
	if id < 0 {
		return 0, ErrAdminNotFound
	}
	if id > 0 {
		var role model.AdminRole
		if err := tx.First(&role, id).Error; err != nil {
			return 0, err
		}
	}
	return id, nil
}
func (s *AdminService) GuardAdminTarget(actor *model.User, id int) error {
	if !actor.CanManageAdmins() {
		return ErrAdminNotFound
	}
	var target model.User
	if err := database.GetDB().First(&target, id).Error; err != nil {
		return err
	}
	if target.IsPanelOwner && actor.Id != target.Id {
		return ErrProtectedAdmin
	}
	return nil
}
