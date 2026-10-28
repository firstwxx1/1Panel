//go:build xpack || enterprise

package router

import (
	"errors"
	"path"
	"sync"

	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/app/service"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/xpack/tamper"
	"github.com/gin-gonic/gin"
)

type TamperRouter struct{}

var errTamperSite = errors.New("website not found")

func tamperStore() (*tamper.Store, error) {
	return tamper.Open(path.Join(global.Dir.DataDir, "tamper"))
}

type tamperSite struct{ alias, domain, root string }

func tamperSites() []tamperSite {
	list, err := repo.NewIWebsiteRepo().List()
	if err != nil {
		return nil
	}
	var res []tamperSite
	for _, w := range list {
		if w.Type == "stream" || w.Alias == "" {
			continue
		}
		res = append(res, tamperSite{w.Alias, w.PrimaryDomain, path.Join(service.GetSiteDir(w.Alias), "index")})
	}
	return res
}

func tamperRoot(alias string) (string, bool) {
	for _, s := range tamperSites() {
		if s.alias == alias {
			return s.root, true
		}
	}
	return "", false
}

var tamperOnce sync.Once

func StartTamper() {
	tamperOnce.Do(func() {
		s, err := tamperStore()
		if err != nil {
			return
		}
		go s.Run(func() map[string]string {
			m := map[string]string{}
			for _, w := range tamperSites() {
				m[w.alias] = w.root
			}
			return m
		})
	})
}

func tamperHandle[T any](fn func(s *tamper.Store, req T) (interface{}, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req T
		if c.Request.Method == "POST" {
			if err := c.ShouldBindJSON(&req); err != nil {
				helper.BadRequest(c, err)
				return
			}
		}
		s, err := tamperStore()
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

type tamperEmpty struct{}

type tamperSiteView struct {
	Alias      string   `json:"alias"`
	Domain     string   `json:"domain"`
	Root       string   `json:"root"`
	Enabled    bool     `json:"enabled"`
	Restore    bool     `json:"restore"`
	Excludes   []string `json:"excludes"`
	Files      int      `json:"files"`
	BaselineAt string   `json:"baselineAt"`
	Pending    int      `json:"pending"`
}

func (a *TamperRouter) InitRouter(Router *gin.RouterGroup) {
	StartTamper()
	g := Router.Group("xpack/tamper")
	g.GET("/settings", tamperHandle(func(s *tamper.Store, _ tamperEmpty) (interface{}, error) {
		cfg := s.Config()
		views := []tamperSiteView{}
		for _, w := range tamperSites() {
			sc := cfg.Sites[w.alias]
			v := tamperSiteView{Alias: w.alias, Domain: w.domain, Root: w.root, Enabled: sc.Enabled, Restore: sc.Restore,
				Excludes: sc.Excludes, Files: sc.Files, Pending: s.EventCount(w.alias)}
			if v.Excludes == nil {
				v.Excludes = []string{}
			}
			if !sc.BaselineAt.IsZero() {
				v.BaselineAt = sc.BaselineAt.Format("2006-01-02 15:04:05")
			}
			views = append(views, v)
		}
		return map[string]interface{}{"enabled": cfg.Enabled, "interval": cfg.Interval, "sites": views}, nil
	}))
	g.POST("/settings", tamperHandle(func(s *tamper.Store, req struct {
		Enabled  bool `json:"enabled"`
		Interval int  `json:"interval"`
	}) (interface{}, error) {
		return nil, s.SetGlobal(req.Enabled, req.Interval)
	}))
	g.POST("/site", tamperHandle(func(s *tamper.Store, req struct {
		Alias    string   `json:"alias"`
		Enabled  bool     `json:"enabled"`
		Restore  bool     `json:"restore"`
		Excludes []string `json:"excludes"`
	}) (interface{}, error) {
		root, ok := tamperRoot(req.Alias)
		if !ok {
			return nil, errTamperSite
		}
		if err := s.SetSite(req.Alias, req.Enabled, req.Restore, req.Excludes); err != nil {
			return nil, err
		}
		if req.Enabled && !s.HasBaseline(req.Alias) { // first protection: take the baseline now
			if _, err := s.Rebuild(req.Alias, root); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}))
	g.POST("/rebuild", tamperHandle(func(s *tamper.Store, req struct {
		Alias string `json:"alias"`
	}) (interface{}, error) {
		root, ok := tamperRoot(req.Alias)
		if !ok {
			return nil, errTamperSite
		}
		n, err := s.Rebuild(req.Alias, root)
		if err != nil {
			return nil, err
		}
		return map[string]int{"files": n}, nil
	}))
	g.POST("/scan", tamperHandle(func(s *tamper.Store, req struct {
		Alias string `json:"alias"`
	}) (interface{}, error) {
		root, ok := tamperRoot(req.Alias)
		if !ok {
			return nil, errTamperSite
		}
		return map[string]int{"found": len(s.Scan(req.Alias, root))}, nil
	}))
	g.POST("/events/search", tamperHandle(func(s *tamper.Store, req struct {
		Site     string `json:"site"`
		Page     int    `json:"page"`
		PageSize int    `json:"pageSize"`
	}) (interface{}, error) {
		items, total := s.Events(req.Site, req.Page, req.PageSize)
		if items == nil {
			items = []tamper.Event{}
		}
		return map[string]interface{}{"items": items, "total": total}, nil
	}))
	g.POST("/events/clear", tamperHandle(func(s *tamper.Store, _ tamperEmpty) (interface{}, error) {
		s.ClearEvents()
		return nil, nil
	}))
	// polled by the master's alert engine
	g.GET("/events/since", func(c *gin.Context) {
		s, err := tamperStore()
		if err != nil {
			helper.InternalServer(c, err)
			return
		}
		var id int64
		for _, ch := range c.Query("id") {
			if ch < '0' || ch > '9' {
				id = 0
				break
			}
			id = id*10 + int64(ch-'0')
		}
		items, last := s.Since(id)
		if items == nil {
			items = []tamper.Event{}
		}
		helper.SuccessWithData(c, map[string]interface{}{"items": items, "last": last})
	})
}
