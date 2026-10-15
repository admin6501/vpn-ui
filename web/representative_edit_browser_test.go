package web

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/controller"
	weblocale "github.com/mhsanaei/3x-ui/v2/web/locale"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/op/go-logging"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
)

func TestRepresentativeEditBrowserRuntime(t *testing.T) {
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
		for _, kind := range []string{"owner", "delegated", "imported-owner", "imported-delegated", "template-owner", "limited-owner"} {
			t.Run(lang+"/"+kind, func(t *testing.T) {
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
				var owner model.User
				if err := db.Where("is_panel_owner = ?", true).First(&owner).Error; err != nil {
					t.Fatal(err)
				}
				manager := model.User{Username: "delegated-manager", Password: "existing-password-hash", Enable: true, Permissions: model.PermManageResellers}
				if err := db.Create(&manager).Error; err != nil {
					t.Fatal(err)
				}
				role := model.RepresentativeRole{Name: "Representative role", Permissions: `{"users":{"read":{"scope":1},"create":true,"update":{"scope":1}}}`, Limits: `{}`, Features: `{}`, Access: `{}`}
				if strings.HasPrefix(kind, "imported-") {
					role.Access = `{"allowed_group_ids":[]}`
				}
				if kind == "template-owner" {
					role.Access = `{"require_template":true}`
				}
				if err := db.Create(&role).Error; err != nil {
					t.Fatal(err)
				}
				rep := model.User{Username: "editable-representative", Password: "existing-password-hash", Enable: true, IsReseller: true, RepresentativeRoleID: role.ID}
				if err := db.Create(&rep).Error; err != nil {
					t.Fatal(err)
				}
				inbound := model.Inbound{UserId: owner.Id, Tag: "imported-inbound", Port: 12000, Protocol: model.VLESS, Settings: `{"clients":[]}`, StreamSettings: `{}`}
				if err := db.Create(&inbound).Error; err != nil {
					t.Fatal(err)
				}
				for _, userID := range []int{rep.Id, manager.Id, owner.Id} {
					if err := db.Create(&model.InboundAccess{UserId: userID, InboundId: inbound.Id}).Error; err != nil {
						t.Fatal(err)
					}
				}
				creatorID := owner.Id
				if strings.Contains(kind, "delegated") {
					creatorID = manager.Id
				}
				if err := db.Create(&model.ResellerProfile{UserId: rep.Id, CreatedBy: creatorID, ConsumptionBased: true, AllowanceBytes: 10 << 30}).Error; err != nil {
					t.Fatal(err)
				}
				if kind == "owner" {
					second := model.Inbound{UserId: owner.Id, Tag: "second-group-inbound", Remark: "Additional group", Port: 12001, Protocol: model.Trojan, Settings: `{"clients":[]}`, StreamSettings: `{}`}
					if err := db.Create(&second).Error; err != nil {
						t.Fatal(err)
					}
				}
				(&service.AccountService{}).MigrationAccounts()
				if err := (&service.GroupService{}).Migrate(); err != nil {
					t.Fatal(err)
				}
				actor := &owner
				if strings.Contains(kind, "delegated") {
					actor = &manager
				}
				if kind == "limited-owner" {
					owner.IsSuperAdmin = false
					owner.Permissions = model.PermManageResellers
					if err := db.Model(&model.User{}).Where("id = ?", owner.Id).Updates(map[string]any{"is_super_admin": false, "permissions": model.PermManageResellers}).Error; err != nil {
						t.Fatal(err)
					}
				}
				r := gin.New()
				r.SetHTMLTemplate(tpl)
				r.Use(func(c *gin.Context) {
					c.Set("LOGIN_USER_ROW", actor)
					c.Set("base_path", "/test/")
					c.Set("I18n", func(_ weblocale.I18nType, key string, _ ...string) string {
						value, _ := localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
						return value
					})
					c.Next()
				})
				controller.NewXUIController(r.Group("/test"))
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
					if strings.HasPrefix(rq.URL.Path, "/test/assets/") {
						http.StripPrefix("/test/", http.FileServer(http.FS(assetsFS))).ServeHTTP(w, rq)
						return
					}
					r.ServeHTTP(w, rq)
				}))
				defer srv.Close()
				script, _ := filepath.Abs("../test_unit/representative_edit_browser_test.cjs")
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "node", script)
				cmd.Env = append(os.Environ(), "REPRESENTATIVE_EDIT_URL="+srv.URL+"/test/panel/resellers", "REPRESENTATIVE_EDIT_KIND="+kind)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("browser: %v\n%s", err, out)
				} else {
					t.Log(string(out))
				}
				var stored model.User
				if err := db.First(&stored, rep.Id).Error; err != nil {
					t.Fatal(err)
				}
				if stored.Nickname != "Edited in browser" || !stored.TwoFactorRequired {
					t.Fatal("changes were not stored", stored.Nickname, stored.TwoFactorRequired)
				}
				var profile model.ResellerProfile
				if err := db.Where("user_id = ?", rep.Id).First(&profile).Error; err != nil {
					t.Fatal(err)
				}
				if profile.AllowanceBytes != 10<<30 {
					t.Fatal("editing metadata changed the traffic allowance")
				}
				var grants []model.InboundAccess
				if err := db.Where("user_id = ?", rep.Id).Find(&grants).Error; err != nil {
					t.Fatal(err)
				}
				wantGrants := 1
				if kind == "owner" {
					wantGrants = 2
				}
				if len(grants) != wantGrants || grants[0].InboundId != inbound.Id {
					t.Fatal("editing metadata changed existing inbound access")
				}
				if stored.Password != rep.Password {
					t.Fatal("blank password changed credentials")
				}
			})
		}
	}
}
