package controller

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/web/session"
	"strconv"
	"strings"
)

func registerGroups(g *gin.RouterGroup) {
	groups := g.Group("/groups", requireNotRepresentative(), requirePerm(model.PermAccessGroups))
	groups.GET("", func(c *gin.Context) { html(c, "groups.html", "pages.groups.title", nil) })
	groups.GET("/list", func(c *gin.Context) {
		rows, err := (&service.GroupService{}).List(session.GetLoginUser(c), true)
		jsonObj(c, rows, err)
	})
	groups.GET("/inbounds", func(c *gin.Context) {
		rows, err := (&service.AccountService{}).AssignableInboundsFor(session.GetLoginUser(c))
		jsonObj(c, rows, err)
	})
	groups.POST("/save", func(c *gin.Context) {
		var form struct {
			ID         int    `form:"id"`
			Name       string `form:"name"`
			InboundIDs []int  `form:"inboundIds"`
			Disabled   *bool  `form:"disabled"`
		}
		if err := c.ShouldBind(&form); err != nil {
			jsonMsg(c, "", err)
			return
		}
		disabled := []bool{}
		if form.Disabled != nil {
			disabled = append(disabled, *form.Disabled)
		}
		row, touched, err := (&service.GroupService{}).Save(session.GetLoginUser(c), form.ID, form.Name, form.InboundIDs, disabled...)
		if err == nil {
			(&InboundController{}).reconcileForInbounds(touched, true)
		}
		jsonObj(c, row, err)
	})
	groups.POST("/delete/:id", func(c *gin.Context) {
		id, _ := strconv.Atoi(c.Param("id"))
		err := (&service.GroupService{}).Delete(session.GetLoginUser(c), id)
		jsonMsg(c, "", err)
	})
}
func postedGroupIDs(c *gin.Context) ([]int, bool, error) {
	raw, ok := c.GetPostFormArray("groupIds")
	if !ok {
		return nil, false, nil
	}
	ids := []int{}
	seen := map[int]bool{}
	for _, value := range raw {
		if value == "" {
			continue
		}
		id, err := strconv.Atoi(value)
		if err != nil || id <= 0 {
			return nil, true, fmt.Errorf("invalid group")
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids, true, nil
}
func (a *InboundController) clientMembershipSelection(c *gin.Context, target int, creating bool) ([]int, bool, error) {
	groups, present, err := postedGroupIDs(c)
	if err != nil {
		return nil, true, err
	}
	if present {
		ids, err := (&service.GroupService{}).Resolve(session.GetLoginUser(c), groups)
		if err != nil {
			return nil, true, err
		}
		if creating && target > 0 {
			found := false
			for _, id := range ids {
				if id == target {
					found = true
				}
			}
			if !found {
				return nil, true, fmt.Errorf("target inbound must belong to the selected groups")
			}
		}
		return ids, true, nil
	}
	ids, explicit := postedMembershipIds(c, target)
	if service.GroupsMigrated(database.GetDB()) && (creating || explicit) {
		return nil, true, fmt.Errorf("select groups for this account; add new inbounds to a group first")
	}
	return ids, explicit, nil
}

func (a *InboundController) bulkGroupMembership(c *gin.Context, req *service.BulkClientUpdateRequest, adding bool) {
	if len(req.GroupIDs) == 0 {
		jsonMsg(c, "", fmt.Errorf("select groups"))
		return
	}
	gs := &service.GroupService{}
	if _, err := gs.Resolve(session.GetLoginUser(c), req.GroupIDs); err != nil {
		jsonMsg(c, "", err)
		return
	}
	if !a.callerOwnsInbounds(c, distinctInboundIds(req.Targets)) {
		jsonMsg(c, "", errNotOwned)
		return
	}
	result := bulkMembershipResult{}
	touched := []int{}
	for _, email := range distinctTargetEmails(req.Targets) {
		account, err := accountService.GetAccountByEmailTx(database.GetDB(), email)
		if err != nil || account == nil {
			result.Skipped++
			result.Reasons = append(result.Reasons, "account not found")
			continue
		}
		current := []int{}
		if err := database.GetDB().Model(&model.AccountGroup{}).Where("account_id = ?", account.Id).Pluck("group_id", &current).Error; err != nil {
			jsonMsg(c, "", err)
			return
		}
		wanted := subtractInboundIds(current, req.GroupIDs)
		if adding {
			wanted = unionInboundIds(current, req.GroupIDs)
		}
		// Resolve only the visible part; Apply retains associations outside the editor's scope.
		choices, err := gs.List(session.GetLoginUser(c), false)
		if err != nil {
			jsonMsg(c, "", err)
			return
		}
		visible := map[int]bool{}
		for _, g := range choices {
			visible[g.ID] = true
		}
		filtered := []int{}
		for _, id := range wanted {
			if visible[id] {
				filtered = append(filtered, id)
			}
		}
		changed, err := gs.Apply(session.GetLoginUser(c), email, 0, filtered)
		if err != nil {
			result.Skipped++
			result.Reasons = append(result.Reasons, fmt.Sprintf("%s: %v", email, err))
			continue
		}
		result.Applied++
		touched = unionInboundIds(touched, changed)
	}
	a.reconcileForInbounds(touched, true)
	jsonObj(c, result, nil)
}

// Inbound editors configure listeners. After migration, provisioning belongs to
// the group-aware client endpoints; embedded client lists cannot bypass them.
func validateInboundGroupRoster(previous *model.Inbound, next *model.Inbound) error {
	if !service.GroupsMigrated(database.GetDB()) {
		return nil
	}
	read := func(settings string) (map[string]bool, error) {
		var object map[string]json.RawMessage
		if err := json.Unmarshal([]byte(settings), &object); err != nil {
			return nil, err
		}
		var clients []struct {
			Email string `json:"email"`
		}
		if raw := object["clients"]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &clients); err != nil {
				return nil, err
			}
		}
		result := map[string]bool{}
		for _, client := range clients {
			result[strings.ToLower(strings.TrimSpace(client.Email))] = true
		}
		return result, nil
	}
	fresh, err := read(next.Settings)
	if err != nil {
		return err
	}
	old := map[string]bool{}
	if previous != nil {
		old, err = read(previous.Settings)
		if err != nil {
			return err
		}
	}
	if len(fresh) != len(old) {
		return fmt.Errorf("manage users through groups on the Clients page")
	}
	for key := range fresh {
		if !old[key] {
			return fmt.Errorf("manage users through groups on the Clients page")
		}
	}
	return nil
}
