package web

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/controller"
	weblocale "github.com/mhsanaei/3x-ui/v2/web/locale"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/op/go-logging"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGroupsBrowserRuntime(t *testing.T) {
	if os.Getenv("RUN_NODE_UI_BROWSER") != "1" {
		t.Skip("enable Chromium checks")
	}
	gin.SetMode(gin.TestMode)
	logger.InitLogger(logging.CRITICAL)
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, lang := range []string{"en_US", "fa_IR"} {
		if _, err := bundle.LoadMessageFile("translation/translate." + lang + ".toml"); err != nil {
			t.Fatal(err)
		}
	}
	for _, lang := range []string{"en-US", "fa-IR"} {
		t.Run(lang, func(t *testing.T) {
			localizer := i18n.NewLocalizer(bundle, lang)
			tpl, err := (&Server{}).getHtmlTemplate(template.FuncMap{"i18n": func(key string, _ ...string) (string, error) {
				return localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
			}})
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
			if err := database.InitDB(config.GetDBPath()); err != nil {
				t.Fatal(err)
			}
			db := database.GetDB()
			if err := db.Create(&model.Setting{Key: "brandName", Value: "برند Nava & Friends"}).Error; err != nil {
				t.Fatal(err)
			}
			var root model.User
			if err := db.Where("is_super_admin = ?", true).First(&root).Error; err != nil {
				t.Fatal(err)
			}
			inbounds := []model.Inbound{{UserId: root.Id, Tag: "group-vless", Remark: "First inbound", Protocol: model.VLESS, Port: 443, Enable: true, Settings: `{"clients":[{"email":"existing-user","id":"11111111-1111-4111-8111-111111111111","password":"kept-password","subId":"kept-sub","enable":true}]}`, StreamSettings: `{"network":"tcp","security":"none"}`}, {UserId: root.Id, Tag: "group-trojan", Remark: "Second inbound", Protocol: model.Trojan, Port: 444, Enable: true, Settings: `{"clients":[]}`, StreamSettings: `{"network":"tcp","security":"none"}`}}
			if err := db.Create(&inbounds).Error; err != nil {
				t.Fatal(err)
			}
			sourceRows := []model.PasarGuardResource{
				{Kind: "groups", SourceID: 27, Data: `{"id":"27","name":"Original backup group","is_disabled":"f"}`},
				{Kind: "inbounds", SourceID: 200, Data: `{"id":"200","tag":"group-trojan"}`},
				{Kind: "inbounds_groups_association", SourceID: 1, Data: `{"group_id":"27","inbound_id":"200"}`},
			}
			if err := db.Create(&sourceRows).Error; err != nil {
				t.Fatal(err)
			}
			accounts := &service.AccountService{}
			accounts.MigrationAccounts()
			if err := (&service.GroupService{}).Migrate(); err != nil {
				t.Fatal(err)
			}
			third := model.Inbound{UserId: root.Id, Tag: "ungrouped", Remark: "Ungrouped inbound", Protocol: model.VLESS, Port: 445, Enable: true, Settings: `{"clients":[]}`, StreamSettings: `{"network":"tcp","security":"none"}`}
			if err := db.Create(&third).Error; err != nil {
				t.Fatal(err)
			}
			role := model.RepresentativeRole{Name: "Historic representative", Permissions: `{"users":{"read":{"scope":1},"create":true,"update":{"scope":1}},"system":{"read":true}}`, Limits: `{}`, Features: `{}`, Access: `{"allowed_group_ids":[]}`}
			if err := db.Create(&role).Error; err != nil {
				t.Fatal(err)
			}
			representative := model.User{Username: "group-representative", Enable: true, IsReseller: true, GroupAccess: true, RepresentativeRoleID: role.ID, RepresentativeRole: &role}
			delegate := model.User{Username: "group-delegate", Enable: true, Permissions: model.PermAccessInbounds | model.PermEditClient}
			for _, actor := range []*model.User{&representative, &delegate} {
				if err := db.Create(actor).Error; err != nil {
					t.Fatal(err)
				}
				for _, ib := range inbounds {
					if err := db.Create(&model.InboundAccess{UserId: actor.Id, InboundId: ib.Id}).Error; err != nil {
						t.Fatal(err)
					}
				}
			}
			var nativeGroups []model.ClientGroup
			if err := db.Find(&nativeGroups).Error; err != nil {
				t.Fatal(err)
			}
			for _, group := range nativeGroups {
				if err := db.Create(&model.RepresentativeGroupAccess{UserID: representative.Id, GroupID: group.ID}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Create(&model.ResellerProfile{UserId: representative.Id, ConsumptionBased: true, Unlimited: true, CreatedBy: root.Id}).Error; err != nil {
				t.Fatal(err)
			}
			r := gin.New()
			r.SetHTMLTemplate(tpl)
			r.Use(func(c *gin.Context) {
				actor := &root
				switch c.GetHeader("X-Test-Actor") {
				case "anonymous":
					actor = nil
				case "delegate":
					actor = &delegate
				case "representative":
					actor = &representative
				}
				// Production sessions load a fresh user per request. Keep that same
				// isolation when permission middleware enriches the cached test user.
				var requestActor *model.User
				if actor != nil {
					copy := *actor
					requestActor = &copy
				}
				c.Set("LOGIN_USER_ROW", requestActor)
				c.Set("base_path", "/test/")
				c.Set("I18n", func(_ weblocale.I18nType, key string, _ ...string) string {
					v, _ := localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
					return v
				})
				c.Next()
			})
			r.POST("/test/fixtures/representative", func(c *gin.Context) {
				err := db.Create(&model.ResellerClient{UserId: representative.Id, Email: "new-group-user", InboundId: inbounds[0].Id}).Error
				if err != nil {
					c.JSON(500, gin.H{"success": false})
					return
				}
				c.JSON(200, gin.H{"success": true})
			})
			r.POST("/test/fixtures/traffic", func(c *gin.Context) {
				if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", "new-group-user").Updates(map[string]any{"up": 1024, "down": 2048, "all_time": 4096}).Error; err != nil {
					t.Error(err)
				}
				c.JSON(200, gin.H{"success": true})
			})
			r.POST("/test/fixtures/brand", func(c *gin.Context) {
				if err := db.Model(&model.Setting{}).Where("key = ?", "brandName").Update("value", c.PostForm("brandName")).Error; err != nil {
					t.Error(err)
				}
				c.JSON(200, gin.H{"success": true})
			})
			controller.NewIndexController(r.Group("/test"))
			controller.NewXUIController(r.Group("/test"))
			controller.NewClientsController(r.Group("/test/panel/api/clients"))
			controller.NewInboundController(r.Group("/test/panel/api/inbounds"))
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
				if strings.HasPrefix(rq.URL.Path, "/test/assets/") {
					http.StripPrefix("/test/", http.FileServer(http.FS(assetsFS))).ServeHTTP(w, rq)
					return
				}
				r.ServeHTTP(w, rq)
			}))
			defer srv.Close()
			script, _ := filepath.Abs("../test_unit/groups_browser_test.cjs")
			ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "node", script)
			cmd.Env = append(os.Environ(), "GROUPS_URL="+srv.URL+"/test/panel/groups")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("browser: %v\n%s", err, out)
			} else {
				t.Log(string(out))
			}
			var existing model.Account
			if err := db.Where("email = ?", "existing-user").First(&existing).Error; err != nil {
				t.Fatal(err)
			}
			if existing.UUID != "11111111-1111-4111-8111-111111111111" || existing.SubID != "kept-sub" {
				t.Fatal("migration or group edits replaced existing identity")
			}
			var created model.Account
			if err := db.Where("email = ?", "new-group-user").First(&created).Error; err != nil {
				t.Fatal(err)
			}
			var traffic xray.ClientTraffic
			if err := db.Where("email = ?", "new-group-user").First(&traffic).Error; err != nil || traffic.Up != 0 || traffic.Down != 0 || traffic.AllTime != 4096 {
				t.Fatal("edit-form reset must preserve lifetime usage", err)
			}
			ids, err := accounts.GetMembershipInboundIds(created.Id)
			if err != nil || len(ids) != 2 {
				t.Fatal("new user did not receive group's inbounds", ids, err)
			}
			var memberships int64
			db.Model(&model.AccountInbound{}).Where("inbound_id = ?", third.Id).Count(&memberships)
			if memberships != 0 {
				t.Fatal("ungrouped inbound received users")
			}
		})
	}
}
