package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/web/session"
)

func (a *InboundController) renewClient(c *gin.Context) {
	var body struct {
		Duration *int64 `form:"renewalDuration" json:"renewalDuration"`
	}
	if err := c.ShouldBind(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	ids, err := accountService.InboundIdsForEmail(c.Param("email"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	err = accountService.RenewAccount(session.GetLoginUser(c), c.Param("email"), body.Duration)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	a.reconcileForInbounds(ids, len(ids) > 0)
	jsonMsg(c, I18nWeb(c, "pages.clients.renewSuccess"), nil)
}

func (a *InboundController) setClientAutoRenew(c *gin.Context) {
	var body struct {
		Enabled  *bool  `form:"enabled" json:"enabled" binding:"required"`
		Limit    *int   `form:"limit" json:"limit"`
		SetPlan  bool   `form:"setPlan" json:"setPlan"`
		Quota    *int64 `form:"quota" json:"quota"`
		Duration *int64 `form:"duration" json:"duration"`
	}
	if err := c.ShouldBind(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	var plan *service.AutoRenewPlan
	if body.SetPlan {
		plan = &service.AutoRenewPlan{Quota: body.Quota, Duration: body.Duration}
	}
	err := accountService.ConfigureAutoRenewPlan(c.Param("email"), *body.Enabled, body.Limit, plan, clientAuditOperation(c))
	jsonMsg(c, I18nWeb(c, "pages.clients.autoRenew"), err)
}
