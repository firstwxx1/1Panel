//go:build xpack || enterprise

package router

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/1Panel-dev/1Panel/core/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/core/middleware"
	"github.com/1Panel-dev/1Panel/core/xpack/node"
	"github.com/gin-gonic/gin"
)

type NodeRouter struct{}

func masterURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, c.Request.Host)
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
		s, err := node.JoinScript(masterURL(c), c.Query("token"))
		if err != nil {
			c.String(http.StatusForbidden, err.Error())
			return
		}
		c.String(http.StatusOK, s)
	})
	join.GET("/agent", func(c *gin.Context) {
		p, err := node.JoinAgentPath(c.Query("token"))
		if err != nil {
			c.String(http.StatusForbidden, err.Error())
			return
		}
		c.File(p)
	})
	join.GET("/bundle", func(c *gin.Context) {
		addr := c.Query("addr")
		if addr == "" {
			addr = c.ClientIP()
		}
		s, err := node.JoinBundle(c.Query("token"), addr)
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
		list, err := node.List()
		nodeDone(c, list, err)
	})

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
	x.POST("nodes/command", func(c *gin.Context) {
		var req struct {
			Token string `json:"token"`
		}
		if !nodeBind(c, &req) {
			return
		}
		cmd := fmt.Sprintf(`curl -fsSLk "%s/api/v2/core/nodejoin/script?token=%s" | bash`, masterURL(c), strings.TrimSpace(req.Token))
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
