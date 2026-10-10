package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/nodewire"
	"github.com/mhsanaei/3x-ui/v2/web/service"
)

// RegisterNodeAgentRoutes are credential-authenticated, not browser-session APIs.
// They expose no administrative operations and an agent is scoped to one node.
func RegisterNodeAgentRoutes(g *gin.RouterGroup) {
	group := g.Group("/node-agent")
	group.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<20)
	})
	group.POST("/enroll", func(c *gin.Context) {
		var request nodewire.Enrollment
		if c.ShouldBindJSON(&request) != nil {
			c.AbortWithStatus(400)
			return
		}
		id, e := (&service.NodeService{}).Enroll(request)
		if e != nil {
			c.AbortWithStatus(403)
			return
		}
		c.JSON(200, nodewire.EnrollmentReply{NodeID: id})
	})
	group.POST("/:id/sync", func(c *gin.Context) {
		id, e := strconv.Atoi(c.Param("id"))
		if e != nil {
			c.AbortWithStatus(404)
			return
		}
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatus(403)
			return
		}
		svc := &service.NodeService{}
		if _, e = svc.Authenticate(id, strings.TrimPrefix(header, "Bearer ")); e != nil {
			c.AbortWithStatus(403)
			return
		}
		var report nodewire.Report
		if c.ShouldBindJSON(&report) != nil {
			c.AbortWithStatus(400)
			return
		}
		if e = svc.AcceptReport(id, report); e != nil {
			c.AbortWithStatus(409)
			return
		}
		snapshot, e := svc.Snapshot(id)
		if e != nil {
			c.AbortWithStatus(503)
			return
		}
		c.JSON(200, snapshot)
	})
}
func NewNodeController(g *gin.RouterGroup) {
	nodes := g.Group("/nodes")
	nodes.Use(requireSuperAdmin())
	nodes.GET("/", func(c *gin.Context) { html(c, "nodes.html", "pages.nodes.title", nil) })
	nodes.GET("/list", func(c *gin.Context) {
		var rows []model.RemoteNode
		err := database.GetDB().Order("id").Find(&rows).Error
		result := []gin.H{}
		for _, n := range rows {
			var report any
			_ = json.Unmarshal([]byte(n.CoreReport), &report)
			result = append(result, gin.H{"node": n, "paired": n.TokenHash != "", "online": n.LastSeen > time.Now().Add(-35*time.Second).UnixMilli(), "report": report})
		}
		jsonObj(c, result, err)
	})
	nodes.POST("/add", func(c *gin.Context) {
		var body struct {
			Name    string `json:"name" form:"name"`
			Address string `json:"address" form:"address"`
		}
		if e := c.ShouldBind(&body); e != nil {
			jsonMsg(c, "", e)
			return
		}
		n, token, e := (&service.NodeService{}).Create(body.Name, body.Address)
		jsonObj(c, gin.H{"node": n, "pairToken": token}, e)
	})
	nodes.POST("/:id/update", func(c *gin.Context) {
		id, e := strconv.Atoi(c.Param("id"))
		var body struct {
			Name    string `json:"name" form:"name"`
			Address string `json:"address" form:"address"`
			Enable  bool   `json:"enable" form:"enable"`
		}
		if e == nil {
			e = c.ShouldBind(&body)
		}
		if e == nil {
			e = (&service.NodeService{}).Update(id, body.Name, body.Address, body.Enable)
		}
		jsonMsg(c, "", e)
	})
	nodes.POST("/:id/delete", func(c *gin.Context) {
		id, e := strconv.Atoi(c.Param("id"))
		if e == nil {
			e = (&service.NodeService{}).Delete(id)
		}
		jsonMsg(c, "", e)
	})
	nodes.POST("/:id/pair", func(c *gin.Context) {
		id, e := strconv.Atoi(c.Param("id"))
		token := ""
		if e == nil {
			token, e = (&service.NodeService{}).Rotate(id)
		}
		jsonObj(c, gin.H{"pairToken": token, "id": id}, e)
	})
	nodes.POST("/:id/install", func(c *gin.Context) {
		id, e := strconv.Atoi(c.Param("id"))
		var body struct {
			Cores []string `json:"cores" form:"cores"`
		}
		if e == nil {
			e = c.ShouldBind(&body)
		}
		if e == nil {
			e = (&service.NodeService{}).InstallCores(id, body.Cores)
		}
		jsonMsg(c, "", e)
	})
}
