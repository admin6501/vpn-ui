package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/op/go-logging"
)

func TestRestoreSchedulesRestartInAuthorizedRequestAfterCredentialReplacement(t *testing.T) {
	logger.InitLogger(logging.CRITICAL)
	gin.SetMode(gin.TestMode)
	t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
	t.Setenv("VPNUI_OFFLINE", "1")
	t.Setenv("VPNUI_NODE_AGENT", "1")
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	if err := service.SetNodeCorePaused("xray", true); err != nil {
		t.Fatal(err)
	}
	(&service.AccountService{}).MigrationAccounts()
	if err := (&service.GroupService{}).Migrate(); err != nil {
		t.Fatal(err)
	}
	var owner model.User
	database.GetDB().First(&owner, 1)
	snapshots := [][]byte{}
	for _, name := range []string{"backup-one-owner", "backup-two-owner"} {
		if err := database.GetDB().Model(&model.User{}).Where("id = ?", owner.Id).Update("username", name).Error; err != nil {
			t.Fatal(err)
		}
		data, err := (&service.ServerService{}).GetDb()
		if err != nil {
			t.Fatal(err)
		}
		snapshots = append(snapshots, data)
	}
	if err := database.GetDB().Model(&model.User{}).Where("id = ?", owner.Id).Update("username", "current-owner").Error; err != nil {
		t.Fatal(err)
	}
	scheduled := 0
	ctrl := &ServerController{restartAfterRestore: func(delay time.Duration) error {
		if delay <= 0 {
			t.Fatal("restart must wait for response")
		}
		scheduled++
		var current model.User
		database.GetDB().First(&current, owner.Id)
		if current.Username == "current-owner" {
			t.Fatal("restart was scheduled before replacement")
		}
		return nil
	}}
	r := gin.New()
	r.Use(withUser(&owner))
	r.POST("/restore", ctrl.importDB)
	post := func(data []byte) map[string]any {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		file, err := form.CreateFormFile("db", "backup.db")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(data); err != nil {
			t.Fatal(err)
		}
		form.Close()
		req := httptest.NewRequest("POST", "/restore", &body)
		req.Header.Set("Content-Type", form.FormDataContentType())
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err, w.Body.String())
		}
		return result
	}
	for i, data := range snapshots {
		result := post(data)
		if result["success"] != true || result["msg"] == "" || result["obj"] == nil {
			t.Fatal("restore did not report success", result)
		}
		if scheduled != i+1 {
			t.Fatal("restore requires a second authenticated request to finish", scheduled)
		}
		var current model.User
		database.GetDB().First(&current, owner.Id)
		want := []string{"backup-one-owner", "backup-two-owner"}[i]
		if current.Username != want {
			t.Fatal(current.Username, want)
		}
	}
	if result := post([]byte("invalid backup")); result["success"] != false {
		t.Fatal("invalid file was accepted", result)
	}
	if scheduled != 2 {
		t.Fatal("validation failure scheduled restart")
	}
	ctrl.restartAfterRestore = func(time.Duration) error { return errors.New("restart unavailable") }
	if result := post(snapshots[0]); result["success"] != false {
		t.Fatal("restart failure was hidden", result)
	}
}
