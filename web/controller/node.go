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
	nodes.Use(requirePerm(model.PermManageNodes))
	nodes.GET("/", func(c *gin.Context) { html(c, "nodes.html", "pages.nodes.title", nil) })
	nodes.GET("/:id/xray", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		var node model.RemoteNode
		if err != nil || database.GetDB().First(&node, id).Error != nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		html(c, "xray.html", "pages.nodes.configuration", gin.H{"node_id": node.ID, "node_name": node.Name})
	})
	nodes.GET("/:id/xray/meta", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		var node model.RemoteNode
		if err != nil || database.GetDB().First(&node, id).Error != nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		tags := []string{}
		err = database.GetDB().Model(&model.Inbound{}).Where("node_id = ?", id).Pluck("tag", &tags).Error
		jsonObj(c, gin.H{"inboundTags": tags, "defaults": json.RawMessage(service.DefaultNodeTemplate())}, err)
	})
	nodes.GET("/list", func(c *gin.Context) {
		var rows []model.RemoteNode
		err := database.GetDB().Order("id").Find(&rows).Error
		result := []gin.H{}
		for _, n := range rows {
			var report map[string]any
			_ = json.Unmarshal([]byte(n.CoreReport), &report)
			if report != nil {
				var cmd nodewire.ConfigCommand
				if json.Unmarshal([]byte(n.ConfigCommand), &cmd) == nil && cmd.Core != "" {
					targets, _ := report["configTargets"].([]any)
					for _, target := range service.NodeInboundConfigurationTargets(n.ID, cmd.Core) {
						targets = append(targets, target)
					}
					report["configTargets"] = targets
				}
			}
			result = append(result, gin.H{"node": n, "paired": n.TokenHash != "", "online": n.LastSeen > time.Now().Add(-35*time.Second).UnixMilli(), "report": report})
		}
		jsonObj(c, result, err)
	})
	nodes.POST("/:id/control", func(c *gin.Context) {
		id, e := strconv.Atoi(c.Param("id"))
		var body struct {
			Action string `json:"action" form:"action"`
		}
		if e == nil {
			e = c.ShouldBind(&body)
		}
		if e == nil {
			e = (&service.NodeService{}).Control(id, body.Action)
		}
		jsonMsg(c, "", e)
	})
	nodes.POST("/:id/core-action", func(c *gin.Context) {
		id, e := strconv.Atoi(c.Param("id"))
		var body struct {
			Core   string `json:"core" form:"core"`
			Action string `json:"action" form:"action"`
		}
		if e == nil {
			e = c.ShouldBind(&body)
		}
		if e == nil {
			e = (&service.NodeService{}).CoreAction(id, nodewire.CoreAction{Core: body.Core, Action: body.Action})
		}
		jsonMsg(c, "", e)
	})

	nodes.POST("/:id/config", func(c *gin.Context) {
		id, e := strconv.Atoi(c.Param("id"))
		var body struct {
			Core    string `json:"core" form:"core"`
			Inbound int    `json:"inbound" form:"inbound"`
			File    string `json:"file" form:"file"`
			Text    string `json:"text" form:"text"`
			Inspect bool   `json:"inspect" form:"inspect"`
			Reset   bool   `json:"reset" form:"reset"`
		}
		if e == nil {
			e = c.ShouldBind(&body)
		}
		if e == nil {
			e = (&service.NodeService{}).Configure(id, nodewire.ConfigCommand{Core: body.Core, Inbound: body.Inbound, File: body.File, Text: body.Text, Inspect: body.Inspect, Reset: body.Reset})
		}
		jsonMsg(c, "", e)
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
