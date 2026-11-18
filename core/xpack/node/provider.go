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
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/init/proxy"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/1Panel-dev/1Panel/core/utils/ssh"
	"github.com/1Panel-dev/1Panel/core/utils/xpack/providers"
	"github.com/gin-gonic/gin"
)

const (
	dockerSystemdDropInDir  = "/etc/systemd/system/docker.service.d"
	dockerSystemdDropInFile = "/etc/systemd/system/docker.service.d/http-proxy.conf"
)

var _ providers.MultiNodeProvider = (*Provider)(nil)

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
		FlushInterval: -1,
		Rewrite: func(r *httputil.ProxyRequest) {
			if r.In.Form == nil {
				r.Out.URL.RawQuery = r.In.URL.RawQuery
			}
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

func (p *Provider) ProxyDocker(proxyURL string) error {
	if global.DB == nil {
		return nil
	}
	status := constant.StatusDisable
	if strings.TrimSpace(proxyURL) != "" {
		status = constant.StatusEnable
	}
	if err := repo.NewISettingRepo().UpdateOrCreate("ProxyDocker", status); err != nil {
		return err
	}
	applyLocalDockerSystemdProxy(proxyURL)
	return nil
}

func GetProxyDockerStatus() string {
	if global.DB == nil {
		return constant.StatusDisable
	}
	val, err := repo.NewISettingRepo().GetValueByKey("ProxyDocker")
	if err != nil || strings.TrimSpace(val) != constant.StatusEnable {
		return constant.StatusDisable
	}
	return constant.StatusEnable
}

func applyLocalDockerSystemdProxy(proxyURL string) {
	if _, err := os.Stat("/etc/systemd/system"); err != nil {
		return
	}
	trimmed := strings.TrimSpace(proxyURL)
	if trimmed == "" {
		if _, err := os.Stat(dockerSystemdDropInFile); err != nil {
			return
		}
		_ = os.Remove(dockerSystemdDropInFile)
	} else {
		if err := os.MkdirAll(dockerSystemdDropInDir, 0o755); err != nil {
			return
		}
		escaped := strings.ReplaceAll(trimmed, `"`, `\"`)
		content := fmt.Sprintf("[Service]\nEnvironment=\"HTTP_PROXY=%s\"\nEnvironment=\"HTTPS_PROXY=%s\"\nEnvironment=\"NO_PROXY=localhost,127.0.0.1,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16\"\n", escaped, escaped)
		if err := os.WriteFile(dockerSystemdDropInFile, []byte(content), 0o644); err != nil {
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "systemctl", "daemon-reload").Run()
}

func buildDockerProxyURL(proxyType, proxyUrl, proxyPort, proxyUser, plainPasswd, proxyDocker string) string {
	if proxyDocker != constant.StatusEnable || proxyType == "" || proxyType == "close" || strings.TrimSpace(proxyUrl) == "" {
		return ""
	}
	var account string
	if proxyUser != "" {
		account = proxyUser
		if plainPasswd != "" {
			account += ":" + plainPasswd
		}
		account += "@"
	}
	host := strings.ReplaceAll(proxyUrl, proxyType+"://", "")
	if proxyPort != "" {
		return fmt.Sprintf("%s://%s%s:%s", proxyType, account, host, proxyPort)
	}
	return fmt.Sprintf("%s://%s%s", proxyType, account, host)
}

func (p *Provider) UpdateGroup(name string, group, newGroup uint) error {
	if global.DB == nil {
		return nil
	}
	switch name {
	case "node":
		return global.DB.Model(&model.Node{}).Where("group_id = ?", group).Update("group_id", newGroup).Error
	case "website", "cronjob":
		var targets []string
		if name == "cronjob" {
			targets = append(targets, "local")
		}
		var nodes []model.Node
		if err := global.DB.Where("kind = ? AND status = ?", kindNode, statusHealthy).Find(&nodes).Error; err == nil {
			for _, n := range nodes {
				if n.Addr != "" {
					targets = append(targets, n.Name)
				}
			}
		}
		if len(targets) == 0 {
			return nil
		}
		payload := map[string]interface{}{
			"groupUpdate": map[string]interface{}{
				"type":  name,
				"oldId": group,
				"newId": newGroup,
			},
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
	default:
		return nil
	}
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

func (p *Provider) ProxyScriptRun(c *gin.Context, currentNode, script string) error {
	if global.DB == nil {
		return errors.New("database is not initialized")
	}
	var n model.Node
	if err := global.DB.Where("name = ? AND kind = ?", currentNode, kindNode).First(&n).Error; err != nil {
		return fmt.Errorf("node not found: %s", currentNode)
	}
	if n.Addr == "" || n.Status == statusPending {
		return fmt.Errorf("node is not ready: %s", currentNode)
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	var staged struct {
		Token string `json:"token"`
	}
	if err := agentJSONCtx(ctx, currentNode, http.MethodPost, "/xpack/node/script-stage", map[string]string{"script": script}, &staged); err != nil {
		return err
	}
	if staged.Token == "" {
		return errors.New("empty script stage token from node")
	}
	tr, err := p.agentTransport()
	if err != nil {
		return err
	}
	cols := c.DefaultQuery("cols", "80")
	rows := c.DefaultQuery("rows", "40")
	target := net.JoinHostPort(n.Addr, fmt.Sprint(n.Port))
	rawQuery := "token=" + url.QueryEscape(staged.Token) + "&cols=" + url.QueryEscape(cols) + "&rows=" + url.QueryEscape(rows)
	var proxyErr error
	rp := &httputil.ReverseProxy{
		FlushInterval: -1,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme = "https"
			r.Out.URL.Host = target
			r.Out.Host = target
			r.Out.URL.Path = "/api/v2/xpack/node/script-run"
			r.Out.URL.RawPath = ""
			r.Out.URL.RawQuery = rawQuery
			r.Out.Header.Set("Proxy-Id", n.ProxyID)
			r.SetXForwarded()
		},
		Transport: tr,
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			proxyErr = err
		},
	}
	defer func() {
		if rec := recover(); rec != nil && rec != http.ErrAbortHandler {
			global.LOG.Debug(rec)
		}
	}()
	rp.ServeHTTP(c.Writer, c.Request)
	if proxyErr != nil && !c.Writer.Written() {
		return proxyErr
	}
	c.Abort()
	return nil
}

func (p *Provider) Sync(dataType string) error {
	if global.DB == nil {
		return nil
	}
	settingRepo := repo.NewISettingRepo()
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
	if dataType == constant.SyncLanguage || dataType == "" {
		if lang, err := settingRepo.GetValueByKey("Language"); err == nil && strings.TrimSpace(lang) != "" {
			payload["language"] = strings.TrimSpace(lang)
		}
	}
	if dataType == constant.SyncEdition || dataType == "" {
		if edition, err := settingRepo.GetValueByKey("Edition"); err == nil && strings.TrimSpace(edition) != "" {
			payload["edition"] = strings.TrimSpace(edition)
		}
	}
	if dataType == constant.SyncSystemProxy || dataType == constant.SyncSystemProxyWithRestartDocker || dataType == "" {
		proxyType, _ := settingRepo.GetValueByKey("ProxyType")
		proxyUrl, _ := settingRepo.GetValueByKey("ProxyUrl")
		proxyPort, _ := settingRepo.GetValueByKey("ProxyPort")
		proxyUser, _ := settingRepo.GetValueByKey("ProxyUser")
		proxyPasswd, _ := settingRepo.GetValueByKey("ProxyPasswd")
		proxyPasswdKeep, _ := settingRepo.GetValueByKey("ProxyPasswdKeep")
		proxyDocker := GetProxyDockerStatus()

		plainPasswd := ""
		if proxyPasswd != "" {
			if dec, err := encrypt.StringDecrypt(proxyPasswd); err == nil {
				plainPasswd = dec
			} else {
				plainPasswd = proxyPasswd
			}
		}
		dockerProxyURL := buildDockerProxyURL(proxyType, proxyUrl, proxyPort, proxyUser, plainPasswd, proxyDocker)
		payload["systemProxy"] = map[string]interface{}{
			"type":              proxyType,
			"url":               proxyUrl,
			"port":              proxyPort,
			"user":              proxyUser,
			"passwd":            plainPasswd,
			"passwdKeep":        proxyPasswdKeep,
			"proxyDocker":       proxyDocker,
			"dockerProxyUrl":    dockerProxyURL,
			"withDockerRestart": dataType == constant.SyncSystemProxyWithRestartDocker,
		}
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
	syncTimeout := 15 * time.Second
	if dataType == constant.SyncSystemProxyWithRestartDocker {
		syncTimeout = 60 * time.Second
	}
	var errs []string
	for _, target := range targets {
		ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
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
	proxyFunc := http.ProxyFromEnvironment
	if global.DB != nil {
		settingRepo := repo.NewISettingRepo()
		proxyType, _ := settingRepo.GetValueByKey("ProxyType")
		proxyUrl, _ := settingRepo.GetValueByKey("ProxyUrl")
		proxyPort, _ := settingRepo.GetValueByKey("ProxyPort")
		proxyUser, _ := settingRepo.GetValueByKey("ProxyUser")
		proxyPasswd, _ := settingRepo.GetValueByKey("ProxyPasswd")

		proxyType = strings.ToLower(strings.TrimSpace(proxyType))
		proxyUrl = strings.TrimSpace(proxyUrl)
		proxyPort = strings.TrimSpace(proxyPort)
		if (proxyType == "http" || proxyType == "https" || proxyType == "socks5") && proxyUrl != "" {
			rawURL := proxyUrl
			if !strings.Contains(rawURL, "://") {
				host := rawURL
				if ip := net.ParseIP(host); ip != nil && strings.Contains(host, ":") {
					host = "[" + host + "]"
				}
				rawURL = proxyType + "://" + host
			}
			if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
				if proxyPort != "" && u.Port() == "" {
					u.Host = net.JoinHostPort(u.Hostname(), proxyPort)
				}
				if proxyUser != "" {
					plainPass := ""
					if proxyPasswd != "" {
						if dec, decErr := encrypt.StringDecrypt(proxyPasswd); decErr == nil {
							plainPass = dec
						} else {
							plainPass = proxyPasswd
						}
					}
					u.User = url.UserPassword(proxyUser, plainPass)
				}
				proxyFunc = http.ProxyURL(u)
			}
		}
	}

	return &http.Transport{
		Proxy:           proxyFunc,
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
		p.transport = nil
	}
}

// Default is the single provider instance shared by the router and the helper.
var Default = NewProvider()
