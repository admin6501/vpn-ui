package controller

import (
	"github.com/gin-gonic/gin"
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
