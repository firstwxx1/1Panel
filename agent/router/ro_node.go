//go:build xpack || enterprise

package router

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/app/service"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/cmd"
	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
	"github.com/1Panel-dev/1Panel/agent/xpack/appsync"
	"github.com/gin-gonic/gin"
)

// NodeRouter holds the endpoints a master uses to manage a child node's agent.
type NodeRouter struct{}

func currentNodeVersion() string {
	ver := strings.TrimSpace(global.CONF.Base.Version)
	if ver != "" && ver != "v2.0.0" && ver != "dev" {
		return ver
	}
	if b, err := os.ReadFile("/etc/1panel/node.json"); err == nil {
		var m struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(b, &m) == nil && strings.TrimSpace(m.Version) != "" {
			return strings.TrimSpace(m.Version)
		}
	}
	return ver
}

func stripAutoSSHKey(akPath string) {
	b, err := os.ReadFile(akPath)
	if err != nil {
		return
	}
	var kept []string
	changed := false
	for _, line := range strings.Split(string(b), "\n") {
		if strings.Contains(line, "1panel-local-terminal") {
			changed = true
			continue
		}
		kept = append(kept, line)
	}
	if !changed {
		return
	}
	out := strings.Join(kept, "\n")
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	_ = os.WriteFile(akPath, []byte(out), 0o600)
}

func randHexSuffix() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func bodyStream(c *gin.Context) (io.ReadCloser, error) {
	if strings.EqualFold(strings.TrimSpace(c.GetHeader("Content-Encoding")), "gzip") {
		return gzip.NewReader(c.Request.Body)
	}
	return io.NopCloser(c.Request.Body), nil
}

func (a *NodeRouter) InitRouter(Router *gin.RouterGroup) {
	g := Router.Group("xpack/node")
	g.GET("/ping", func(c *gin.Context) {
		hasGeoIP := false
		if global.Dir.DataDir != "" {
			if st, err := os.Stat(path.Join(global.Dir.DataDir, "geo", "GeoIP.mmdb")); err == nil && st.Size() > 100*1024 {
				hasGeoIP = true
			}
		}
		helper.SuccessWithData(c, map[string]interface{}{
			"master":   global.IsMaster,
			"version":  currentNodeVersion(),
			"arch":     runtime.GOARCH,
			"hasGeoIP": hasGeoIP,
		})
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

	// unbind forgets the master: certificates, proxy-id, and node config are removed
	g.POST("/unbind", func(c *gin.Context) {
		if global.IsMaster {
			helper.BadRequest(c, os.ErrPermission)
			return
		}
		_ = os.RemoveAll(filepath.Join("/etc/1panel", "node"))
		_ = os.Remove("/etc/1panel/node.json")
		_ = os.Remove("/etc/1panel/.nodeProxyID")
		stripAutoSSHKey("/root/.ssh/authorized_keys")
		_, hasStandalone := os.Stat("/usr/local/bin/1pctl")
		helper.Success(c)
		go func() {
			time.Sleep(300 * time.Millisecond)
			if hasStandalone == nil {
				os.Exit(1) // systemd restarts agent into local master mode
			}
			_ = exec.Command("systemctl", "disable", "1panel-agent").Run()
			os.Exit(0)
		}()
	})

	g.POST("/upgrade", func(c *gin.Context) {
		if wantArch := strings.TrimSpace(c.GetHeader("X-Arch")); wantArch != "" && wantArch != runtime.GOARCH {
			helper.BadRequest(c, fmt.Errorf("architecture mismatch: got %s, want %s", wantArch, runtime.GOARCH))
			return
		}
		tmpPath := "/usr/local/bin/1panel-agent.upgrade.tmp"
		_ = os.Remove(tmpPath)
		f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			helper.InternalServer(c, err)
			return
		}
		cleanup := func() {
			_ = f.Close()
			_ = os.Remove(tmpPath)
		}
		stream, err := bodyStream(c)
		if err != nil {
			cleanup()
			helper.BadRequest(c, fmt.Errorf("invalid compressed stream: %w", err))
			return
		}
		defer stream.Close()
		hasher := sha256.New()
		tee := io.TeeReader(stream, hasher)
		var magic [4]byte
		if _, err := io.ReadFull(tee, magic[:]); err != nil {
			cleanup()
			helper.BadRequest(c, fmt.Errorf("read binary header failed: %w", err))
			return
		}
		if !bytes.Equal(magic[:], []byte{0x7f, 'E', 'L', 'F'}) {
			cleanup()
			helper.BadRequest(c, errors.New("invalid ELF binary"))
			return
		}
		if _, err := f.Write(magic[:]); err != nil {
			cleanup()
			helper.InternalServer(c, err)
			return
		}
		rest, err := io.Copy(f, tee)
		if err != nil {
			cleanup()
			helper.InternalServer(c, err)
			return
		}
		totalSize := rest + 4
		if totalSize <= 1<<20 {
			cleanup()
			helper.BadRequest(c, fmt.Errorf("agent binary too small (%d bytes)", totalSize))
			return
		}
		gotSum := hex.EncodeToString(hasher.Sum(nil))
		if wantSum := strings.TrimSpace(c.GetHeader("X-Sha256")); wantSum != "" && !strings.EqualFold(gotSum, wantSum) {
			cleanup()
			helper.BadRequest(c, fmt.Errorf("sha256 mismatch: got %s, want %s", gotSum, wantSum))
			return
		}
		_ = f.Sync()
		if err := f.Close(); err != nil {
			_ = os.Remove(tmpPath)
			helper.InternalServer(c, err)
			return
		}
		_ = os.Chmod(tmpPath, 0o755)
		if err := os.Rename(tmpPath, "/usr/local/bin/1panel-agent"); err != nil {
			_ = os.Remove(tmpPath)
			helper.InternalServer(c, err)
			return
		}
		if ver := strings.TrimSpace(c.GetHeader("X-Version")); ver != "" {
			if b, err := os.ReadFile("/etc/1panel/node.json"); err == nil {
				var cfg map[string]interface{}
				if json.Unmarshal(b, &cfg) == nil && cfg != nil {
					cfg["version"] = ver
					if out, err := json.Marshal(cfg); err == nil {
						_ = os.WriteFile("/etc/1panel/node.json", out, 0o600)
					}
				}
			}
			global.CONF.Base.Version = ver
			if global.DB != nil {
				_ = global.DB.Model(&model.Setting{}).Where("key = ?", "SystemVersion").Update("value", ver).Error
			}
		}
		helper.Success(c)
		go func() {
			time.Sleep(300 * time.Millisecond)
			if err := exec.Command("systemctl", "restart", "1panel-agent").Run(); err != nil {
				os.Exit(1)
			}
			time.Sleep(2 * time.Second)
			os.Exit(1)
		}()
	})

	g.POST("/geoip", func(c *gin.Context) {
		if global.Dir.DataDir == "" {
			helper.InternalServer(c, errors.New("data directory not initialized"))
			return
		}
		geoDir := path.Join(global.Dir.DataDir, "geo")
		if err := os.MkdirAll(geoDir, 0o755); err != nil {
			helper.InternalServer(c, err)
			return
		}
		tmpPath := path.Join(geoDir, "GeoIP.mmdb.tmp")
		_ = os.Remove(tmpPath)
		f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			helper.InternalServer(c, err)
			return
		}
		stream, err := bodyStream(c)
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)
			helper.BadRequest(c, fmt.Errorf("invalid compressed stream: %w", err))
			return
		}
		defer stream.Close()
		hasher := sha256.New()
		n, err := io.Copy(f, io.TeeReader(stream, hasher))
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			_ = os.Remove(tmpPath)
			if err != nil {
				helper.InternalServer(c, err)
			} else {
				helper.InternalServer(c, closeErr)
			}
			return
		}
		if n <= 100*1024 {
			_ = os.Remove(tmpPath)
			helper.BadRequest(c, fmt.Errorf("GeoIP.mmdb too small (%d bytes)", n))
			return
		}
		gotSum := hex.EncodeToString(hasher.Sum(nil))
		if wantSum := strings.TrimSpace(c.GetHeader("X-Sha256")); wantSum != "" && !strings.EqualFold(gotSum, wantSum) {
			_ = os.Remove(tmpPath)
			helper.BadRequest(c, fmt.Errorf("sha256 mismatch: got %s, want %s", gotSum, wantSum))
			return
		}
		if err := os.Rename(tmpPath, path.Join(geoDir, "GeoIP.mmdb")); err != nil {
			_ = os.Remove(tmpPath)
			helper.InternalServer(c, err)
			return
		}
		helper.Success(c)
	})

	g.POST("/sync-meta", func(c *gin.Context) {
		var req struct {
			BackupAccounts *[]model.BackupAccount `json:"backupAccounts"`
			Scripts        *[]model.ScriptLibrary `json:"scripts"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			helper.BadRequest(c, err)
			return
		}
		if req.BackupAccounts != nil {
			items := make([]model.BackupAccount, 0, len(*req.BackupAccounts))
			for _, item := range *req.BackupAccounts {
				item.IsPublic = true
				if item.AccessKey != "" {
					if enc, err := encrypt.StringEncrypt(item.AccessKey); err == nil {
						item.AccessKey = enc
					}
				}
				if item.Credential != "" {
					if enc, err := encrypt.StringEncrypt(item.Credential); err == nil {
						item.Credential = enc
					}
				}
				items = append(items, item)
			}
			if err := repo.NewIBackupRepo().SyncAll(items); err != nil {
				helper.InternalServer(c, err)
				return
			}
		}
		if req.Scripts != nil {
			if err := repo.NewIScriptRepo().SyncAll(*req.Scripts); err != nil {
				helper.InternalServer(c, err)
				return
			}
		}
		helper.Success(c)
	})

	g.POST("/check-backup-used", func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			helper.BadRequest(c, err)
			return
		}
		if err := service.NewIBackupService().CheckUsed(req.Name, true); err != nil {
			helper.InternalServer(c, err)
			return
		}
		helper.Success(c)
	})

	g.POST("/sync/stream", func(c *gin.Context) {
		dst := strings.TrimSpace(c.Query("dst"))
		if dst == "" || !strings.HasPrefix(dst, "/") {
			helper.BadRequest(c, errors.New("absolute dst path is required"))
			return
		}
		cleanDst := path.Clean(dst)
		if cleanDst == "/" || strings.HasPrefix(cleanDst, "/proc/") || strings.HasPrefix(cleanDst, "/sys/") || strings.HasPrefix(cleanDst, "/dev/") {
			helper.BadRequest(c, errors.New("forbidden destination path"))
			return
		}
		perm := os.FileMode(0o644)
		if modeStr := strings.TrimSpace(c.Query("mode")); modeStr != "" {
			if parsed, err := strconv.ParseUint(modeStr, 8, 32); err == nil && parsed > 0 {
				perm = os.FileMode(parsed)
			}
		}
		if err := os.MkdirAll(path.Dir(cleanDst), 0o755); err != nil {
			helper.InternalServer(c, err)
			return
		}
		partPath := cleanDst + ".part." + randHexSuffix()
		f, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
		if err != nil {
			helper.InternalServer(c, err)
			return
		}
		_, copyErr := io.Copy(f, c.Request.Body)
		closeErr := f.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(partPath)
			if copyErr != nil {
				helper.InternalServer(c, copyErr)
			} else {
				helper.InternalServer(c, closeErr)
			}
			return
		}
		_ = os.Chmod(partPath, perm)
		if err := os.Rename(partPath, cleanDst); err != nil {
			_ = os.Remove(partPath)
			helper.InternalServer(c, err)
			return
		}
		helper.Success(c)
	})

	g.POST("/ssl-sync", func(c *gin.Context) {
		var req struct {
			MasterSSLID   uint      `json:"masterSslId"`
			PrimaryDomain string    `json:"primaryDomain"`
			Domains       string    `json:"domains"`
			Pem           string    `json:"pem"`
			PrivateKey    string    `json:"privateKey"`
			Organization  string    `json:"organization"`
			Type          string    `json:"type"`
			KeyType       string    `json:"keyType"`
			Description   string    `json:"description"`
			ExpireDate    time.Time `json:"expireDate"`
			StartDate     time.Time `json:"startDate"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			helper.BadRequest(c, err)
			return
		}
		if strings.TrimSpace(req.Pem) == "" || strings.TrimSpace(req.PrivateKey) == "" {
			helper.BadRequest(c, errors.New("certificate pem and private key are required"))
			return
		}
		if block, _ := pem.Decode([]byte(req.PrivateKey)); block == nil {
			helper.BadRequest(c, errors.New("invalid private key pem"))
			return
		}
		var leaf *x509.Certificate
		rest := []byte(req.Pem)
		for len(rest) > 0 {
			block, next := pem.Decode(rest)
			if block == nil {
				break
			}
			if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
				leaf = cert
				if len(cert.DNSNames) > 0 || len(cert.IPAddresses) > 0 {
					break
				}
			}
			rest = next
		}
		if leaf == nil {
			helper.BadRequest(c, errors.New("invalid certificate pem"))
			return
		}
		if req.PrimaryDomain == "" {
			if len(leaf.DNSNames) > 0 {
				req.PrimaryDomain = leaf.DNSNames[0]
				if req.Domains == "" && len(leaf.DNSNames) > 1 {
					req.Domains = strings.Join(leaf.DNSNames[1:], ",")
				}
			} else if len(leaf.IPAddresses) > 0 {
				req.PrimaryDomain = leaf.IPAddresses[0].String()
			} else {
				req.PrimaryDomain = leaf.Subject.CommonName
			}
		}
		if req.ExpireDate.IsZero() {
			req.ExpireDate = leaf.NotAfter
		}
		if req.StartDate.IsZero() {
			req.StartDate = leaf.NotBefore
		}
		if req.Organization == "" {
			if len(leaf.Issuer.Organization) > 0 {
				req.Organization = leaf.Issuer.Organization[0]
			} else {
				req.Organization = leaf.Issuer.CommonName
			}
		}
		if req.Type == "" {
			req.Type = leaf.Issuer.CommonName
		}

		var ssl model.WebsiteSSL
		found := false
		if req.MasterSSLID > 0 {
			if err := global.DB.Where("master_ssl_id = ?", req.MasterSSLID).First(&ssl).Error; err == nil {
				found = true
			}
		}
		if !found {
			if err := global.DB.Where("primary_domain = ?", req.PrimaryDomain).First(&ssl).Error; err == nil {
				found = true
			}
		}
		ssl.PrimaryDomain = req.PrimaryDomain
		ssl.Domains = req.Domains
		ssl.Pem = req.Pem
		ssl.PrivateKey = req.PrivateKey
		ssl.Organization = req.Organization
		ssl.Type = req.Type
		if req.KeyType != "" {
			ssl.KeyType = req.KeyType
		}
		if req.Description != "" {
			ssl.Description = req.Description
		}
		ssl.ExpireDate = req.ExpireDate
		ssl.StartDate = req.StartDate
		ssl.Status = constant.SSLReady
		ssl.Provider = constant.FromMaster
		if req.MasterSSLID > 0 {
			ssl.MasterSSLID = req.MasterSSLID
		}
		if found {
			if err := global.DB.Save(&ssl).Error; err != nil {
				helper.InternalServer(c, err)
				return
			}
			var websites []model.Website
			_ = global.DB.Where("website_ssl_id = ?", ssl.ID).Find(&websites).Error
			if len(websites) > 0 {
				for _, w := range websites {
					sslDir := service.GetSitePath(w, service.SiteSSLDir)
					if err := os.MkdirAll(sslDir, 0o755); err != nil {
						continue
					}
					_ = os.WriteFile(path.Join(sslDir, "fullchain.pem"), []byte(ssl.Pem), 0o644)
					_ = os.WriteFile(path.Join(sslDir, "privkey.pem"), []byte(ssl.PrivateKey), 0o600)
				}
				var openrestyApp model.App
				if err := global.DB.Where("`key` = ?", constant.AppOpenresty).First(&openrestyApp).Error; err == nil {
					var inst model.AppInstall
					if err := global.DB.Where("app_id = ?", openrestyApp.ID).First(&inst).Error; err == nil && inst.Status == constant.StatusRunning && inst.ContainerName != "" {
						_ = cmd.NewCommandMgr(cmd.WithTimeout(20*time.Second)).Run("docker", "exec", "-i", inst.ContainerName, "nginx", "-s", "reload")
					}
				}
			}
		} else {
			if err := global.DB.Create(&ssl).Error; err != nil {
				helper.InternalServer(c, err)
				return
			}
		}
		helper.Success(c)
	})
}
