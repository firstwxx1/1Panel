package node

import (
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/common"
	"github.com/1Panel-dev/1Panel/core/utils/ssh"
)

const (
	defaultAgentPort = 9999
	agentBinaryDest  = "/usr/local/bin/1panel-agent"
)

const systemdUnit = `[Unit]
Description=1Panel Agent (managed node)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/1panel-agent
Restart=on-failure
RestartSec=5
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
`

// agentBinaryPath is the agent executable shipped next to the core binary.
func agentBinaryPath() (string, error) {
	candidates := []string{agentBinaryDest}
	if exe, err := os.Executable(); err == nil {
		candidates = append([]string{filepath.Join(filepath.Dir(exe), "1panel-agent")}, candidates...)
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("1panel-agent binary not found on the master")
}

// unameArch is the `uname -m` value matching the master's own build.
func unameArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "aarch64"
	case "arm":
		return "armv7l"
	default:
		return "x86_64"
	}
}

func heredoc(path, body, tag string) string {
	return fmt.Sprintf("cat > %s <<'%s'\n%s\n%s\n", path, tag, strings.TrimRight(body, "\n"), tag)
}

// bundleScript renders the shell that lays down certificates, node config and the
// systemd unit on a child node. It is shared by the SSH and one-click installs.
func bundleScript(n *model.Node, addr string) (string, error) {
	a, err := loadAuthority()
	if err != nil {
		return "", err
	}
	crt, key, err := a.issue("1panel-node-"+n.Name, []string{addr}, x509.ExtKeyUsageServerAuth)
	if err != nil {
		return "", err
	}
	cfg, _ := json.Marshal(map[string]interface{}{
		"name": n.Name, "addr": addr, "port": n.Port, "baseDir": "/opt/1panel", "version": global.CONF.Base.Version,
	})
	var sb strings.Builder
	sb.WriteString("set -e\numask 077\nmkdir -p /etc/1panel/node /opt/1panel\n")
	sb.WriteString(heredoc("/etc/1panel/node/ca.crt", string(a.caPEM), "__1P_CA__"))
	sb.WriteString(heredoc("/etc/1panel/node/server.crt", string(crt), "__1P_CRT__"))
	sb.WriteString(heredoc("/etc/1panel/node/server.key", string(key), "__1P_KEY__"))
	sb.WriteString(heredoc("/etc/1panel/node.json", string(cfg), "__1P_CFG__"))
	sb.WriteString(fmt.Sprintf("printf '%%s' '%s' > /etc/1panel/.nodeProxyID\n", n.ProxyID))
	sb.WriteString("umask 022\n")
	sb.WriteString(heredoc("/etc/systemd/system/1panel-agent.service", systemdUnit, "__1P_UNIT__"))
	return sb.String(), nil
}

const startScript = "systemctl daemon-reload && systemctl enable 1panel-agent >/dev/null 2>&1 && systemctl restart 1panel-agent"

// installScript is what a one-click command pipes into bash on the child.
func installScript(masterURL, token string) string {
	return fmt.Sprintf(`#!/bin/bash
set -e
MASTER="%s"
TOKEN="%s"
if [ "$(id -u)" != "0" ]; then echo "please run as root"; exit 1; fi
if [ "$(uname -m)" != "%s" ]; then echo "architecture mismatch: master is %s, this machine is $(uname -m)"; exit 1; fi
command -v curl >/dev/null || { echo "curl is required"; exit 1; }
command -v systemctl >/dev/null || { echo "systemd is required"; exit 1; }
echo "[1/3] downloading agent..."
curl -fsSLk "$MASTER/api/v2/core/nodejoin/agent?token=$TOKEN" -o %s.new
chmod +x %s.new
echo "[2/3] fetching credentials..."
curl -fsSLk "$MASTER/api/v2/core/nodejoin/bundle?token=$TOKEN&addr=${NODE_ADDR:-}" -o /tmp/1panel-node-bundle.sh
bash /tmp/1panel-node-bundle.sh
rm -f /tmp/1panel-node-bundle.sh
echo "[3/3] starting agent..."
mv -f %s.new %s
%s
echo "1Panel agent installed. The node will show online in the master shortly."
`, masterURL, token, unameArch(), unameArch(), agentBinaryDest, agentBinaryDest, agentBinaryDest, agentBinaryDest, startScript)
}

type SSHInstall struct {
	Addr       string `json:"addr"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	AuthMode   string `json:"authMode"`
	Password   string `json:"password"`
	PrivateKey string `json:"privateKey"`
	PassPhrase string `json:"passPhrase"`
}

func setStatus(id uint, status, message string) {
	_ = global.DB.Model(&model.Node{}).Where("id = ?", id).Updates(map[string]interface{}{"status": status, "message": message}).Error
}

// installViaSSH pushes the agent, certificates and unit to the machine and starts it.
func installViaSSH(id uint, req SSHInstall) {
	fail := func(err error) {
		global.LOG.Errorf("install node %d via ssh failed: %v", id, err)
		setStatus(id, statusFailed, err.Error())
	}
	var n model.Node
	if err := global.DB.First(&n, id).Error; err != nil {
		return
	}
	client, err := ssh.NewClient(ssh.ConnInfo{
		User: req.User, Addr: req.Addr, Port: req.Port, AuthMode: req.AuthMode,
		Password: req.Password, PrivateKey: []byte(req.PrivateKey), PassPhrase: []byte(req.PassPhrase),
		DialTimeOut: 15 * time.Second,
	})
	if err != nil {
		fail(fmt.Errorf("ssh connect failed: %w", err))
		return
	}
	defer client.Close()
	sudo := client.SudoHandleCmd()
	if req.User != "root" && sudo == "" {
		fail(errors.New("a root user or passwordless sudo is required"))
		return
	}
	run := func(cmd string) (string, error) {
		session, err := client.Client.NewSession()
		if err != nil {
			return "", err
		}
		defer session.Close()
		out, err := session.CombinedOutput(cmd)
		return string(out), err
	}
	if out, err := run("uname -m"); err != nil || strings.TrimSpace(out) != unameArch() {
		fail(fmt.Errorf("architecture mismatch: master is %s, node reports %q", unameArch(), strings.TrimSpace(out)))
		return
	}
	binPath, err := agentBinaryPath()
	if err != nil {
		fail(err)
		return
	}
	f, err := os.Open(binPath)
	if err != nil {
		fail(err)
		return
	}
	defer f.Close()
	upload, err := client.Client.NewSession()
	if err != nil {
		fail(err)
		return
	}
	upload.Stdin = f
	err = upload.Run(fmt.Sprintf("%smkdir -p /usr/local/bin && %stee %s.new >/dev/null && %schmod +x %s.new", sudo, sudo, agentBinaryDest, sudo, agentBinaryDest))
	upload.Close()
	if err != nil {
		fail(fmt.Errorf("upload agent failed: %w", err))
		return
	}
	script, err := bundleScript(&n, req.Addr)
	if err != nil {
		fail(err)
		return
	}
	bundle, err := client.Client.NewSession()
	if err != nil {
		fail(err)
		return
	}
	bundle.Stdin = strings.NewReader(script)
	out, err := bundle.CombinedOutput(sudo + "bash -s")
	bundle.Close()
	if err != nil {
		fail(fmt.Errorf("write node config failed: %s %w", strings.TrimSpace(string(out)), err))
		return
	}
	if out, err := run(fmt.Sprintf("%sbash -c 'mv -f %s.new %s && %s'", sudo, agentBinaryDest, agentBinaryDest, startScript)); err != nil {
		fail(fmt.Errorf("start agent failed: %s %w", strings.TrimSpace(out), err))
		return
	}
	// the collector flips the node to Healthy once its agent answers
	_ = global.DB.Model(&model.Node{}).Where("id = ?", id).Updates(map[string]interface{}{"addr": req.Addr, "status": statusInstalling, "message": ""}).Error
}

func newSecret(n int) string { return common.RandStr(n) }
