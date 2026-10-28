package node

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/1Panel-dev/1Panel/core/utils/req_helper/proxy_local"
)

const (
	kindNode  = "node"
	kindPanel = "panel"

	statusHealthy    = "Healthy"
	statusUnhealthy  = "Unhealthy"
	statusInstalling = "Installing"
	statusFailed     = "Failed"
	statusPending    = "Pending"

	collectInterval = 10 * time.Second
	installTimeout  = 5 * time.Minute
	// a node goes offline after this many consecutive failed probes
	failLimit = 2
)

type metrics struct {
	CPUTotal          int     `json:"cpuTotal"`
	CPUUsedPercent    float64 `json:"cpuUsedPercent"`
	MemoryTotal       uint64  `json:"memoryTotal"`
	MemoryUsedPercent float64 `json:"memoryUsedPercent"`
	Load1             float64 `json:"load1"`
}

type osInfo struct {
	PrettyDistro string `json:"prettyDistro"`
	Platform     string `json:"platform"`
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

var (
	failMu    sync.Mutex
	failCount = map[uint]int{}

	localMu   sync.RWMutex
	localInfo = NodeView{Name: "local", Kind: kindNode, Addr: "127.0.0.1", Status: statusHealthy, IsLocal: true}
)

func collectLoop() {
	t := time.NewTicker(collectInterval)
	defer t.Stop()
	for {
		collectOnce()
		<-t.C
	}
}

func collectOnce() {
	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("node collector panic: %v", r)
		}
	}()
	collectLocal()
	var nodes []model.Node
	if err := global.DB.Where("kind IN ?", []string{kindNode, kindPanel}).Find(&nodes).Error; err != nil {
		return
	}
	var wg sync.WaitGroup
	for i := range nodes {
		n := nodes[i]
		if n.Addr == "" || n.Status == statusPending {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if n.Kind == kindPanel {
				checkPanel(n)
			} else {
				checkNode(n)
			}
		}()
	}
	wg.Wait()
}

func decode[T any](raw interface{}, out *T) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func collectLocal() {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var m metrics
	raw, err := proxy_local.NewLocalClientWithContext(ctx, "/api/v2/dashboard/current/node", http.MethodGet, nil, nil, 8*time.Second)
	localMu.Lock()
	defer localMu.Unlock()
	localInfo.Version = global.CONF.Base.Version
	if err != nil || decode(raw, &m) != nil {
		localInfo.Status = statusUnhealthy
		return
	}
	localInfo.Status = statusHealthy
	localInfo.CPUTotal, localInfo.CPUUsedPercent = m.CPUTotal, m.CPUUsedPercent
	localInfo.MemoryTotal, localInfo.MemoryUsedPercent, localInfo.Load1 = m.MemoryTotal, m.MemoryUsedPercent, m.Load1
	if localInfo.SystemVersion == "" {
		var o osInfo
		if raw, err := proxy_local.NewLocalClientWithContext(ctx, "/api/v2/dashboard/base/os", http.MethodGet, nil, nil, 8*time.Second); err == nil && decode(raw, &o) == nil {
			localInfo.SystemVersion = o.PrettyDistro
		}
	}
}

// agentCall calls a child agent endpoint over mTLS and returns the "data" payload.
func agentCall(n model.Node, method, path string, timeout time.Duration) (json.RawMessage, error) {
	tr, err := Default.agentTransport()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, "https://"+net.JoinHostPort(n.Addr, fmt.Sprint(n.Port))+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Proxy-Id", n.ProxyID)
	client := &http.Client{Transport: tr, Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent returned %d", resp.StatusCode)
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	if env.Code != 200 {
		return nil, fmt.Errorf("agent error %d: %s", env.Code, env.Message)
	}
	return env.Data, nil
}

func agentGet(n model.Node, path string, timeout time.Duration) (json.RawMessage, error) {
	return agentCall(n, http.MethodGet, path, timeout)
}

func recordFailure(n model.Node, err error) {
	failMu.Lock()
	failCount[n.ID]++
	count := failCount[n.ID]
	failMu.Unlock()
	update := map[string]interface{}{"message": err.Error()}
	switch {
	case n.Status == statusInstalling:
		if time.Since(n.CreatedAt) > installTimeout {
			update["status"] = statusFailed
		}
	case count >= failLimit && n.Status == statusHealthy:
		update["status"] = statusUnhealthy
	}
	_ = global.DB.Model(&model.Node{}).Where("id = ?", n.ID).Updates(update).Error
}

func checkNode(n model.Node) {
	raw, err := agentGet(n, "/api/v2/dashboard/current/node", 6*time.Second)
	if err != nil {
		recordFailure(n, err)
		return
	}
	var m metrics
	if err := json.Unmarshal(raw, &m); err != nil {
		recordFailure(n, err)
		return
	}
	failMu.Lock()
	delete(failCount, n.ID)
	failMu.Unlock()
	update := map[string]interface{}{
		"status": statusHealthy, "message": "", "last_seen": time.Now(),
		"cpu_total": m.CPUTotal, "cpu_used_percent": m.CPUUsedPercent,
		"memory_total": m.MemoryTotal, "memory_used_percent": m.MemoryUsedPercent, "load1": m.Load1,
	}
	if raw, err := agentGet(n, "/api/v2/xpack/node/ping", 4*time.Second); err == nil {
		var ping struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(raw, &ping) == nil && ping.Version != "" {
			update["version"] = ping.Version
		}
	}
	if n.SystemVersion == "" {
		if raw, err := agentGet(n, "/api/v2/dashboard/base/os", 4*time.Second); err == nil {
			var o osInfo
			if json.Unmarshal(raw, &o) == nil {
				update["system_version"] = o.PrettyDistro
			}
		}
	}
	_ = global.DB.Model(&model.Node{}).Where("id = ?", n.ID).Updates(update).Error
}

// panelURL builds the base URL of a bookmarked panel.
func panelURL(n model.Node) string {
	proto := n.Protocol
	if proto == "" {
		proto = "http"
	}
	return fmt.Sprintf("%s://%s", proto, net.JoinHostPort(n.Addr, fmt.Sprint(n.Port)))
}

func panelCall(n model.Node, path string) (json.RawMessage, error) {
	key, err := encrypt.StringDecrypt(n.APIKey)
	if err != nil || key == "" {
		return nil, fmt.Errorf("api key is not set")
	}
	ts := fmt.Sprint(time.Now().Unix())
	sum := md5.Sum([]byte("1panel" + key + ts))
	req, err := http.NewRequest(http.MethodGet, panelURL(n)+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("1Panel-Token", hex.EncodeToString(sum[:]))
	req.Header.Set("1Panel-Timestamp", ts)
	client := &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil || env.Code != 200 {
		return nil, fmt.Errorf("panel returned %d", resp.StatusCode)
	}
	return env.Data, nil
}

func checkPanel(n model.Node) {
	raw, err := panelCall(n, "/api/v2/core/settings/search")
	if err != nil {
		_ = global.DB.Model(&model.Node{}).Where("id = ?", n.ID).
			Updates(map[string]interface{}{"status": statusUnhealthy, "message": strings.TrimSpace(err.Error())}).Error
		return
	}
	var s struct {
		SystemVersion string `json:"systemVersion"`
	}
	_ = json.Unmarshal(raw, &s)
	update := map[string]interface{}{"status": statusHealthy, "message": "", "version": s.SystemVersion, "last_seen": time.Now()}
	if raw, err := panelCall(n, "/api/v2/dashboard/current/node"); err == nil {
		var m metrics
		if json.Unmarshal(raw, &m) == nil {
			update["cpu_total"], update["cpu_used_percent"] = m.CPUTotal, m.CPUUsedPercent
			update["memory_total"], update["memory_used_percent"], update["load1"] = m.MemoryTotal, m.MemoryUsedPercent, m.Load1
		}
	}
	_ = global.DB.Model(&model.Node{}).Where("id = ?", n.ID).Updates(update).Error
}

// probeNow runs one immediate health probe, used by "test connection".
func probeNow(n model.Node) error {
	if n.Kind == kindPanel {
		_, err := panelCall(n, "/api/v2/core/settings/search")
		return err
	}
	_, err := agentGet(n, "/api/v2/dashboard/current/node", 6*time.Second)
	return err
}
