//go:build xpack || enterprise

package router

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/1Panel-dev/1Panel/core/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/core/middleware"
	"github.com/1Panel-dev/1Panel/core/xpack/node"
	"github.com/gin-gonic/gin"
)

type NodeRouter struct{}

var (
	safeHostRule  = regexp.MustCompile(`^[A-Za-z0-9.\-]+$`)
	joinTokenRule = regexp.MustCompile(`^[A-Za-z0-9_-]{16,80}$`)
)

func validHostHeader(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("missing host header")
	}
	for _, r := range raw {
		if r <= 0x20 || r == 0x7f || strings.ContainsRune("/?#@'\"`$;&|<>\\()", r) {
			return "", errors.New("invalid host header")
		}
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		host = raw
		if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
			inner := host[1 : len(host)-1]
			if net.ParseIP(inner) == nil {
				return "", errors.New("invalid ipv6 host")
			}
			return host, nil
		}
		if net.ParseIP(host) == nil && !safeHostRule.MatchString(host) {
			return "", errors.New("invalid host header")
		}
		return host, nil
	}
	p, pErr := strconv.Atoi(port)
	if pErr != nil || p < 1 || p > 65535 {
		return "", errors.New("invalid port in host header")
	}
	if net.ParseIP(host) == nil && !safeHostRule.MatchString(host) {
		return "", errors.New("invalid host in host header")
	}
	return net.JoinHostPort(host, port), nil
}

func masterURL(c *gin.Context) (string, error) {
	host, err := validHostHeader(c.Request.Host)
	if err != nil {
		return "", err
	}
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, host), nil
}

func validJoinToken(raw string) (string, error) {
	tok := strings.TrimSpace(raw)
	if !joinTokenRule.MatchString(tok) {
		return "", errors.New("invalid token")
	}
	return tok, nil
}

func nodeBind(c *gin.Context, req interface{}) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		helper.BadRequest(c, err)
		return false
	}
	return true
}

func nodeDone(c *gin.Context, data interface{}, err error) {
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	if data == nil {
		helper.Success(c)
		return
	}
	helper.SuccessWithData(c, data)
}

type idsReq struct {
	IDs     []uint `json:"ids"`
	GroupID uint   `json:"groupID"`
}

type idReq struct {
	ID         uint `json:"id"`
	IsFavorite bool `json:"isFavorite"`
}

func (r *NodeRouter) InitRouter(Router *gin.RouterGroup) {
	// public, token authenticated; consumed by `curl | bash` on a joining machine
	join := Router.Group("nodejoin")
	join.GET("/script", func(c *gin.Context) {
		tok, err := validJoinToken(c.Query("token"))
		if err != nil {
			c.String(http.StatusForbidden, err.Error())
			return
		}
		mURL, err := masterURL(c)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}
		s, err := node.JoinScript(mURL, tok)
		if err != nil {
			c.String(http.StatusForbidden, err.Error())
			return
		}
		c.String(http.StatusOK, s)
	})
	join.GET("/agent", func(c *gin.Context) {
		tok, err := validJoinToken(c.Query("token"))
		if err != nil {
			c.String(http.StatusForbidden, err.Error())
			return
		}
		p, err := node.JoinAgentPath(tok)
		if err != nil {
			c.String(http.StatusForbidden, err.Error())
			return
		}
		c.File(p)
	})
	join.GET("/bundle", func(c *gin.Context) {
		tok, err := validJoinToken(c.Query("token"))
		if err != nil {
			c.String(http.StatusForbidden, err.Error())
			return
		}
		addr := c.Query("addr")
		if addr == "" {
			addr = c.ClientIP()
		}
		s, err := node.JoinBundle(tok, addr)
		if err != nil {
			c.String(http.StatusForbidden, err.Error())
			return
		}
		c.String(http.StatusOK, s)
	})

	auth := Router.Group("").Use(middleware.SessionAuth()).Use(middleware.PasswordExpired())
	auth.POST("nodes/list", func(c *gin.Context) {
		list, err := node.List()
		nodeDone(c, list, err)
	})
	auth.GET("nodes/simple/all", func(c *gin.Context) {
		list, err := node.ListSimplePanels()
		nodeDone(c, list, err)
	})
	licenseOK := func(c *gin.Context) {
		helper.SuccessWithData(c, gin.H{"status": "Bound", "productPro": "253402300799"})
	}
	auth.GET("licenses/status", licenseOK)
	auth.GET("licenses/master/status", licenseOK)

	x := Router.Group("xpack").Use(middleware.SessionAuth()).Use(middleware.PasswordExpired())
	for _, kind := range []string{"nodes", "panels"} {
		kind := kind
		k := "node"
		if kind == "panels" {
			k = "panel"
		}
		x.POST(kind+"/search", func(c *gin.Context) {
			var req node.SearchReq
			if !nodeBind(c, &req) {
				return
			}
			req.Kind = k
			res, err := node.Search(req)
			nodeDone(c, res, err)
		})
		x.POST(kind+"/create", func(c *gin.Context) {
			var req node.CreateReq
			if !nodeBind(c, &req) {
				return
			}
			req.Kind = k
			res, err := node.Create(req)
			nodeDone(c, res, err)
		})
		x.POST(kind+"/update", func(c *gin.Context) {
			var req node.UpdateReq
			if !nodeBind(c, &req) {
				return
			}
			nodeDone(c, nil, node.Update(req))
		})
		x.POST(kind+"/del", func(c *gin.Context) {
			var req idsReq
			if !nodeBind(c, &req) {
				return
			}
			nodeDone(c, nil, node.Delete(req.IDs))
		})
		x.POST(kind+"/group", func(c *gin.Context) {
			var req idsReq
			if !nodeBind(c, &req) {
				return
			}
			nodeDone(c, nil, node.ChangeGroup(req.IDs, req.GroupID))
		})
		x.POST(kind+"/test", func(c *gin.Context) {
			var req idReq
			if !nodeBind(c, &req) {
				return
			}
			nodeDone(c, nil, node.Test(req.ID))
		})
	}
	x.POST("nodes/favorite", func(c *gin.Context) {
		var req idReq
		if !nodeBind(c, &req) {
			return
		}
		nodeDone(c, nil, node.SetFavorite(req.ID, req.IsFavorite))
	})
	x.POST("nodes/restart", func(c *gin.Context) {
		var req idReq
		if !nodeBind(c, &req) {
			return
		}
		nodeDone(c, nil, node.Restart(req.ID))
	})
	x.POST("nodes/upgrade", func(c *gin.Context) {
		var req struct {
			IDs []uint `json:"ids"`
		}
		if !nodeBind(c, &req) {
			return
		}
		res, err := node.UpgradeNodes(req.IDs)
		if err != nil {
			helper.InternalServer(c, err)
			return
		}
		if res == nil {
			res = []node.UpgradeNodeResult{}
		}
		helper.SuccessWithData(c, gin.H{"results": res})
	})
	x.POST("nodes/join-cmd", func(c *gin.Context) {
		var req struct {
			ID uint `json:"id"`
		}
		if !nodeBind(c, &req) {
			return
		}
		mURL, err := masterURL(c)
		if err != nil {
			helper.BadRequest(c, err)
			return
		}
		cmd, err := node.GetJoinCommandByID(req.ID, mURL)
		if err != nil {
			helper.InternalServer(c, err)
			return
		}
		helper.SuccessWithData(c, gin.H{"command": cmd})
	})
	x.POST("nodes/command", func(c *gin.Context) {
		var req struct {
			Token string `json:"token"`
		}
		if !nodeBind(c, &req) {
			return
		}
		tok, err := validJoinToken(req.Token)
		if err != nil {
			helper.BadRequest(c, err)
			return
		}
		mURL, err := masterURL(c)
		if err != nil {
			helper.BadRequest(c, err)
			return
		}
		cmd := fmt.Sprintf("curl -fsSLk %q | bash", mURL+"/api/v2/core/nodejoin/script?token="+url.QueryEscape(tok))
		helper.SuccessWithData(c, cmd)
	})
	// resource sync: the master streams data between two nodes' agents
	x.POST("sync/files", func(c *gin.Context) {
		var req node.SyncFileReq
		if !nodeBind(c, &req) {
			return
		}
		t, err := node.SyncFiles(req)
		nodeDone(c, t, err)
	})
	x.POST("sync/images", func(c *gin.Context) {
		var req node.SyncImageReq
		if !nodeBind(c, &req) {
			return
		}
		t, err := node.SyncImages(req)
		nodeDone(c, t, err)
	})
	x.POST("sync/certs", func(c *gin.Context) {
		var req node.SyncCertReq
		if !nodeBind(c, &req) {
			return
		}
		t, err := node.SyncCerts(req)
		nodeDone(c, t, err)
	})
	x.POST("sync/apps", func(c *gin.Context) {
		var req node.SyncAppReq
		if !nodeBind(c, &req) {
			return
		}
		t, err := node.SyncApp(req)
		nodeDone(c, t, err)
	})
	// interface settings; search is public because the login page needs it
	Router.GET("xpack/settings/search", func(c *gin.Context) { helper.SuccessWithData(c, node.UIGetSettings()) })
	x.GET("settings/app/local-sync", func(c *gin.Context) { helper.SuccessWithData(c, gin.H{"enabled": false}) })
	x.POST("settings/update", func(c *gin.Context) {
		var req struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if !nodeBind(c, &req) {
			return
		}
		nodeDone(c, nil, node.UISetSetting(req.Key, req.Value))
	})
	x.POST("settings/upload", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 5<<20)
		fh, err := c.FormFile("file")
		if err != nil {
			helper.BadRequest(c, err)
			return
		}
		f, err := fh.Open()
		if err != nil {
			helper.BadRequest(c, err)
			return
		}
		defer f.Close()
		nodeDone(c, nil, node.UIUpload(c.PostForm("kind"), f))
	})
	x.GET("alert/config", func(c *gin.Context) { helper.SuccessWithData(c, node.GetAlertConfig()) })
	x.POST("alert/config", func(c *gin.Context) {
		var req node.AlertConfig
		if !nodeBind(c, &req) {
			return
		}
		nodeDone(c, nil, node.SaveAlertConfig(req))
	})
	x.POST("alert/telegram/test", func(c *gin.Context) {
		var req node.AlertTelegram
		if !nodeBind(c, &req) {
			return
		}
		nodeDone(c, nil, node.TestTelegram(req))
	})
	x.POST("alert/telegram/chats", func(c *gin.Context) {
		var req node.AlertTelegram
		if !nodeBind(c, &req) {
			return
		}
		res, err := node.DiscoverChatIDs(req)
		nodeDone(c, res, err)
	})
	x.GET("alert/records", func(c *gin.Context) { helper.SuccessWithData(c, node.AlertRecords()) })
	x.POST("alert/records/clear", func(c *gin.Context) {
		node.ClearAlertRecords()
		helper.Success(c)
	})
	x.GET("sync/tasks", func(c *gin.Context) { helper.SuccessWithData(c, node.SyncTasks()) })
	x.POST("sync/tasks/cancel", func(c *gin.Context) {
		var req struct {
			ID string `json:"id"`
		}
		if !nodeBind(c, &req) {
			return
		}
		nodeDone(c, nil, node.CancelSyncTask(req.ID))
	})
	x.POST("sync/tasks/del", func(c *gin.Context) {
		var req struct {
			IDs []string `json:"ids"`
		}
		if !nodeBind(c, &req) {
			return
		}
		node.DeleteSyncTasks(req.IDs)
		helper.Success(c)
	})
	// apps awaiting upgrade, used by the node switcher; none tracked yet
	x.GET("nodes/apps/update", func(c *gin.Context) { helper.SuccessWithData(c, []interface{}{}) })
}
