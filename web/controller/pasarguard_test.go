package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPasarGuardReadAllDoesNotWidenOwnWrite(t *testing.T) {
	user := &model.User{Id: 2, Enable: true, IsReseller: true, RepresentativeRoleID: 3,
		RepresentativeRole: &model.RepresentativeRole{Permissions: `{"users":{"read":{"scope":2},"update":{"scope":1}}}`}}
	r := gin.New()
	r.GET("/test", withUser(user), requirePerm(model.PermAccessInbounds), requirePerm(model.PermEditClient), func(c *gin.Context) {
		if !user.IsReseller {
			t.Error("read-all leaked into own-write scope")
		}
		c.Status(200)
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", nil))
	if rec.Code != 200 {
		t.Fatal("own write was refused")
	}
}

func TestPasarGuardMissingResetPermissionCannotUseEditPermission(t *testing.T) {
	user := &model.User{Id: 2, Enable: true, IsReseller: true, RepresentativeRoleID: 3,
		RepresentativeRole: &model.RepresentativeRole{Permissions: `{"users":{"read":{"scope":1},"update":{"scope":1}}}`}}
	reached := false
	r := gin.New()
	r.POST("/resetClientTraffic/:email", withUser(user), requirePerm(model.PermEditClient), func(c *gin.Context) { reached = true; c.Status(200) })
	req := httptest.NewRequest(http.MethodPost, "/resetClientTraffic/customer", nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if reached {
		t.Fatal("reset permission bypassed")
	}
}
