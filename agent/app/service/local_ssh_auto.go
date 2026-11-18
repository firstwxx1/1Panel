package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
	"github.com/1Panel-dev/1Panel/agent/utils/ssh"
	gossh "golang.org/x/crypto/ssh"
)

const autoKeyComment = "1panel-local-terminal"

var autoLocalSSHOnce sync.Once

// EnsureLocalSSH sets up the terminal's "local connection" without asking for a password:
// it creates a key pair on this machine, authorizes it for root and stores it as the local
// connection. It does nothing when a connection is already configured or the user turned the
// local terminal off.
func EnsureLocalSSH() {
	autoLocalSSHOnce.Do(func() {
		defer func() { _ = recover() }()
		time.Sleep(10 * time.Second) // let the database and sshd settle after boot
		if os.Geteuid() != 0 {
			return
		}
		if show, _ := settingRepo.GetValueByKey("LocalSSHConnShow"); show == constant.StatusDisable {
			return
		}
		if conn, _ := settingRepo.GetValueByKey("LocalSSHConn"); conn != "" && localConnWorks(conn) {
			return
		}
		if err := setupLocalSSH(); err != nil {
			global.LOG.Warnf("auto local ssh: %v", err)
		}
	})
}

// localConnWorks reports whether the stored connection still logs in; a stale one is replaced.
func localConnWorks(enc string) bool {
	plain, err := encrypt.StringDecrypt(enc)
	if err != nil {
		return false
	}
	var item model.LocalConnInfo
	if json.Unmarshal([]byte(plain), &item) != nil {
		return false
	}
	info := ssh.ConnInfo{User: item.User, Addr: item.Addr, Port: int(item.Port), AuthMode: item.AuthMode, Password: item.Password, PrivateKey: []byte(item.PrivateKey)}
	if item.PassPhrase != "" {
		info.PassPhrase = []byte(item.PassPhrase)
	}
	c, err := ssh.NewClient(info)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func sshPortFromDropIns() (int, bool) {
	files, err := filepath.Glob("/etc/ssh/sshd_config.d/*.conf")
	if err != nil || len(files) == 0 {
		return 0, false
	}
	sort.Strings(files)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if p, ok := parseSSHPortBytes(data, false); ok {
			return p, true
		}
	}
	return 0, false
}

func parseSSHPortBytes(data []byte, followInclude bool) (int, bool) {
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		if strings.EqualFold(f[0], "Match") {
			break
		}
		if followInclude && strings.EqualFold(f[0], "Include") && strings.Contains(f[1], "sshd_config.d") {
			if p, ok := sshPortFromDropIns(); ok {
				return p, true
			}
		}
		if strings.EqualFold(f[0], "Port") {
			if p, err := strconv.Atoi(f[1]); err == nil && p > 0 && p <= 65535 {
				return p, true
			}
		}
	}
	return 0, false
}

func localSSHPort() int {
	if data, err := os.ReadFile(sshPath); err == nil {
		if p, ok := parseSSHPortBytes(data, true); ok {
			return p
		}
	}
	if p, ok := sshPortFromDropIns(); ok {
		return p
	}
	return 22
}

func setupLocalSSH() error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		return err
	}
	line := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(sshPub))) + " " + autoKeyComment + "\n"

	dir := "/root/.ssh"
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	akPath := filepath.Join(dir, "authorized_keys")
	old, readErr := os.ReadFile(akPath)
	oldExists := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	var kept []string
	if oldExists {
		for _, existing := range strings.Split(string(old), "\n") {
			if strings.Contains(existing, autoKeyComment) {
				continue
			}
			kept = append(kept, existing)
		}
	}
	prefix := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	if prefix != "" {
		prefix += "\n"
	}
	if err := os.WriteFile(akPath, []byte(prefix+line), 0o600); err != nil {
		return err
	}

	port := localSSHPort()
	client, err := ssh.NewClient(ssh.ConnInfo{User: "root", Addr: "127.0.0.1", Port: port, AuthMode: "key", PrivateKey: privPEM})
	if err != nil { // sshd refuses root key login: undo and leave manual setup to the user
		if oldExists {
			_ = os.WriteFile(akPath, old, 0o600)
		} else {
			_ = os.Remove(akPath)
		}
		return err
	}
	client.Close()

	item := model.LocalConnInfo{Addr: "127.0.0.1", Port: uint(port), User: "root", AuthMode: "key", PrivateKey: string(privPEM)}
	b, _ := json.Marshal(&item)
	enc, err := encrypt.StringEncrypt(string(b))
	if err != nil {
		return err
	}
	return settingRepo.Update("LocalSSHConn", enc)
}
