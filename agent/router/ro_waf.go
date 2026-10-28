//go:build xpack || enterprise

package router

import (
	"fmt"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/xpack/waf"
	"github.com/gin-gonic/gin"
)

type WafRouter struct{}

func wafStore() (*waf.Store, error) {
	app, err := repo.NewIAppRepo().GetFirst(repo.NewIAppRepo().WithKey(constant.AppOpenresty))
	if err != nil {
		return nil, err
	}
	install, err := repo.NewIAppInstallRepo().GetFirst(repo.NewIAppInstallRepo().WithAppId(app.ID))
	if err != nil {
		return nil, err
	}
	return &waf.Store{OpenrestyPath: install.GetPath(), Container: install.ContainerName}, nil
}

// wafHandle binds the JSON body for POST requests and runs fn with the WAF store.
func wafHandle[T any](fn func(s *waf.Store, req T) (interface{}, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req T
		if c.Request.Method == "POST" {
			if err := c.ShouldBindJSON(&req); err != nil {
				helper.BadRequest(c, err)
				return
			}
		}
		s, err := wafStore()
		if err != nil {
			helper.InternalServer(c, err)
			return
		}
		res, err := fn(s, req)
		if err != nil {
			helper.InternalServer(c, err)
			return
		}
		if res == nil {
			helper.Success(c)
			return
		}
		helper.SuccessWithData(c, res)
	}
}

type wafPatchReq struct {
	Key   string                 `json:"key"`
	Patch map[string]interface{} `json:"patch"`
}

type wafRuleListReq struct {
	Name   string `json:"name"`
	Site   string `json:"site"`
	Search string `json:"search"`
	Page   int    `json:"page"`
	Size   int    `json:"size"`
}

type wafRuleSaveReq struct {
	Name  string   `json:"name"`
	Site  string   `json:"site"`
	Rule  waf.Rule `json:"rule"`
	OldID string   `json:"oldId"`
}

type wafRuleDelReq struct {
	Name string   `json:"name"`
	Site string   `json:"site"`
	IDs  []string `json:"ids"`
}

type wafApplyReq struct {
	Keys []string `json:"keys"`
}

type wafEmpty struct{}

type wafBlockRemoveReq struct {
	IPs []string `json:"ips"`
}

// wafCSV streams a generated CSV file back to the browser.
func wafCSV(name string, build func(s *waf.Store, c *gin.Context) ([]byte, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		s, err := wafStore()
		if err != nil {
			helper.InternalServer(c, err)
			return
		}
		data, err := build(s, c)
		if err != nil {
			helper.InternalServer(c, err)
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.csv"`, name, time.Now().Format("20060102150405")))
		c.Data(200, "text/csv; charset=utf-8", data)
	}
}

func (a *WafRouter) InitRouter(Router *gin.RouterGroup) {
	g := Router.Group("xpack/waf")
	g.GET("/overview", wafHandle(func(s *waf.Store, _ wafEmpty) (interface{}, error) {
		return waf.LoadOverview(s.OpenrestyPath)
	}))
	g.POST("/logs/search", wafHandle(func(s *waf.Store, req waf.LogQuery) (interface{}, error) {
		return waf.SearchAttackLogs(s.OpenrestyPath, req)
	}))
	g.POST("/blocks/search", wafHandle(func(s *waf.Store, req waf.BlockQuery) (interface{}, error) {
		return waf.ListBlocks(s, req)
	}))
	g.POST("/blocks/remove", wafHandle(func(s *waf.Store, req wafBlockRemoveReq) (interface{}, error) {
		return nil, waf.RemoveBlocks(s, req.IPs)
	}))
	g.POST("/blocks/clear", wafHandle(func(s *waf.Store, _ wafEmpty) (interface{}, error) {
		return nil, waf.ClearBlocks(s)
	}))
	g.POST("/blocks/export", wafCSV("waf-block", func(s *waf.Store, c *gin.Context) ([]byte, error) {
		var req waf.BlockQuery
		_ = c.ShouldBindJSON(&req)
		req.Page, req.Size = 1, 200
		page, err := waf.ListBlocks(s, req)
		if err != nil {
			return nil, err
		}
		return waf.ExportBlocks(page.Items)
	}))
	g.GET("/logs/options", wafHandle(func(s *waf.Store, _ wafEmpty) (interface{}, error) {
		return waf.LoadLogOptions(s.OpenrestyPath)
	}))
	g.POST("/logs/clear", wafHandle(func(s *waf.Store, _ wafEmpty) (interface{}, error) {
		return nil, waf.ClearLogs(s.OpenrestyPath)
	}))
	g.POST("/logs/export", wafCSV("waf-log", func(s *waf.Store, c *gin.Context) ([]byte, error) {
		var req waf.LogQuery
		_ = c.ShouldBindJSON(&req)
		return waf.ExportLogs(s.OpenrestyPath, req)
	}))
	g.POST("/report", wafHandle(func(s *waf.Store, req waf.LogQuery) (interface{}, error) {
		return waf.LoadReport(s.OpenrestyPath, req)
	}))
	g.GET("/global",wafHandle(func(s *waf.Store, _ wafEmpty) (interface{}, error) {
		return s.GetGlobal()
	}))
	g.POST("/global/update", wafHandle(func(s *waf.Store, req wafPatchReq) (interface{}, error) {
		return nil, s.UpdateGlobal(req.Patch)
	}))
	g.GET("/sites", wafHandle(func(s *waf.Store, _ wafEmpty) (interface{}, error) {
		sites, err := s.ListSites()
		if err != nil {
			return nil, err
		}
		webRepo := repo.NewIWebsiteRepo()
		for i := range sites {
			if site, err := webRepo.GetFirst(webRepo.WithAlias(sites[i].Key)); err == nil {
				sites[i].Remark = site.Remark
			}
		}
		return sites, nil
	}))
	g.POST("/global/apply", wafHandle(func(s *waf.Store, req wafApplyReq) (interface{}, error) {
		return nil, s.ApplyToSites(req.Keys)
	}))
	g.POST("/site", wafHandle(func(s *waf.Store, req wafPatchReq) (interface{}, error) {
		return s.GetSite(req.Key)
	}))
	g.POST("/site/update", wafHandle(func(s *waf.Store, req wafPatchReq) (interface{}, error) {
		return nil, s.UpdateSite(req.Key, req.Patch)
	}))
	g.POST("/rules/search", wafHandle(func(s *waf.Store, req wafRuleListReq) (interface{}, error) {
		return s.ListRules(req.Name, req.Site, req.Search, req.Page, req.Size)
	}))
	g.POST("/rules/save", wafHandle(func(s *waf.Store, req wafRuleSaveReq) (interface{}, error) {
		return nil, s.SaveRule(req.Name, req.Site, req.Rule, req.OldID)
	}))
	g.POST("/rules/delete", wafHandle(func(s *waf.Store, req wafRuleDelReq) (interface{}, error) {
		return nil, s.DeleteRules(req.Name, req.Site, req.IDs)
	}))
}
