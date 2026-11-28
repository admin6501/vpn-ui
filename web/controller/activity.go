package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/web/session"
	"gorm.io/gorm"
)

var activityService service.ActivityService

func recordPanelLogin(c *gin.Context, user *model.User, kind string) {
	if err := activityService.RecordLogin(user, getRemoteIp(c), loginOS(c.GetHeader("Sec-CH-UA-Platform"), c.Request.UserAgent()), kind); err != nil {
		logger.Warning("Could not record panel login activity:", err)
	}
}

func loginOS(platform, agent string) string {
	switch strings.Trim(strings.TrimSpace(platform), `"`) {
	case "Android", "iOS", "macOS", "Windows", "Linux", "Chrome OS":
		return strings.Trim(strings.TrimSpace(platform), `"`)
	}
	for _, os := range []struct{ token, name string }{
		{"Android", "Android"}, {"iPhone", "iOS"}, {"iPad", "iOS"}, {"iPod", "iOS"}, {"CrOS", "Chrome OS"},
		{"Windows", "Windows"}, {"Mac OS", "macOS"}, {"Linux", "Linux"}, {"FreeBSD", "FreeBSD"},
	} {
		if strings.Contains(agent, os.token) {
			return os.name
		}
	}
	return ""
}

func activityPage(mode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data := gin.H{"activityMode": mode, "trashDaysPerGB": 0}
		if user := session.GetLoginUser(c); mode == "trash" && user.IsRepresentativeAccount() {
			if profile, err := (&service.ResellerService{}).ProfileFor(user.Id); err == nil && !profile.ConsumptionBased {
				data["trashDaysPerGB"] = profile.DaysPerGB
			}
		}
		html(c, "activity.html", "pages.activity."+mode, data)
	}
}

func activityRows[T any](c *gin.Context, rows []T, err error) {
	more := len(rows) > service.ActivityPageSize
	if more {
		rows = rows[:service.ActivityPageSize]
	}
	jsonObj(c, gin.H{"rows": rows, "more": more}, err)
}

func registerActivityRoutes(api *gin.RouterGroup) {
	g := api.Group("/activity")
	g.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	g.GET("/logins", func(c *gin.Context) {
		user := session.GetLoginUser(c)
		rows, err := activityService.LoginEvents(user, c.Query("scope") == "own", activityBefore(c))
		more := len(rows) > service.ActivityPageSize
		if more {
			rows = rows[:service.ActivityPageSize]
		}
		result := gin.H{"rows": rows, "more": more}
		if err == nil && user.IsPanelOwner {
			admins, representatives, loadErr := activityService.LoginLogging()
			result["logAdmins"], result["logRepresentatives"] = admins, representatives
			err = loadErr
		}
		jsonObj(c, result, err)
	})
	g.POST("/logins/settings", requirePanelOwner(), func(c *gin.Context) {
		var input struct {
			Representative bool
			Enabled        *bool
		}
		if !bindActivity(c, &input) {
			return
		}
		if input.Enabled == nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("missing login logging preference"))
			return
		}
		jsonMsg(c, "", activityService.SetLoginLogging(input.Representative, *input.Enabled))
	})
	g.POST("/logins/purge", requirePanelOwner(), func(c *gin.Context) {
		var input struct{ Date string }
		if !bindActivity(c, &input) {
			return
		}
		date, err := time.Parse("2006-01-02", input.Date)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("invalid date"))
			return
		}
		jsonMsg(c, "", activityService.PurgeLoginEvents(date, time.Now()))
	})
	g.GET("/renewals", func(c *gin.Context) {
		user := session.GetLoginUser(c)
		rows, err := activityService.RenewalEvents(user, activityBefore(c))
		more := len(rows) > service.ActivityPageSize
		if more {
			rows = rows[:service.ActivityPageSize]
		}
		jsonObj(c, gin.H{"rows": rows, "more": more, "enabled": !user.RenewalNoticesDisabled}, err)
	})
	g.POST("/renewals/settings", func(c *gin.Context) {
		var input struct{ Enabled *bool }
		if !bindActivity(c, &input) {
			return
		}
		if input.Enabled == nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("missing notification preference"))
			return
		}
		jsonMsg(c, "", activityService.SetRenewalNotices(session.GetLoginUser(c), *input.Enabled))
	})
	g.GET("/notices", func(c *gin.Context) {
		user := session.GetLoginUser(c)
		expired := service.ActivityNotice{}
		if user.IsPanelOwner || user.Can(model.PermAccessInbounds) {
			var err error
			expired, err = activityService.Notifications(user, "expired")
			if err != nil {
				jsonObj(c, nil, err)
				return
			}
		}
		renewals, err := activityService.Notifications(user, "renewals")
		if err != nil {
			jsonObj(c, nil, err)
			return
		}
		changes, err := activityService.Notifications(user, "changes")
		if err != nil {
			jsonObj(c, nil, err)
			return
		}
		payments, err := activityService.Notifications(user, "payments")
		var lowQuota service.ActivityNotice
		if err == nil && user.Can(model.PermAccessInbounds) && !session.LowQuotaNoticeRead(c) {
			lowQuota, err = activityService.LowQuotaNotice(user)
		}
		jsonObj(c, gin.H{"expired": expired, "renewals": renewals, "payments": payments, "changes": changes, "lowQuota": lowQuota}, err)
	})
	g.POST("/notices/dismiss", func(c *gin.Context) {
		var input struct {
			Kind    string
			Through int64
		}
		if !bindActivity(c, &input) {
			return
		}
		user := session.GetLoginUser(c)
		valid := input.Kind == "renewals" || input.Kind == "changes" && user.IsPanelOwner ||
			input.Kind == "expired" && (user.IsPanelOwner || user.Can(model.PermAccessInbounds)) ||
			(input.Kind == "payments" || input.Kind == "lowQuota") && user.Can(model.PermAccessInbounds)
		if input.Through <= 0 || !valid {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("invalid notice"))
			return
		}
		if input.Kind == "lowQuota" {
			jsonMsg(c, "", session.MarkLowQuotaNoticeRead(c))
			return
		}
		jsonMsg(c, "", activityService.DismissNotice(user, input.Kind, input.Through))
	})
	g.GET("/lowQuota", requirePerm(model.PermAccessInbounds), func(c *gin.Context) {
		user := session.GetLoginUser(c)
		rows, err := activityService.LowQuotaClients(user, activityBefore(c))
		more := len(rows) > service.ActivityPageSize
		if more {
			rows = rows[:service.ActivityPageSize]
		}
		jsonObj(c, gin.H{"rows": rows, "more": more, "enabled": !user.LowQuotaNoticesDisabled}, err)
	})
	g.POST("/lowQuota/settings", requirePerm(model.PermAccessInbounds), func(c *gin.Context) {
		var input struct{ Enabled *bool }
		if !bindActivity(c, &input) {
			return
		}
		if input.Enabled == nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("missing low quota preference"))
			return
		}
		jsonMsg(c, "", activityService.SetLowQuotaNotices(session.GetLoginUser(c), *input.Enabled))
	})
	registerPaymentReminderRoutes(g.Group("/payments", requirePerm(model.PermAccessInbounds)))
	registerTrashRoutes(g.Group("/trash"))
	owner := g.Group("", requirePanelOwner())
	owner.GET("/gateway", func(c *gin.Context) {
		status, err := (&service.XrayService{}).GatewayStatus(c.Request.Context())
		jsonObj(c, status, err)
	})
	g.GET("/changes/settings", func(c *gin.Context) {
		enabled, err := activityService.ChangeLogging()
		if err != nil {
			jsonObj(c, nil, err)
			return
		}
		user := session.GetLoginUser(c)
		retentionDays := 0
		if user.IsPanelOwner {
			retentionDays, err = activityService.ChangeRetention()
			if err != nil {
				jsonObj(c, nil, err)
				return
			}
		}
		months, err := activityService.ChangeMonths(user)
		jsonObj(c, gin.H{"enabled": enabled, "months": months, "retentionDays": retentionDays}, err)
	})
	owner.POST("/changes/settings", func(c *gin.Context) {
		var input struct{ Enabled *bool }
		if !bindActivity(c, &input) {
			return
		}
		if input.Enabled == nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("missing activity setting"))
			return
		}
		jsonMsg(c, "", activityService.SetChangeLogging(*input.Enabled))
	})
	g.GET("/changes", func(c *gin.Context) {
		month, err := time.Parse("2006-01", c.Query("month"))
		if err != nil {
			jsonObj(c, nil, errors.New("invalid month"))
			return
		}
		user := session.GetLoginUser(c)
		owners := []string{}
		if user.IsPanelOwner {
			owners, err = activityService.ChangeOwners(month)
			if err != nil {
				jsonObj(c, nil, err)
				return
			}
		}
		rows, err := activityService.ChangeEvents(month, c.Query("owner"), activityBefore(c), user)
		more := len(rows) > service.ActivityPageSize
		if more {
			rows = rows[:service.ActivityPageSize]
		}
		jsonObj(c, gin.H{"rows": rows, "more": more, "owners": owners}, err)
	})
	g.POST("/changes/hide", func(c *gin.Context) {
		var input struct{ ID int64 }
		if !bindActivity(c, &input) {
			return
		}
		jsonMsg(c, "", activityService.HideClientChange(session.GetLoginUser(c), input.ID))
	})
	owner.POST("/changes/retention", func(c *gin.Context) {
		var input struct{ Days *int }
		if !bindActivity(c, &input) {
			return
		}
		if input.Days == nil || *input.Days < 0 || *input.Days > 3650 {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("invalid activity retention"))
			return
		}
		jsonMsg(c, "", activityService.SetChangeRetention(*input.Days))
	})
	owner.POST("/changes/purge", func(c *gin.Context) {
		var input struct{ Month string }
		if !bindActivity(c, &input) {
			return
		}
		month, err := time.Parse("2006-01", input.Month)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("invalid month"))
			return
		}
		jsonMsg(c, "", activityService.PurgeChangeEvents(month))
	})
	registerSubscriptionAppRoutes(owner)
}

func registerPaymentReminderRoutes(g *gin.RouterGroup) {
	g.GET("", func(c *gin.Context) {
		user := session.GetLoginUser(c)
		rows, err := activityService.PendingPayments(user, activityBefore(c))
		more := len(rows) > service.ActivityPageSize
		if more {
			rows = rows[:service.ActivityPageSize]
		}
		jsonObj(c, gin.H{"rows": rows, "more": more, "enabled": user.PaymentReminders}, err)
	})
	g.POST("/settings", func(c *gin.Context) {
		var input struct{ Enabled *bool }
		if !bindActivity(c, &input) {
			return
		}
		if input.Enabled == nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("missing payment reminder setting"))
			return
		}
		jsonMsg(c, "", activityService.SetPaymentReminders(session.GetLoginUser(c), *input.Enabled))
	})
	g.POST("/confirm", func(c *gin.Context) {
		var input struct{ ID int64 }
		if !bindActivity(c, &input) {
			return
		}
		err := activityService.ConfirmPayment(session.GetLoginUser(c), input.ID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.activity.paymentUnavailable"))
			return
		}
		if err != nil {
			jsonMsg(c, "", err)
			return
		}
		pureJsonMsg(c, http.StatusOK, true, I18nWeb(c, "pages.activity.paymentRecorded"))
	})
}

func registerTrashRoutes(g *gin.RouterGroup) {
	trash := service.TrashService{}
	for _, kind := range []string{service.TrashClient, service.TrashManager} {
		scope := g.Group("/" + kind)
		if kind == service.TrashManager {
			scope.Use(requirePanelOwner())
		}
		scope.GET("", func(c *gin.Context) {
			rows, err := trash.List(session.GetLoginUser(c), kind, activityBefore(c))
			activityRows(c, rows, err)
		})
		scope.POST("/restore", func(c *gin.Context) {
			var input struct {
				ID int64
				service.TrashClientPlan
			}
			if !bindActivity(c, &input) {
				return
			}
			ids, err := trash.Restore(session.GetLoginUser(c), kind, input.ID, &input.TrashClientPlan)
			if errors.Is(err, service.ErrTrashClientPlan) {
				pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.activity.trashClientNote"))
				return
			}
			if errors.Is(err, service.ErrTrashConflict) || errors.Is(err, gorm.ErrRecordNotFound) {
				pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.activity.trashUnavailable"))
				return
			}
			if err == nil && len(ids) != 0 {
				(&InboundController{}).reconcileForInbounds(ids, true)
			}
			jsonMsg(c, "", err)
		})
		scope.POST("/delete", func(c *gin.Context) {
			var input struct{ ID int64 }
			if !bindActivity(c, &input) {
				return
			}
			if input.ID <= 0 {
				jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("invalid trash entry"))
				return
			}
			jsonMsg(c, "", trash.Purge(session.GetLoginUser(c), kind, input.ID))
		})
		scope.POST("/empty", func(c *gin.Context) {
			var input struct{ Confirm bool }
			if !bindActivity(c, &input) {
				return
			}
			if !input.Confirm {
				jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("missing trash confirmation"))
				return
			}
			jsonMsg(c, "", trash.Purge(session.GetLoginUser(c), kind, 0))
		})
	}
}

func activityBefore(c *gin.Context) int64 {
	before, _ := strconv.ParseInt(c.Query("before"), 10, 64)
	return before
}

func bindActivity(c *gin.Context, input any) bool {
	if c.ContentType() != "application/json" || c.ShouldBindJSON(input) != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("invalid activity request"))
		return false
	}
	return true
}

func registerSubscriptionAppRoutes(g *gin.RouterGroup) {
	settings := &service.SettingService{}
	g.GET("/apps", func(c *gin.Context) {
		apps, err := settings.SubscriptionApps()
		jsonObj(c, apps, err)
	})
	g.POST("/apps", func(c *gin.Context) {
		var input struct{ Apps []service.SubscriptionApp }
		if !bindActivity(c, &input) {
			return
		}
		err := settings.SetSubscriptionApps(input.Apps)
		if errors.Is(err, service.ErrInvalidSubscriptionApps) {
			pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.activity.invalidApps"))
			return
		}
		jsonMsg(c, "", err)
	})
}
