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
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/app/service"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/i18n"
	"github.com/1Panel-dev/1Panel/agent/utils/cmd"
	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
	"github.com/1Panel-dev/1Panel/agent/utils/terminal"
	"github.com/1Panel-dev/1Panel/agent/xpack/appsync"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// NodeRouter holds the endpoints a master uses to manage a child node's agent.
type NodeRouter struct{}

type stagedScript struct {
	script    string
	expiresAt time.Time
}

var (
	stagedScriptsMu sync.Mutex
	stagedScripts   = make(map[string]stagedScript)
)

func randHexToken(byteLen int) string {
	b := make([]byte, byteLen)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

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
		ver := strings.TrimSpace(c.Query("version"))
		if ver == "" {
			ver = strings.TrimSpace(c.GetHeader("X-Version"))
		}
		if ver != "" {
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
			Language       *string                `json:"language"`
			Edition        *string                `json:"edition"`
			SystemProxy    *struct {
				Type              string `json:"type"`
				URL               string `json:"url"`
				Port              string `json:"port"`
				User              string `json:"user"`
				Passwd            string `json:"passwd"`
				PasswdKeep        string `json:"passwdKeep"`
				ProxyDocker       string `json:"proxyDocker"`
				DockerProxyURL    string `json:"dockerProxyUrl"`
				WithDockerRestart bool   `json:"withDockerRestart"`
			} `json:"systemProxy"`
			GroupUpdate *struct {
				Type  string `json:"type"`
				OldID uint   `json:"oldId"`
				NewID uint   `json:"newId"`
			} `json:"groupUpdate"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			helper.BadRequest(c, err)
			return
		}
		settingRepo := repo.NewISettingRepo()
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
		if req.Language != nil {
			lang := strings.TrimSpace(*req.Language)
			if err := settingRepo.UpdateOrCreate("Language", lang); err != nil {
				helper.InternalServer(c, err)
				return
			}
			i18n.SetLanguage(lang)
		}
		if req.Edition != nil {
			edition := strings.TrimSpace(*req.Edition)
			if err := settingRepo.UpdateOrCreate("Edition", edition); err != nil {
				helper.InternalServer(c, err)
				return
			}
			global.CONF.Base.Edition = edition
		}
		if req.SystemProxy != nil {
			p := req.SystemProxy
			encPasswd := ""
			if p.Passwd != "" {
				var err error
				encPasswd, err = encrypt.StringEncrypt(p.Passwd)
				if err != nil {
					helper.InternalServer(c, err)
					return
				}
			}
			for _, kv := range [][2]string{
				{"ProxyType", p.Type},
				{"ProxyUrl", p.URL},
				{"ProxyPort", p.Port},
				{"ProxyUser", p.User},
				{"ProxyPasswd", encPasswd},
				{"ProxyPasswdKeep", p.PasswdKeep},
			} {
				if err := settingRepo.UpdateOrCreate(kv[0], kv[1]); err != nil {
					helper.InternalServer(c, err)
					return
				}
			}
			if _, err := os.Stat("/etc/systemd/system"); err == nil {
				const dockerProxyConf = "/etc/systemd/system/docker.service.d/http-proxy.conf"
				if p.ProxyDocker == "Enable" && strings.TrimSpace(p.DockerProxyURL) != "" {
					u := strings.ReplaceAll(strings.TrimSpace(p.DockerProxyURL), `"`, `\"`)
					if err := os.MkdirAll(filepath.Dir(dockerProxyConf), 0o755); err == nil {
						content := fmt.Sprintf("[Service]\nEnvironment=\"HTTP_PROXY=%s\"\nEnvironment=\"HTTPS_PROXY=%s\"\nEnvironment=\"NO_PROXY=localhost,127.0.0.1,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16\"\n", u, u)
						_ = os.WriteFile(dockerProxyConf, []byte(content), 0o644)
						_ = exec.Command("systemctl", "daemon-reload").Run()
					}
				} else if p.ProxyDocker == "Disable" {
					if _, err := os.Stat(dockerProxyConf); err == nil {
						_ = os.Remove(dockerProxyConf)
					}
					_ = exec.Command("systemctl", "daemon-reload").Run()
				}
				if p.WithDockerRestart {
					_ = exec.Command("systemctl", "restart", "docker").Run()
				}
			}
		}
		if req.GroupUpdate != nil && global.DB != nil {
			switch req.GroupUpdate.Type {
			case "website":
				if err := global.DB.Model(&model.Website{}).Where("website_group_id = ?", req.GroupUpdate.OldID).Update("website_group_id", req.GroupUpdate.NewID).Error; err != nil {
					helper.InternalServer(c, err)
					return
				}
			case "cronjob":
				if err := global.DB.Model(&model.Cronjob{}).Where("group_id = ?", req.GroupUpdate.OldID).Update("group_id", req.GroupUpdate.NewID).Error; err != nil {
					helper.InternalServer(c, err)
					return
				}
			case "host":
				if err := global.DB.Model(&model.Host{}).Where("group_id = ?", req.GroupUpdate.OldID).Update("group_id", req.GroupUpdate.NewID).Error; err != nil {
					helper.InternalServer(c, err)
					return
				}
			}
		}
		helper.Success(c)
	})

	g.POST("/script-stage", func(c *gin.Context) {
		var req struct {
			Script string `json:"script"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			helper.BadRequest(c, err)
			return
		}
		if strings.TrimSpace(req.Script) == "" {
			helper.BadRequest(c, errors.New("script is required"))
			return
		}
		tok := randHexToken(16)
		now := time.Now()
		stagedScriptsMu.Lock()
		for k, v := range stagedScripts {
			if now.After(v.expiresAt) {
				delete(stagedScripts, k)
			}
		}
		stagedScripts[tok] = stagedScript{
			script:    req.Script,
			expiresAt: now.Add(60 * time.Second),
		}
		stagedScriptsMu.Unlock()
		helper.SuccessWithData(c, gin.H{"token": tok})
	})

	g.GET("/script-run", func(c *gin.Context) {
		tok := strings.TrimSpace(c.Query("token"))
		now := time.Now()
		stagedScriptsMu.Lock()
		for k, v := range stagedScripts {
			if now.After(v.expiresAt) {
				delete(stagedScripts, k)
			}
		}
		item, ok := stagedScripts[tok]
		if ok {
			delete(stagedScripts, tok)
		}
		stagedScriptsMu.Unlock()
		if tok == "" || !ok || now.After(item.expiresAt) {
			helper.BadRequest(c, errors.New("invalid or expired script token"))
			return
		}
		cols, err := strconv.Atoi(c.DefaultQuery("cols", "80"))
		if err != nil || cols <= 0 {
			cols = 80
		}
		rows, err := strconv.Atoi(c.DefaultQuery("rows", "40"))
		if err != nil || rows <= 0 {
			rows = 40
		}
		wsUpgrader := websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 16384,
			CheckOrigin:     func(*http.Request) bool { return true },
		}
		wsConn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer wsConn.Close()

		writeWsCloseErr := func(err error) {
			reason := err.Error()
			if len(reason) > 120 {
				reason = reason[:120]
			}
			_ = wsConn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, reason), time.Now().Add(time.Second))
		}

		tmpPath := "/tmp/1panel-script-" + randHexToken(8) + ".sh"
		if err := os.WriteFile(tmpPath, []byte(item.script), 0o700); err != nil {
			writeWsCloseErr(err)
			return
		}
		defer os.Remove(tmpPath)

		slave, err := terminal.NewCommand("bash")
		if err != nil {
			writeWsCloseErr(err)
			return
		}
		defer slave.Close()

		tty, err := terminal.NewLocalWsSession(cols, rows, wsConn, slave, true)
		if err != nil {
			writeWsCloseErr(err)
			return
		}

		quitChan := make(chan bool, 3)
		tty.Start(quitChan)
		go slave.Wait(quitChan)
		time.Sleep(100 * time.Millisecond)
		_, _ = slave.Write([]byte("bash " + tmpPath + "\n"))
		<-quitChan
		_ = wsConn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
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
