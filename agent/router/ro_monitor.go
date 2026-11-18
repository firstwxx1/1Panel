//go:build xpack || enterprise

package router

import (
	"errors"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/app/service"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/xpack/monitor"
	"github.com/1Panel-dev/1Panel/agent/xpack/waf"
	"github.com/gin-gonic/gin"
)

type MonitorRouter struct{}

func monitorStore() (*monitor.Store, error) {
	return monitor.Open(path.Join(global.Dir.DataDir, "monitor"))
}

func monitorSites() []monitor.Site {
	list, err := repo.NewIWebsiteRepo().List()
	if err != nil {
		return nil
	}
	var res []monitor.Site
	for _, w := range list {
		if w.Type == "stream" || w.Alias == "" {
			continue
		}
		res = append(res, monitor.Site{Alias: w.Alias, LogPath: path.Join(service.GetSiteDir(w.Alias), "log", "access.log")})
	}
	return res
}

func monitorGeoPaths() []string {
	app, err := repo.NewIAppRepo().GetFirst(repo.NewIAppRepo().WithKey(constant.AppOpenresty))
	if err != nil || app.ID == 0 {
		return nil
	}
	install, err := repo.NewIAppInstallRepo().GetFirst(repo.NewIAppInstallRepo().WithAppId(app.ID))
	if err != nil || install.ID == 0 {
		return nil
	}
	return []string{path.Join(install.GetPath(), "1pwaf", "data", "geo", "GeoIP.mmdb")}
}

var monitorOnce sync.Once

// StartMonitor launches the access-log collector once per process.
func StartMonitor() {
	monitorOnce.Do(func() {
		s, err := monitorStore()
		if err != nil {
			return
		}
		c := &monitor.Collector{Store: s, Sites: monitorSites, GeoPaths: monitorGeoPaths}
		go c.Run()
	})
}

func monitorHandle[T any](fn func(s *monitor.Store, req T) (interface{}, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req T
		if c.Request.Method == "POST" {
			if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
				helper.BadRequest(c, err)
				return
			}
		}
		s, err := monitorStore()
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

type monitorSiteItem struct {
	Alias   string `json:"alias"`
	Domain  string `json:"domain"`
	Enabled bool   `json:"enabled"`
}

type monitorSettings struct {
	monitor.Config
	Sites2 []monitorSiteItem `json:"siteList"`
	HasRT  bool              `json:"hasRt"`
}

func openrestyConf() (string, *monitorNginx, error) {
	app, err := repo.NewIAppRepo().GetFirst(repo.NewIAppRepo().WithKey(constant.AppOpenresty))
	if err != nil {
		return "", nil, err
	}
	install, err := repo.NewIAppInstallRepo().GetFirst(repo.NewIAppInstallRepo().WithAppId(app.ID))
	if err != nil {
		return "", nil, err
	}
	return path.Join(install.GetPath(), "conf", "nginx.conf"), &monitorNginx{container: install.ContainerName}, nil
}

type monitorNginx struct{ container string }

var (
	logFormatMainRe       = regexp.MustCompile(`(?s)\blog_format\s+main\b[^;]*;`)
	logFormatTrailingQuRe = regexp.MustCompile(`(['"])(\s*;)\z`)
)

func monitorHasRT() bool {
	file, _, err := openrestyConf()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	m := logFormatMainRe.FindString(string(data))
	return m != "" && strings.Contains(m, "$request_time")
}

func writeFileAtomic(file string, data []byte, perm os.FileMode) error {
	if perm == 0 {
		perm = 0o644
	}
	tmp := file + ".monitor-tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

func monitorSettingsView(s *monitor.Store) monitorSettings {
	cfg := s.Config()
	res := monitorSettings{Config: cfg, HasRT: monitorHasRT()}
	list, _ := repo.NewIWebsiteRepo().List()
	for _, w := range list {
		if w.Type == "stream" || w.Alias == "" {
			continue
		}
		res.Sites2 = append(res.Sites2, monitorSiteItem{Alias: w.Alias, Domain: w.PrimaryDomain, Enabled: cfg.Sites[w.Alias]})
	}
	return res
}

type monitorSiteReq struct {
	Site string `json:"site"`
}

type monitorEmpty struct{}

func (a *MonitorRouter) InitRouter(Router *gin.RouterGroup) {
	StartMonitor()
	g := Router.Group("xpack/monitor")
	g.GET("/settings", monitorHandle(func(s *monitor.Store, _ monitorEmpty) (interface{}, error) {
		return monitorSettingsView(s), nil
	}))
	g.POST("/settings", monitorHandle(func(s *monitor.Store, req monitor.Config) (interface{}, error) {
		old := s.Config()
		for alias, on := range old.Sites {
			if on && !req.Sites[alias] || old.Enabled && !req.Enabled {
				s.ForgetOffset(alias)
			}
		}
		return nil, s.SaveConfig(req)
	}))
	g.POST("/rt/enable", monitorHandle(func(s *monitor.Store, _ monitorEmpty) (interface{}, error) {
		file, ng, err := openrestyConf()
		if err != nil {
			return nil, err
		}
		perm := os.FileMode(0o644)
		if st, err := os.Stat(file); err == nil {
			perm = st.Mode().Perm()
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		text := string(data)
		loc := logFormatMainRe.FindStringIndex(text)
		if loc == nil {
			return nil, errors.New("log_format main directive not found in nginx.conf")
		}
		block := text[loc[0]:loc[1]]
		if strings.Contains(block, "$request_time") {
			return nil, nil
		}
		if !logFormatTrailingQuRe.MatchString(block) {
			return nil, errors.New("unsupported log_format main format in nginx.conf")
		}
		newBlock := logFormatTrailingQuRe.ReplaceAllString(block, " $$request_time$1$2")
		updated := text[:loc[0]] + newBlock + text[loc[1]:]

		if err := writeFileAtomic(file+".bak-monitor", data, perm); err != nil {
			return nil, err
		}
		if err := writeFileAtomic(file, []byte(updated), perm); err != nil {
			return nil, err
		}
		if err := nginxReload(ng.container); err != nil {
			_ = writeFileAtomic(file, data, perm)
			return nil, err
		}
		return nil, nil
	}))
	g.POST("/clear", monitorHandle(func(s *monitor.Store, req monitorSiteReq) (interface{}, error) {
		return nil, s.Clear(req.Site)
	}))
	g.POST("/overview", monitorHandle(func(s *monitor.Store, req monitor.RangeReq) (interface{}, error) {
		return s.Overview(req)
	}))
	g.POST("/distribution", monitorHandle(func(s *monitor.Store, req monitor.DistReq) (interface{}, error) {
		return s.Distribution(req)
	}))
	g.POST("/errors", monitorHandle(func(s *monitor.Store, req monitor.ErrReq) (interface{}, error) {
		return s.Errors(req)
	}))
	g.POST("/realtime", monitorHandle(func(s *monitor.Store, req monitor.RealtimeReq) (interface{}, error) {
		res, err := s.Realtime(req)
		if err != nil {
			return nil, err
		}
		var size int64
		for _, site := range monitorSites() {
			if req.Site != "" && req.Site != site.Alias {
				continue
			}
			if st, err := os.Stat(site.LogPath); err == nil {
				size += st.Size()
			}
		}
		res["logSize"] = size
		return res, nil
	}))
	g.POST("/live", monitorHandle(func(s *monitor.Store, req monitorSiteReq) (interface{}, error) {
		return s.Live(req.Site)
	}))
	g.POST("/sites", monitorHandle(func(s *monitor.Store, req monitor.RangeReq) (interface{}, error) {
		view := monitorSettingsView(s)
		var aliases []string
		for _, it := range view.Sites2 {
			aliases = append(aliases, it.Alias)
		}
		totals, err := s.SiteTotals(aliases, req)
		if err != nil {
			return nil, err
		}
		type row struct {
			Alias   string `json:"alias"`
			Domain  string `json:"domain"`
			Enabled bool   `json:"enabled"`
			monitor.Totals
		}
		rows := []row{}
		for _, it := range view.Sites2 {
			rows = append(rows, row{Alias: it.Alias, Domain: it.Domain, Enabled: it.Enabled, Totals: totals[it.Alias]})
		}
		return rows, nil
	}))
}

func nginxReload(container string) error {
	return (&waf.Store{Container: container}).Reload()
}
