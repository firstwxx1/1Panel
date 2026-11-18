//go:build !xpack && !enterprise

package helper

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/init/proxy"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/1Panel-dev/1Panel/core/utils/ssh"
	"github.com/1Panel-dev/1Panel/core/utils/xpack/providers"
	"github.com/gin-gonic/gin"
)

var _ providers.MultiNodeProvider = (*multiNodeHelper)(nil)

type multiNodeHelper struct{}

func NewIMultiNodeProvider() *multiNodeHelper {
	return &multiNodeHelper{}
}

func (m *multiNodeHelper) Proxy(c *gin.Context, currentNode string) {
	if currentNode != "local" && currentNode != "" {
		c.Next()
		return
	}
	defer func() {
		if err := recover(); err != nil && err != http.ErrAbortHandler {
			global.LOG.Debug(err)
		}
	}()
	proxy.LocalAgentProxy.ServeHTTP(c.Writer, c.Request)
	c.Abort()
}

func (m *multiNodeHelper) ProxyDocker(proxyURL string) error {
	if global.DB == nil {
		return nil
	}
	status := constant.StatusDisable
	if strings.TrimSpace(proxyURL) != "" {
		status = constant.StatusEnable
	}
	return repo.NewISettingRepo().UpdateOrCreate("ProxyDocker", status)
}

func (m *multiNodeHelper) UpdateGroup(name string, group, newGroup uint) error { return nil }

func (m *multiNodeHelper) CheckBackupUsed(name string) error { return nil }

func (m *multiNodeHelper) LoadRequestTransport() *http.Transport {
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

func (m *multiNodeHelper) LoadNodeInfo(currentNode string) (*ssh.ConnInfo, string, error) {
	return nil, "", nil
}

func (m *multiNodeHelper) Sync(dataType string) error { return nil }

func (m *multiNodeHelper) AutoUpgradeWithMaster() {}

func (m *multiNodeHelper) ProxyScriptRun(c *gin.Context, currentNode, script string) error {
	return fmt.Errorf("multi-node is not supported in community edition")
}
