package middleware

import (
	"crypto/subtle"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/mcpapi"
	"github.com/1Panel-dev/1Panel/agent/utils/xpack"
	"github.com/gin-gonic/gin"
)

const nodeProxyIDPath = "/etc/1panel/.nodeProxyID"

var (
	proxyIDMu    sync.RWMutex
	proxyIDVal   string
	proxyIDMTime time.Time
)

var loadNodeProxyID = func() (string, error) {
	st, err := os.Stat(nodeProxyIDPath)
	if err != nil {
		return "", err
	}
	mtime := st.ModTime()
	proxyIDMu.RLock()
	if proxyIDVal != "" && mtime.Equal(proxyIDMTime) {
		v := proxyIDVal
		proxyIDMu.RUnlock()
		return v, nil
	}
	proxyIDMu.RUnlock()

	data, err := os.ReadFile(nodeProxyIDPath)
	if err != nil {
		return "", err
	}
	val := strings.TrimSpace(string(data))
	proxyIDMu.Lock()
	proxyIDVal = val
	proxyIDMTime = mtime
	proxyIDMu.Unlock()
	return val, nil
}

// isLocalSocket reports whether the request arrived over this machine's agent.sock.
func isLocalSocket(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	if addr, ok := c.Request.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && addr.Network() == "unix" {
		return true
	}
	remoteAddr := c.Request.RemoteAddr
	return remoteAddr == "@" || strings.HasSuffix(remoteAddr, ".sock")
}

func Certificate() gin.HandlerFunc {
	return func(c *gin.Context) {
		if global.IsMaster {
			c.Next()
			return
		}
		if c.Request.TLS == nil && isLocalSocket(c) {
			c.Next()
			return
		}
		if !xpack.MultiNodeProvider.ValidateCertificate(c) {
			CloseDirectly(c)
			return
		}
		mcpRequest, _ := mcpapi.Check(c.Request)
		mcpTransport := mcpRequest || c.Request.URL.Path == mcpapi.CapabilitiesPath
		masterProxyID := strings.TrimSpace(c.Request.Header.Get("Proxy-Id"))
		proxyID, err := loadNodeProxyID()
		proxyID = strings.TrimSpace(proxyID)
		if err != nil || proxyID == "" || masterProxyID == "" || subtle.ConstantTimeCompare([]byte(proxyID), []byte(masterProxyID)) != 1 {
			global.LOG.Warnf("rejected request from %s: invalid or missing proxy-id", c.Request.RemoteAddr)
			if mcpTransport {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			CloseDirectly(c)
			return
		}
		c.Request = mcpapi.BindVerifiedNodeTransport(c.Request)
		c.Next()
	}
}

func CloseDirectly(c *gin.Context) {
	hijacker, ok := c.Writer.(http.Hijacker)
	if !ok {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetLinger(0)
	}
	_ = conn.Close()
	c.Abort()
}
