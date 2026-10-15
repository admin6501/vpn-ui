package controller

import (
	"strconv"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/web/session"

	"github.com/gin-gonic/gin"
)

// adminForm is the add/edit payload. Bodies arrive form-urlencoded (axios
// Qs.stringify's everything), so these bind by `form` tag via ShouldBind, never
// ShouldBindJSON. Permissions arrive as repeated `permissions` keys, which is what
// Qs's arrayFormat:'repeat' emits for an array.
type adminForm struct {
	AdminRoleID  *int     `json:"adminRoleId" form:"adminRoleId"`
	Id           int      `json:"id" form:"id"`
	Username     string   `json:"username" form:"username"`
	Password     string   `json:"password" form:"password"`
	Nickname     string   `json:"nickname" form:"nickname"`
	Enable       bool     `json:"enable" form:"enable"`
	IsSuperAdmin bool     `json:"isSuperAdmin" form:"isSuperAdmin"`
	Permissions  []string `json:"permissions" form:"permissions"`
	// InboundIds is retained for legacy callers before the V3 group migration.
	// GroupIDs uses a blank entry to revoke all; omission preserves existing grants.
	InboundIds []string `json:"inboundIds" form:"inboundIds"`
	GroupIDs   []string `json:"groupIds" form:"groupIds"`
}

// inboundIds parses the wire form's ids, dropping blanks and unparseable entries
// rather than failing the whole save over one bad value.
func (f *adminForm) inboundIds() []int {
	out := make([]int, 0, len(f.InboundIds))
	for _, raw := range f.InboundIds {
		if raw == "" {
			continue
		}
		if id, err := strconv.Atoi(raw); err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out
}

// spec maps the wire form onto the service's shape.
func (f *adminForm) spec() service.AdminSpec {
	return service.AdminSpec{
		AdminRoleID:  f.AdminRoleID,
		Username:     f.Username,
		Password:     f.Password,
		Nickname:     f.Nickname,
		Permissions:  model.PermissionsFromSlugs(f.Permissions),
		Enable:       f.Enable,
		IsSuperAdmin: f.IsSuperAdmin,
		InboundIds:   f.inboundIds(),
		GroupIDs:     (&resellerForm{GroupIDs: f.GroupIDs}).groupIDs(),
	}
}

// AdminController serves admin and role CRUD for super admins and the protected owner; the
// gating lives on the group below, not in the handlers.
type AdminController struct {
	BaseController

	adminService service.AdminService
}

// NewAdminController creates an AdminController and initializes its routes.
func NewAdminController(g *gin.RouterGroup) *AdminController {
	a := &AdminController{}
	a.initRouter(g)
	return a
}

func (a *AdminController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/admins")
	g.Use(requireAdminManagement())

	g.GET("/list", a.list)
	g.GET("/roles", a.roles)
	g.POST("/roles/add", a.saveRole)
	g.POST("/roles/update/:id", a.saveRole)
	g.POST("/roles/delete/:id", a.deleteRole)
	g.GET("/permissions", a.permissions)
	g.GET("/inbounds", a.inbounds)
	g.GET("/groups", a.groups)
	g.POST("/add", a.add)
	g.POST("/update/:id", a.update)
	g.POST("/del/:id", a.del)
	g.POST("/reassign/:from/:to", a.reassign)
}

func (a *AdminController) list(c *gin.Context) {
	admins, err := a.adminService.GetAdmins()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.title"), err)
		return
	}
	jsonObj(c, admins, nil)
}

// permissions returns the canonical slug list so the UI renders exactly the
// permissions this build enforces, rather than a hard-coded copy that can drift.
func (a *AdminController) permissions(c *gin.Context) {
	jsonObj(c, model.AllPermissions, nil)
}

// inbounds retains the legacy read endpoint for administrator-management clients.
// The current grant checklist uses groups instead.
func (a *AdminController) inbounds(c *gin.Context) {
	list, err := a.adminService.AllInboundsBrief()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.title"), err)
		return
	}
	jsonObj(c, list, nil)
}

func (a *AdminController) groups(c *gin.Context) {
	rows, err := a.adminService.AssignableGroups(session.GetLoginUser(c))
	jsonObj(c, rows, err)
}

func (a *AdminController) add(c *gin.Context) {
	form := &adminForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.add"), err)
		return
	}
	_, err := a.adminService.AddAdmin(form.spec())
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.add"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.admins.add"), nil)
}

func (a *AdminController) update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.edit"), err)
		return
	}
	form := &adminForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.edit"), err)
		return
	}
	// Protect the permanent owner and prevent disabling the current login.
	// Self-demotion remains possible while administrator recovery is preserved.
	if err := a.adminService.GuardAdminTarget(session.GetLoginUser(c), id); err != nil {
		adminRoleResult(c, err)
		return
	}
	if me := session.GetLoginUser(c); me != nil && me.Id == id && !form.Enable {
		jsonMsg(c, I18nWeb(c, "pages.admins.edit"), errSelfDemote)
		return
	}
	err = a.adminService.UpdateAdmin(id, form.spec())
	if err != nil {
		adminRoleResult(c, err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.admins.edit"), nil)
}

func (a *AdminController) del(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.del"), err)
		return
	}
	if err := a.adminService.GuardAdminTarget(session.GetLoginUser(c), id); err != nil {
		adminRoleResult(c, err)
		return
	}
	if me := session.GetLoginUser(c); me != nil && me.Id == id {
		jsonMsg(c, I18nWeb(c, "pages.admins.del"), errSelfDelete)
		return
	}
	if err := a.adminService.DeleteAdmin(id); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.del"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.admins.del"), nil)
}

func (a *AdminController) reassign(c *gin.Context) {
	from, err := strconv.Atoi(c.Param("from"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.reassign"), err)
		return
	}
	to, err := strconv.Atoi(c.Param("to"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.reassign"), err)
		return
	}
	moved, err := a.adminService.ReassignInbounds(from, to)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.admins.reassign"), err)
		return
	}
	jsonObj(c, gin.H{"moved": moved}, nil)
}

func (a *AdminController) roles(c *gin.Context) {
	rows, err := a.adminService.AdminRoles()
	jsonObj(c, rows, err)
}
func (a *AdminController) saveRole(c *gin.Context) {
	var body struct {
		Name        string   `json:"name" form:"name"`
		Permissions []string `json:"permissions" form:"permissions"`
	}
	if err := c.ShouldBind(&body); err != nil {
		jsonMsg(c, "", err)
		return
	}
	id := 0
	var err error
	if c.Param("id") != "" {
		id, err = strconv.Atoi(c.Param("id"))
		if err != nil || id <= 0 {
			jsonMsg(c, "", service.ErrAdminNotFound)
			return
		}
	}
	err = a.adminService.SaveAdminRole(session.GetLoginUser(c), id, body.Name, model.PermissionsFromSlugs(body.Permissions))
	adminRoleResult(c, err)
}
func (a *AdminController) deleteRole(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err == nil && id > 0 {
		err = a.adminService.DeleteAdminRole(session.GetLoginUser(c), id)
	} else {
		err = service.ErrAdminNotFound
	}
	adminRoleResult(c, err)
}
func adminRoleResult(c *gin.Context, err error) {
	key := ""
	switch err {
	case service.ErrProtectedAdmin:
		key = "pages.admins.ownerProtected"
	case service.ErrAdminRoleAssigned:
		key = "pages.admins.roleAssigned"
	case service.ErrAdminRoleName:
		key = "pages.admins.roleNameRequired"
	case service.ErrAdminRoleDuplicate:
		key = "pages.admins.roleDuplicate"
	}
	if key != "" {
		pureJsonMsg(c, 200, false, I18nWeb(c, key))
		return
	}
	jsonMsg(c, "", err)
}
