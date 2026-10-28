package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
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

func localSSHPort() int {
	data, err := os.ReadFile(sshPath)
	if err != nil {
		return 22
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.EqualFold(f[0], "Port") {
			if p, err := strconv.Atoi(f[1]); err == nil && p > 0 {
				return p
			}
		}
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
	old, _ := os.ReadFile(akPath)
	prefix := string(old)
	if prefix != "" && !strings.HasSuffix(prefix, "\n") {
		prefix += "\n"
	}
	if err := os.WriteFile(akPath, []byte(prefix+line), 0o600); err != nil {
		return err
	}

	port := localSSHPort()
	client, err := ssh.NewClient(ssh.ConnInfo{User: "root", Addr: "127.0.0.1", Port: port, AuthMode: "key", PrivateKey: privPEM})
	if err != nil { // sshd refuses root key login: undo and leave manual setup to the user
		_ = os.WriteFile(akPath, old, 0o600)
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
