package controller

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
	"strconv"
	"strings"
)

func (a *ResellerController) roles(c *gin.Context) {
	roles := make([]model.RepresentativeRole, 0)
	err := database.GetDB().Where("is_owner = ?", false).Order("id").Find(&roles).Error
	jsonObj(c, roles, err)
}

type representativeRoleForm struct {
	Name                        string `form:"name"`
	Permissions                 string `form:"permissions"`
	Limits                      string `form:"limits"`
	Features                    string `form:"features"`
	Access                      string `form:"access"`
	DisabledWhenLimited         bool   `form:"disabledWhenLimited"`
	DisconnectUsersWhenLimited  bool   `form:"disconnectUsersWhenLimited"`
	DisconnectUsersWhenDisabled bool   `form:"disconnectUsersWhenDisabled"`
}

func (a *ResellerController) saveRole(c *gin.Context) {
	var f representativeRoleForm
	if err := c.ShouldBind(&f); err != nil {
		jsonMsg(c, "Role", err)
		return
	}
	f.Name = strings.TrimSpace(f.Name)
	if f.Name == "" || len(f.Name) > 64 {
		jsonMsg(c, "Role", errors.New("invalid role name"))
		return
	}
	for _, value := range []string{f.Permissions, f.Limits, f.Features, f.Access} {
		var object map[string]json.RawMessage
		if json.Unmarshal([]byte(value), &object) != nil || object == nil {
			jsonMsg(c, "Role", errors.New("invalid role policy"))
			return
		}
	}
	role := model.RepresentativeRole{Name: f.Name, Permissions: f.Permissions, Limits: f.Limits, Features: f.Features, Access: f.Access,
		DisabledWhenLimited: f.DisabledWhenLimited, DisconnectUsersWhenLimited: f.DisconnectUsersWhenLimited, DisconnectUsersWhenDisabled: f.DisconnectUsersWhenDisabled}
	id, _ := strconv.Atoi(c.Param("id"))
	var err error
	if id > 0 {
		var existing model.RepresentativeRole
		if database.GetDB().First(&existing, id).Error != nil || existing.IsOwner {
			jsonMsg(c, "Role", errors.New("owner role cannot be changed"))
			return
		}
		role.ID = id
		err = database.GetDB().Save(&role).Error
	} else {
		err = database.GetDB().Create(&role).Error
	}
	jsonObj(c, role, err)
}

// Assigned roles cannot disappear: changing permissions implicitly by deleting a
// live role would strand representatives and their users.
func (a *ResellerController) deleteRole(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err == nil && id > 0 {
		err = database.GetDB().Transaction(func(tx *gorm.DB) error {
			var role model.RepresentativeRole
			if err := tx.First(&role, id).Error; err != nil {
				return err
			}
			if role.IsOwner {
				return errors.New("owner role cannot be deleted")
			}
			var count int64
			if err := tx.Model(&model.User{}).Where("representative_role_id = ?", id).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("role is assigned to representatives; change their role first")
			}
			return tx.Delete(&role).Error
		})
	} else {
		err = errors.New("invalid role id")
	}
	jsonMsg(c, "Role", err)
}
