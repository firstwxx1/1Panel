//go:build xpack || enterprise

package router

import (
	"os"
	"path/filepath"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/agent/app/service"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/xpack/appsync"
	"github.com/gin-gonic/gin"
)

// NodeRouter holds the endpoints a master uses to manage a child node's agent.
type NodeRouter struct{}

func (a *NodeRouter) InitRouter(Router *gin.RouterGroup) {
	g := Router.Group("xpack/node")
	g.GET("/ping", func(c *gin.Context) {
		helper.SuccessWithData(c, map[string]interface{}{"master": global.IsMaster, "version": global.CONF.Base.Version})
	})
	appsync.Register(g)
	go service.EnsureLocalSSH()
	g.POST("/restart", func(c *gin.Context) {
		helper.Success(c)
		go func() {
			time.Sleep(time.Second)
			os.Exit(1) // non-zero so systemd brings the agent back
		}()
	})
	// unbind forgets the master: certificates and node config are removed and the agent
	// exits (systemd does not restart a clean exit), so the old master can no longer connect
	g.POST("/unbind", func(c *gin.Context) {
		if global.IsMaster {
			helper.BadRequest(c, os.ErrPermission)
			return
		}
		_ = os.RemoveAll(filepath.Join("/etc/1panel", "node"))
		_ = os.Remove("/etc/1panel/node.json")
		helper.Success(c)
		go func() {
			time.Sleep(time.Second)
			os.Exit(0)
		}()
	})
}
