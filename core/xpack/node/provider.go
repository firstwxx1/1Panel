package node

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/init/proxy"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/1Panel-dev/1Panel/core/utils/ssh"
	"github.com/gin-gonic/gin"
)

// Provider is the xpack multi-node implementation: it forwards a request to the
// child agent named by the CurrentNode header over mutual TLS.
type Provider struct {
	once      sync.Once
	mu        sync.Mutex
	transport *http.Transport
}

func NewProvider() *Provider { return &Provider{} }

// StartBackground begins the node status collector and alert engine; safe to call repeatedly.
func (p *Provider) StartBackground() {
	p.once.Do(func() {
		go collectLoop()
		go alertLoop()
	})
}

func (p *Provider) agentTransport() (*http.Transport, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.transport != nil {
		return p.transport, nil
	}
	a, err := loadAuthority()
	if err != nil {
		return nil, err
	}
	pair, err := tls.X509KeyPair(a.clientCrt, a.clientKey)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(a.caPEM)
	p.transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{pair},
			RootCAs:      pool,
			MinVersion:   tls.VersionTLS12,
		},
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     60 * time.Second,
	}
	return p.transport, nil
}

func fail(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"code": code, "message": msg})
	c.Abort()
}

func (p *Provider) Proxy(c *gin.Context, currentNode string) {
	p.StartBackground()
	path := c.Request.URL.Path
	// all /api/v2/core/xpack/* endpoints are served by the master's core router
	if strings.HasPrefix(path, "/api/v2/core/xpack/") || path == "/api/v2/core/xpack" {
		c.Next()
		return
	}
	if currentNode == "" || currentNode == "local" {
		defer func() {
			if err := recover(); err != nil && err != http.ErrAbortHandler {
				global.LOG.Debug(err)
			}
		}()
		proxy.LocalAgentProxy.ServeHTTP(c.Writer, c.Request)
		c.Abort()
		return
	}
	if strings.HasPrefix(path, "/api/v2/core") {
		c.Next()
		return
	}
	var n model.Node
	if err := global.DB.Where("name = ? AND kind = ?", currentNode, kindNode).First(&n).Error; err != nil {
		fail(c, http.StatusBadGateway, "node not found: "+currentNode)
		return
	}
	if n.Addr == "" || n.Status == statusPending {
		fail(c, http.StatusBadGateway, "node is not ready: "+currentNode)
		return
	}
	tr, err := p.agentTransport()
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	target := net.JoinHostPort(n.Addr, fmt.Sprint(n.Port))
	rp := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme = "https"
			r.Out.URL.Host = target
			r.Out.Host = target
			r.Out.Header.Set("Proxy-Id", n.ProxyID)
			r.SetXForwarded()
		},
		Transport: tr,
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			rw.WriteHeader(http.StatusBadGateway)
			_, _ = rw.Write([]byte("Bad Gateway: " + err.Error()))
		},
	}
	defer func() {
		if err := recover(); err != nil && err != http.ErrAbortHandler {
			global.LOG.Debug(err)
		}
	}()
	rp.ServeHTTP(c.Writer, c.Request)
	c.Abort()
}

func (p *Provider) ProxyDocker(proxyURL string) error { return nil }

func (p *Provider) UpdateGroup(name string, group, newGroup uint) error {
	if name != "node" {
		return nil
	}
	return global.DB.Model(&model.Node{}).Where("group_id = ?", group).Update("group_id", newGroup).Error
}

func (p *Provider) CheckBackupUsed(name string) error {
	if global.DB == nil {
		return nil
	}
	targets := []string{"local"}
	var nodes []model.Node
	if err := global.DB.Where("kind = ? AND status = ?", kindNode, statusHealthy).Find(&nodes).Error; err == nil {
		for _, n := range nodes {
			if n.Addr != "" {
				targets = append(targets, n.Name)
			}
		}
	}
	for _, target := range targets {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := agentJSONCtx(ctx, target, http.MethodPost, "/xpack/node/check-backup-used", map[string]string{"name": name}, nil)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) LoadNodeInfo(currentNode string) (*ssh.ConnInfo, string, error) {
	return nil, "", fmt.Errorf("SSH credentials are not stored on master for node %q", currentNode)
}

func (p *Provider) Sync(dataType string) error {
	if global.DB == nil {
		return nil
	}
	payload := map[string]interface{}{}
	if dataType == constant.SyncBackupAccounts || dataType == "" {
		var backups []model.BackupAccount
		if err := global.DB.Where("is_public = ?", true).Find(&backups).Error; err != nil {
			return err
		}
		for i := range backups {
			if backups[i].AccessKey != "" {
				if plain, err := encrypt.StringDecrypt(backups[i].AccessKey); err == nil {
					backups[i].AccessKey = plain
				}
			}
			if backups[i].Credential != "" {
				if plain, err := encrypt.StringDecrypt(backups[i].Credential); err == nil {
					backups[i].Credential = plain
				}
			}
		}
		payload["backupAccounts"] = backups
	}
	if dataType == constant.SyncScripts || dataType == "" {
		var scripts []model.ScriptLibrary
		if err := global.DB.Where("is_interactive = ?", false).Find(&scripts).Error; err != nil {
			return err
		}
		items := make([]map[string]interface{}, 0, len(scripts))
		for _, s := range scripts {
			items = append(items, map[string]interface{}{
				"name":   s.Name,
				"script": s.Script,
			})
		}
		payload["scripts"] = items
	}
	if len(payload) == 0 {
		return nil
	}

	targets := []string{"local"}
	var nodes []model.Node
	if err := global.DB.Where("kind = ? AND status = ?", kindNode, statusHealthy).Find(&nodes).Error; err == nil {
		for _, n := range nodes {
			if n.Addr != "" {
				targets = append(targets, n.Name)
			}
		}
	}
	var errs []string
	for _, target := range targets {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := agentJSONCtx(ctx, target, http.MethodPost, "/xpack/node/sync-meta", payload, nil)
		cancel()
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", target, err))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (p *Provider) AutoUpgradeWithMaster() {
	go func() {
		if _, err := UpgradeNodes(nil); err != nil {
			global.LOG.Warnf("auto upgrade child nodes failed: %v", err)
		}
	}()
}

// RevokeChildTerminals fans out terminal session revocation to all healthy child nodes.
func RevokeChildTerminals(scope, userID, authSessionID string) error {
	if global.DB == nil {
		return nil
	}
	var nodes []model.Node
	if err := global.DB.Where("kind = ? AND status = ?", kindNode, statusHealthy).Find(&nodes).Error; err != nil {
		return nil
	}
	body := map[string]string{
		"scope":         scope,
		"userId":        userID,
		"authSessionId": authSessionID,
	}
	var errs []string
	for _, n := range nodes {
		if n.Addr == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := agentJSONCtx(ctx, n.Name, http.MethodPost, "/internal/terminal/sessions/revoke", body, nil)
		cancel()
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", n.Name, err))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (p *Provider) LoadRequestTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: (&net.Dialer{
			Timeout:   60 * time.Second,
			KeepAlive: 60 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       15 * time.Second,
	}
}

// resetTransport drops pooled connections after a node is removed or rotated.
func (p *Provider) resetTransport() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.transport != nil {
		p.transport.CloseIdleConnections()
	}
}

// Default is the single provider instance shared by the router and the helper.
var Default = NewProvider()
