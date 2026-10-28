//go:build xpack || enterprise

package helper

import (
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/buserr"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/common"
	"github.com/1Panel-dev/1Panel/agent/utils/xpack/providers"
	"github.com/gin-gonic/gin"
)

// NodeConfigPath marks an agent as a managed child node. It is written by the
// install script a master hands out; a machine without it runs as the master's agent.
const NodeConfigPath = "/etc/1panel/node.json"

type nodeConfig struct {
	Name    string `json:"name"`
	Addr    string `json:"addr"`
	Port    uint   `json:"port"`
	BaseDir string `json:"baseDir"`
	Version string `json:"version"`
}

type multiNodeHelper struct{}

func NewIMultiNodeProvider() providers.MultiNodeProvider {
	return &multiNodeHelper{}
}

func loadNodeConfig() (*nodeConfig, bool) {
	data, err := os.ReadFile(NodeConfigPath)
	if err != nil {
		return nil, false
	}
	var cfg nodeConfig
	if json.Unmarshal(data, &cfg) != nil {
		return nil, false
	}
	return &cfg, true
}

func (m *multiNodeHelper) RemoveTamper(website string) {}

func (m *multiNodeHelper) StartClam(startClam *model.Clam, isUpdate bool) (int, error) {
	return 0, buserr.New("ErrXpackNotFound")
}

func (m *multiNodeHelper) LoadNodeInfo(isBase bool) (model.NodeInfo, error) {
	var info model.NodeInfo
	if cfg, ok := loadNodeConfig(); ok {
		info.Scope = "node"
		info.BaseDir = cfg.BaseDir
		if info.BaseDir == "" {
			info.BaseDir = "/opt/1panel"
		}
		info.NodePort = cfg.Port
		info.Version = cfg.Version
		global.IsMaster = false
		return info, nil
	}
	info.BaseDir = common.LoadParams("BASE_DIR")
	info.Version = common.LoadParams("ORIGINAL_VERSION")
	info.Scope = "master"
	global.IsMaster = true
	return info, nil
}

func (m *multiNodeHelper) GetImagePrefix() string { return "" }

func (m *multiNodeHelper) IsUseCustomApp() bool { return false }

func (m *multiNodeHelper) IsXpack() bool { return false }

func (m *multiNodeHelper) LoadRequestTransport() *http.Transport {
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

// ValidateCertificate accepts only requests that completed the mutual-TLS handshake
// against the master's CA; the listener already refuses everything else.
func (m *multiNodeHelper) ValidateCertificate(c *gin.Context) bool {
	return c.Request.TLS != nil && len(c.Request.TLS.VerifiedChains) > 0
}

func (m *multiNodeHelper) PushSSLToNode(websiteSSL *model.WebsiteSSL) error { return nil }

func (m *multiNodeHelper) GetAgentInfo() (*dto.AgentInfo, error) {
	if cfg, ok := loadNodeConfig(); ok {
		return &dto.AgentInfo{NodeName: cfg.Name, NodeAddr: cfg.Addr}, nil
	}
	return nil, nil
}
