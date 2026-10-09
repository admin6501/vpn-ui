package controller

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"strconv"
)

func (a *ResellerController) roles(c *gin.Context) {
	var roles []model.RepresentativeRole
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
