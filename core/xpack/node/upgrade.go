package node

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/global"
)

type UpgradeNodeResult struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

var (
	geoMu       sync.Mutex
	geoSynced   = map[uint]bool{}
	geoInFlight = map[uint]bool{}
	geoHashPath string
	geoHashMT   time.Time
	geoHashVal  string
)

func clearGeoSynced(id uint) {
	geoMu.Lock()
	delete(geoSynced, id)
	geoMu.Unlock()
}

func masterInstallDir() string {
	if global.CONF.Base.InstallDir != "" {
		return global.CONF.Base.InstallDir
	}
	return "/opt"
}

func masterGeoIPPath() string {
	return path.Join(masterInstallDir(), "1panel/geo/GeoIP.mmdb")
}

func ensureMasterGeoIP() (string, string, error) {
	target := masterGeoIPPath()
	st, err := os.Stat(target)
	if err != nil || st.IsDir() || st.Size() <= 100*1024 {
		patterns := []string{
			filepath.Join(masterInstallDir(), "1panel/apps/openresty/*/1pwaf/data/geo/GeoIP.mmdb"),
			filepath.Join(masterInstallDir(), "1panel/apps/local/openresty/*/1pwaf/data/geo/GeoIP.mmdb"),
		}
		copied := false
		for _, pattern := range patterns {
			matches, _ := filepath.Glob(pattern)
			for _, src := range matches {
				srcSt, sErr := os.Stat(src)
				if sErr != nil || srcSt.IsDir() || srcSt.Size() <= 100*1024 {
					continue
				}
				if mkErr := os.MkdirAll(path.Dir(target), 0o755); mkErr != nil {
					continue
				}
				in, oErr := os.Open(src)
				if oErr != nil {
					continue
				}
				tmp := target + ".tmp"
				out, cErr := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
				if cErr != nil {
					_ = in.Close()
					continue
				}
				_, cpErr := io.Copy(out, in)
				_ = in.Close()
				clErr := out.Close()
				if cpErr != nil || clErr != nil {
					_ = os.Remove(tmp)
					continue
				}
				if rErr := os.Rename(tmp, target); rErr == nil {
					global.LOG.Info("copied GeoIP.mmdb from local openresty 1pwaf")
					copied = true
					break
				}
				_ = os.Remove(tmp)
			}
			if copied {
				break
			}
		}
		st, err = os.Stat(target)
		if err != nil || st.IsDir() || st.Size() <= 100*1024 {
			return "", "", errors.New("master GeoIP.mmdb not available")
		}
	}

	geoMu.Lock()
	if geoHashPath == target && geoHashVal != "" && st.ModTime().Equal(geoHashMT) {
		sum := geoHashVal
		geoMu.Unlock()
		return target, sum, nil
	}
	geoMu.Unlock()

	sum, err := fileSHA256(target)
	if err != nil {
		return "", "", err
	}
	geoMu.Lock()
	geoHashPath = target
	geoHashMT = st.ModTime()
	geoHashVal = sum
	geoMu.Unlock()
	return target, sum, nil
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func pushBinaryStream(n model.Node, apiPath, filePath string, headers map[string]string, timeout time.Duration) error {
	tr, err := Default.agentTransport()
	if err != nil {
		return err
	}
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	pr, pw := io.Pipe()
	go func() {
		gw, _ := gzip.NewWriterLevel(pw, gzip.BestSpeed)
		_, cpErr := io.Copy(gw, f)
		clErr := gw.Close()
		if cpErr != nil {
			_ = pw.CloseWithError(cpErr)
		} else if clErr != nil {
			_ = pw.CloseWithError(clErr)
		} else {
			_ = pw.Close()
		}
	}()

	reqURL := "https://" + net.JoinHostPort(n.Addr, fmt.Sprint(n.Port)) + apiPath
	req, err := http.NewRequest(http.MethodPost, reqURL, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Proxy-Id", n.ProxyID)
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	client := &http.Client{Transport: tr, Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agent returned http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return err
	}
	if env.Code != http.StatusOK {
		return fmt.Errorf("agent error %d: %s", env.Code, env.Message)
	}
	return nil
}

func pushGeoIPToNode(n model.Node) error {
	geoPath, geoSum, err := ensureMasterGeoIP()
	if err != nil {
		return err
	}
	return pushBinaryStream(n, "/api/v2/xpack/node/geoip", geoPath, map[string]string{
		"X-Sha256": geoSum,
	}, 6*time.Minute)
}

func maybeSyncNodeGeoIP(n model.Node, hasGeoIP bool) {
	if hasGeoIP {
		geoMu.Lock()
		geoSynced[n.ID] = true
		geoMu.Unlock()
		return
	}
	geoMu.Lock()
	if geoSynced[n.ID] || geoInFlight[n.ID] {
		geoMu.Unlock()
		return
	}
	geoInFlight[n.ID] = true
	geoMu.Unlock()

	go func(target model.Node) {
		err := pushGeoIPToNode(target)
		geoMu.Lock()
		delete(geoInFlight, target.ID)
		if err == nil {
			geoSynced[target.ID] = true
		}
		geoMu.Unlock()
		if err != nil {
			global.LOG.Debugf("sync GeoIP.mmdb to node %s skipped/failed: %v", target.Name, err)
		} else {
			global.LOG.Infof("synced GeoIP.mmdb to node %s", target.Name)
		}
	}(n)
}

// EnsureMasterAndNodesGeoIP ensures GeoIP.mmdb is present on the master and pushes it to healthy child nodes lacking it.
func EnsureMasterAndNodesGeoIP() {
	if global.DB == nil {
		return
	}
	if _, _, err := ensureMasterGeoIP(); err != nil {
		return
	}
	var nodes []model.Node
	if err := global.DB.Where("kind = ? AND status = ?", kindNode, statusHealthy).Find(&nodes).Error; err != nil {
		return
	}
	for _, n := range nodes {
		if n.Addr == "" {
			continue
		}
		geoMu.Lock()
		done := geoSynced[n.ID] || geoInFlight[n.ID]
		geoMu.Unlock()
		if done {
			continue
		}
		raw, err := agentGet(n, "/api/v2/xpack/node/ping", 5*time.Second)
		if err != nil {
			continue
		}
		var ping struct {
			HasGeoIP *bool `json:"hasGeoIP"`
		}
		if json.Unmarshal(raw, &ping) == nil && ping.HasGeoIP != nil {
			maybeSyncNodeGeoIP(n, *ping.HasGeoIP)
		} else {
			maybeSyncNodeGeoIP(n, false)
		}
	}
}

func masterVersion() string {
	if v, err := repo.NewISettingRepo().GetValueByKey("SystemVersion"); err == nil && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return strings.TrimSpace(global.CONF.Base.Version)
}

// UpgradeNodes streams the master's 1panel-agent binary (and GeoIP.mmdb if present) to specified or all bound child nodes.
func UpgradeNodes(ids []uint) ([]UpgradeNodeResult, error) {
	if global.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	binPath, err := agentBinaryPath()
	if err != nil {
		return nil, err
	}
	shaSum, err := fileSHA256(binPath)
	if err != nil {
		return nil, fmt.Errorf("compute agent sha256 failed: %w", err)
	}
	ver := masterVersion()

	var nodes []model.Node
	db := global.DB.Where("kind = ?", kindNode)
	if len(ids) > 0 {
		db = db.Where("id IN ?", ids)
	} else {
		db = db.Where("addr <> '' AND status <> ?", statusPending)
	}
	if err := db.Order("id ASC").Find(&nodes).Error; err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return []UpgradeNodeResult{}, nil
	}

	results := make([]UpgradeNodeResult, len(nodes))
	var wg sync.WaitGroup
	for i := range nodes {
		i := i
		n := nodes[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := UpgradeNodeResult{ID: n.ID, Name: n.Name}
			if n.Addr == "" || n.Status == statusPending {
				res.Message = "node has not joined yet"
				results[i] = res
				return
			}
			if raw, pErr := agentGet(n, "/api/v2/xpack/node/ping", 6*time.Second); pErr == nil {
				var ping struct {
					Arch string `json:"arch"`
				}
				if json.Unmarshal(raw, &ping) == nil && ping.Arch != "" && ping.Arch != runtime.GOARCH {
					res.Message = fmt.Sprintf("architecture mismatch: master is %s, node is %s", runtime.GOARCH, ping.Arch)
					results[i] = res
					return
				}
			}
			// Push GeoIP.mmdb first while the current agent is running (non-fatal if master lacks GeoIP.mmdb).
			if err := pushGeoIPToNode(n); err == nil {
				geoMu.Lock()
				geoSynced[n.ID] = true
				geoMu.Unlock()
			} else {
				clearGeoSynced(n.ID)
			}
			upgradePath := "/api/v2/xpack/node/upgrade"
			if ver != "" {
				upgradePath += "?version=" + url.QueryEscape(ver)
			}
			err := pushBinaryStream(n, upgradePath, binPath, map[string]string{
				"X-Sha256":  shaSum,
				"X-Arch":    runtime.GOARCH,
				"X-Version": ver,
			}, 8*time.Minute)
			if err != nil && strings.Contains(err.Error(), "http 404") {
				err = upgradeLegacyNode(n, binPath)
			}
			if err != nil {
				res.Message = err.Error()
				results[i] = res
				return
			}
			// Ensure GeoIP.mmdb is synced after upgrade as well (especially for legacy nodes that lacked /geoip before).
			clearGeoSynced(n.ID)
			go func(target model.Node) {
				time.Sleep(4 * time.Second)
				maybeSyncNodeGeoIP(target, false)
			}(n)
			if ver != "" {
				_ = global.DB.Model(&model.Node{}).Where("id = ?", n.ID).Update("version", ver).Error
			}
			res.OK = true
			res.Message = "upgraded"
			results[i] = res
		}()
	}
	wg.Wait()
	return results, nil
}

// upgradeLegacyNode upgrades an older child agent that predates POST /api/v2/xpack/node/upgrade
// by uploading a gzip-compressed tar.gz (~27MB instead of 76MB) to /tmp, extracting, moving into /usr/local/bin, and restarting.
func upgradeLegacyNode(n model.Node, binPath string) error {
	tr, err := Default.agentTransport()
	if err != nil {
		return err
	}
	f, err := os.Open(binPath)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		var wErr error
		defer func() {
			_ = mw.Close()
			if wErr != nil {
				_ = pw.CloseWithError(wErr)
			} else {
				_ = pw.Close()
			}
		}()
		if wErr = mw.WriteField("path", "/tmp"); wErr != nil {
			return
		}
		if wErr = mw.WriteField("overwrite", "true"); wErr != nil {
			return
		}
		part, err := mw.CreateFormFile("file", "1panel-agent-up.tar.gz")
		if err != nil {
			wErr = err
			return
		}
		gw, _ := gzip.NewWriterLevel(part, gzip.BestSpeed)
		tw := tar.NewWriter(gw)
		hdr := &tar.Header{
			Name:    "1panel-agent",
			Mode:    0o755,
			Size:    st.Size(),
			ModTime: time.Now(),
		}
		if wErr = tw.WriteHeader(hdr); wErr != nil {
			_ = gw.Close()
			return
		}
		if _, wErr = io.Copy(tw, f); wErr != nil {
			_ = tw.Close()
			_ = gw.Close()
			return
		}
		if wErr = tw.Close(); wErr != nil {
			_ = gw.Close()
			return
		}
		wErr = gw.Close()
	}()

	uploadURL := "https://" + net.JoinHostPort(n.Addr, fmt.Sprint(n.Port)) + "/api/v2/files/upload"
	req, err := http.NewRequest(http.MethodPost, uploadURL, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Proxy-Id", n.ProxyID)

	client := &http.Client{Transport: tr, Timeout: 8 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("legacy upload agent failed: %w", err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("legacy upload returned http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return err
	}
	if env.Code != http.StatusOK {
		return fmt.Errorf("legacy upload error %d: %s", env.Code, env.Message)
	}

	// Decompress into /tmp/1panel-agent-up (async task), wait until extracted, move over /usr/local/bin/1panel-agent, wait until moved, clean up, and restart.
	if err := agentJSON(n.Name, http.MethodPost, "/files/decompress", map[string]interface{}{
		"path": "/tmp/1panel-agent-up.tar.gz",
		"dst":  "/tmp/1panel-agent-up",
		"type": "tar.gz",
	}, nil); err != nil {
		return fmt.Errorf("legacy decompress failed: %w", err)
	}
	if !waitRemoteFile(n.Name, "/tmp/1panel-agent-up/1panel-agent", true, 45*time.Second) {
		return errors.New("legacy decompress timed out waiting for /tmp/1panel-agent-up/1panel-agent")
	}
	if err := agentJSON(n.Name, http.MethodPost, "/files/move", map[string]interface{}{
		"type":     "cut",
		"oldPaths": []string{"/tmp/1panel-agent-up/1panel-agent"},
		"newPath":  "/usr/local/bin",
		"name":     "1panel-agent",
		"cover":    true,
	}, nil); err != nil {
		return fmt.Errorf("legacy move binary failed: %w", err)
	}
	if !waitRemoteFile(n.Name, "/tmp/1panel-agent-up/1panel-agent", false, 30*time.Second) {
		return errors.New("legacy move timed out waiting for cut completion")
	}
	_ = agentJSON(n.Name, http.MethodPost, "/files/del", map[string]interface{}{
		"path":        "/tmp/1panel-agent-up.tar.gz",
		"isDir":       false,
		"forceDelete": true,
	}, nil)
	_ = agentJSON(n.Name, http.MethodPost, "/files/del", map[string]interface{}{
		"path":        "/tmp/1panel-agent-up",
		"isDir":       true,
		"forceDelete": true,
	}, nil)
	if _, err := agentCall(n, http.MethodPost, "/api/v2/xpack/node/restart", 10*time.Second); err != nil {
		return fmt.Errorf("legacy restart failed: %w", err)
	}
	return nil
}

func waitRemoteFile(nodeName, filePath string, wantExist bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var exists bool
		if err := agentJSON(nodeName, http.MethodPost, "/files/check", map[string]interface{}{
			"path": filePath,
		}, &exists); err == nil && exists == wantExist {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}
