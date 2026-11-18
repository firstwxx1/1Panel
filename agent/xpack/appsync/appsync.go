//go:build xpack || enterprise

// Package appsync backs "app transfer": it exposes an installed app's record and
// re-creates that record on another node once the master has copied the app directory.
package appsync

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/gin-gonic/gin"
)

var safeIdentRule = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func validateSafeIdent(s string) error {
	if s == "" || s == "." || s == ".." || !safeIdentRule.MatchString(s) {
		return fmt.Errorf("invalid app identifier: %q", s)
	}
	return nil
}

type Meta struct {
	Install model.AppInstall `json:"install"`
	AppKey  string           `json:"appKey"`
	Dir     string           `json:"dir"`    // app directory on this node
	Parent  string           `json:"parent"` // its parent, where it must land on the target
	Images  []string         `json:"images"`
}

func loadInstall(name string) (*model.AppInstall, error) {
	if err := validateSafeIdent(name); err != nil {
		return nil, err
	}
	var in model.AppInstall
	if err := global.DB.Where("name = ?", name).First(&in).Error; err != nil {
		return nil, fmt.Errorf("app not found: %s", name)
	}
	if err := global.DB.Where("id = ?", in.AppId).First(&in.App).Error; err != nil {
		return nil, err
	}
	return &in, nil
}

func ensurePanelNetwork() {
	if exec.Command("docker", "network", "inspect", "1panel-network").Run() != nil {
		_ = exec.Command("docker", "network", "create", "1panel-network").Run()
	}
}

func compose(dir string, args ...string) ([]byte, error) {
	a := append([]string{"compose", "-f", filepath.Join(dir, "docker-compose.yml")}, args...)
	return exec.Command("docker", a...).CombinedOutput()
}

func meta(name string) (*Meta, error) {
	in, err := loadInstall(name)
	if err != nil {
		return nil, err
	}
	m := &Meta{Install: *in, AppKey: in.App.Key, Dir: in.GetPath(), Parent: in.GetAppPath()}
	if out, err := compose(m.Dir, "config", "--images"); err == nil {
		for _, l := range strings.Split(string(out), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				m.Images = append(m.Images, l)
			}
		}
	}
	return m, nil
}

type ImportReq struct {
	Install model.AppInstall `json:"install"`
	AppKey  string           `json:"appKey"`
	Start   bool             `json:"start"`
}

func importApp(req ImportReq) error {
	in := req.Install
	if err := validateSafeIdent(in.Name); err != nil {
		return err
	}
	if err := validateSafeIdent(req.AppKey); err != nil {
		return err
	}
	var n int64
	global.DB.Model(&model.AppInstall{}).Where("name = ?", in.Name).Count(&n)
	if n > 0 {
		return fmt.Errorf("an app named %s is already installed on this node", in.Name)
	}
	if in.ContainerName != "" {
		global.DB.Model(&model.AppInstall{}).Where("container_name = ?", in.ContainerName).Count(&n)
		if n > 0 {
			return fmt.Errorf("container name %s is already used by another app on this node", in.ContainerName)
		}
	}
	if in.HttpPort > 0 {
		global.DB.Model(&model.AppInstall{}).Where("http_port = ? OR https_port = ?", in.HttpPort, in.HttpPort).Count(&n)
		if n > 0 {
			return fmt.Errorf("port %d is already used by another app on this node", in.HttpPort)
		}
	}
	if in.HttpsPort > 0 {
		global.DB.Model(&model.AppInstall{}).Where("https_port = ? OR http_port = ?", in.HttpsPort, in.HttpsPort).Count(&n)
		if n > 0 {
			return fmt.Errorf("https port %d is already used by another app on this node", in.HttpsPort)
		}
	}
	var app model.App
	if err := global.DB.Where("`key` = ?", req.AppKey).First(&app).Error; err != nil {
		return fmt.Errorf("app %s is not in this node's app store; sync the app list first", req.AppKey)
	}
	var detail model.AppDetail
	if err := global.DB.Where("app_id = ? AND version = ?", app.ID, in.Version).First(&detail).Error; err != nil {
		return fmt.Errorf("version %s of %s is not available on this node", in.Version, req.AppKey)
	}
	in.ID, in.AppId, in.AppDetailId = 0, app.ID, detail.ID
	in.App = app
	if _, err := compose(in.GetPath(), "config", "-q"); err != nil {
		return fmt.Errorf("app files are missing or invalid in %s", in.GetPath())
	}
	in.Status = "Stopped"
	if err := global.DB.Create(&in).Error; err != nil {
		return err
	}
	if req.Start {
		ensurePanelNetwork()
		if out, err := compose(in.GetPath(), "up", "-d"); err != nil {
			global.DB.Model(&in).Updates(map[string]interface{}{"status": "Error", "message": string(out)})
			return fmt.Errorf("start failed: %s", strings.TrimSpace(string(out)))
		}
		global.DB.Model(&in).Update("status", "Running")
	}
	return nil
}

func Register(g *gin.RouterGroup) {
	g.GET("/app/meta", func(c *gin.Context) {
		m, err := meta(c.Query("name"))
		if err != nil {
			helper.BadRequest(c, err)
			return
		}
		helper.SuccessWithData(c, m)
	})
	// where an app of this store key must be placed on this node
	g.GET("/app/dir", func(c *gin.Context) {
		key := c.Query("key")
		if err := validateSafeIdent(key); err != nil {
			helper.BadRequest(c, err)
			return
		}
		var app model.App
		if err := global.DB.Where("`key` = ?", key).First(&app).Error; err != nil {
			app = model.App{Key: key}
			if strings.HasPrefix(key, constant.AppResourceLocal) {
				app.Resource = constant.AppResourceLocal
			}
		}
		in := model.AppInstall{Name: "x", App: app}
		helper.SuccessWithData(c, in.GetAppPath())
	})
	g.POST("/app/import", func(c *gin.Context) {
		var req ImportReq
		if err := c.ShouldBindJSON(&req); err != nil {
			helper.BadRequest(c, err)
			return
		}
		if err := importApp(req); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": http.StatusInternalServerError, "message": err.Error()})
			return
		}
		helper.Success(c)
	})
	g.POST("/app/compose", func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
			Op   string `json:"op"` // stop | start
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			helper.BadRequest(c, err)
			return
		}
		in, err := loadInstall(req.Name)
		if err != nil {
			helper.BadRequest(c, err)
			return
		}
		arg := []string{"stop"}
		status := "Stopped"
		if req.Op == "start" {
			ensurePanelNetwork()
			arg, status = []string{"up", "-d"}, "Running"
		}
		if out, err := compose(in.GetPath(), arg...); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": http.StatusInternalServerError, "message": strings.TrimSpace(string(out))})
			return
		}
		global.DB.Model(in).Update("status", status)
		helper.Success(c)
	})
}
