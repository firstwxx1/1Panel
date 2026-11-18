//go:build xpack || enterprise

package helper

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/buserr"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/common"
	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
	"github.com/1Panel-dev/1Panel/agent/utils/xpack/providers"
	"github.com/1Panel-dev/1Panel/agent/xpack/tamper"
	"github.com/gin-gonic/gin"
)

// NodeConfigPath marks an agent as a managed child node. It is written by the
// install script a master hands out; a machine without it runs as the master's agent.
const NodeConfigPath = "/etc/1panel/node.json"

// RemoveTamperHook allows the tamper subsystem to register an in-memory cleanup callback.
var RemoveTamperHook func(website string)

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

func (m *multiNodeHelper) RemoveTamper(website string) {
	website = strings.TrimSpace(website)
	if website == "" || website == "." || website == ".." || strings.ContainsAny(website, "/\\") {
		return
	}
	if RemoveTamperHook != nil {
		RemoveTamperHook(website)
		return
	}
	if s, err := tamper.DefaultStore(); err == nil && s != nil {
		s.RemoveSite(website)
		return
	}
	if global.Dir.DataDir == "" {
		return
	}
	tamperDir := path.Join(global.Dir.DataDir, "tamper")
	_ = os.Remove(path.Join(tamperDir, "baseline", website+".json"))
	_ = os.RemoveAll(path.Join(tamperDir, "backup", website))
	_ = os.RemoveAll(path.Join(tamperDir, "quarantine", website))
}

func (m *multiNodeHelper) StartClam(startClam *model.Clam, isUpdate bool) (int, error) {
	return 0, buserr.New("ErrXpackNotFound")
}

func (m *multiNodeHelper) LoadNodeInfo(isBase bool) (model.NodeInfo, error) {
	var info model.NodeInfo
	if cfg, ok := loadNodeConfig(); ok {
		info.Scope = "node"
		info.BaseDir = cfg.BaseDir
		if info.BaseDir == "" {
			info.BaseDir = "/opt"
		}
		info.NodePort = cfg.Port
		compiled := strings.TrimSpace(global.CONF.Base.Version)
		if compiled != "" && compiled != "v2.0.0" && compiled != "dev" {
			info.Version = compiled
		} else {
			info.Version = cfg.Version
		}
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

func (m *multiNodeHelper) IsXpack() bool { return true }

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
// against the master's CA; the listener already refuses everything else. Requests arriving
// over the local unix socket (no TLS at all) come from the machine's own core and pass.
func (m *multiNodeHelper) ValidateCertificate(c *gin.Context) bool {
	if c.Request.TLS == nil {
		if addr, ok := c.Request.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && addr.Network() == "unix" {
			return true
		}
		return c.Request.RemoteAddr == "@" || strings.HasSuffix(c.Request.RemoteAddr, ".sock")
	}
	return len(c.Request.TLS.VerifiedChains) > 0
}

func (m *multiNodeHelper) PushSSLToNode(websiteSSL *model.WebsiteSSL) error {
	if !global.IsMaster || websiteSSL == nil || !websiteSSL.PushNode || strings.TrimSpace(websiteSSL.Nodes) == "" || global.CoreDB == nil {
		return nil
	}
	type coreSetting struct {
		Key   string `gorm:"column:key"`
		Value string `gorm:"column:value"`
	}
	var settingRows []coreSetting
	if err := global.CoreDB.Table("settings").
		Where("`key` IN ?", []string{"NodeCACrt", "NodeClientCrt", "NodeClientKey", "EncryptKey"}).
		Find(&settingRows).Error; err != nil {
		return fmt.Errorf("read master mTLS settings failed: %w", err)
	}
	kv := make(map[string]string, len(settingRows))
	for _, r := range settingRows {
		kv[r.Key] = r.Value
	}
	if kv["NodeCACrt"] == "" || kv["NodeClientCrt"] == "" || kv["NodeClientKey"] == "" || kv["EncryptKey"] == "" {
		return errors.New("master mTLS credentials are not initialized")
	}
	clientKeyPlain, err := encrypt.StringDecryptWithKey(kv["NodeClientKey"], kv["EncryptKey"])
	if err != nil || clientKeyPlain == "" {
		return fmt.Errorf("decrypt master client key failed: %w", err)
	}
	pair, err := tls.X509KeyPair([]byte(kv["NodeClientCrt"]), []byte(clientKeyPlain))
	if err != nil {
		return fmt.Errorf("load master client certificate failed: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(kv["NodeCACrt"])) {
		return errors.New("invalid master CA certificate")
	}

	type coreNode struct {
		ID      uint   `gorm:"column:id"`
		Name    string `gorm:"column:name"`
		Addr    string `gorm:"column:addr"`
		Port    int    `gorm:"column:port"`
		ProxyID string `gorm:"column:proxy_id"`
	}
	var allNodes []coreNode
	if err := global.CoreDB.Table("nodes").Where("kind = ?", "node").Find(&allNodes).Error; err != nil {
		return fmt.Errorf("query target nodes failed: %w", err)
	}
	byKey := make(map[string]coreNode, len(allNodes)*2)
	for _, n := range allNodes {
		byKey[n.Name] = n
		byKey[strconv.FormatUint(uint64(n.ID), 10)] = n
	}

	payload, err := json.Marshal(map[string]interface{}{
		"masterSslId":   websiteSSL.ID,
		"primaryDomain": websiteSSL.PrimaryDomain,
		"domains":       websiteSSL.Domains,
		"pem":           websiteSSL.Pem,
		"privateKey":    websiteSSL.PrivateKey,
		"organization":  websiteSSL.Organization,
		"type":          websiteSSL.Type,
		"keyType":       websiteSSL.KeyType,
		"description":   websiteSSL.Description,
		"expireDate":    websiteSSL.ExpireDate,
		"startDate":     websiteSSL.StartDate,
	})
	if err != nil {
		return err
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{pair},
			RootCAs:      pool,
			MinVersion:   tls.VersionTLS12,
		},
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 15 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 20 * time.Second}

	var errs []string
	seen := map[uint]bool{}
	for _, rawTarget := range strings.Split(websiteSSL.Nodes, ",") {
		target := strings.TrimSpace(rawTarget)
		if target == "" || target == "local" {
			continue
		}
		n, ok := byKey[target]
		if !ok {
			errs = append(errs, fmt.Sprintf("node %s not found", target))
			continue
		}
		if seen[n.ID] {
			continue
		}
		seen[n.ID] = true
		if n.Addr == "" || n.Port <= 0 {
			errs = append(errs, fmt.Sprintf("node %s has not joined yet", n.Name))
			continue
		}
		url := fmt.Sprintf("https://%s/api/v2/xpack/node/ssl-sync", net.JoinHostPort(n.Addr, strconv.Itoa(n.Port)))
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", n.Name, err))
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Proxy-Id", n.ProxyID)
		resp, err := client.Do(req)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", n.Name, err))
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		var env struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &env) != nil || env.Code != http.StatusOK {
			msg := env.Message
			if msg == "" {
				msg = fmt.Sprintf("http %d", resp.StatusCode)
			}
			errs = append(errs, fmt.Sprintf("%s: %s", n.Name, msg))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (m *multiNodeHelper) GetAgentInfo() (*dto.AgentInfo, error) {
	if cfg, ok := loadNodeConfig(); ok {
		return &dto.AgentInfo{NodeName: cfg.Name, NodeAddr: cfg.Addr}, nil
	}
	return nil, nil
}
