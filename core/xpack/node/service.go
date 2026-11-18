package node

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
)

// NodeView is the shape every node page and the node switcher consume.
type NodeView struct {
	ID          uint   `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Addr        string `json:"addr"`
	Port        int    `json:"port"`
	GroupID     uint   `json:"groupID"`
	GroupBelong string `json:"groupBelong"`
	Description string `json:"description"`
	IsFavorite  bool   `json:"isFavorite"`
	IsLocal     bool   `json:"isLocal"`
	IsXpack     bool   `json:"isXpack"`
	IsBound     bool   `json:"isBound"`

	Status        string `json:"status"`
	Message       string `json:"message"`
	Version       string `json:"version"`
	SystemVersion string `json:"systemVersion"`
	SyncStatus    string `json:"syncStatus"`

	Protocol         string `json:"protocol"`
	SecurityEntrance string `json:"securityEntrance"`
	HasAPIKey        bool   `json:"hasApiKey"`

	CPUTotal          int       `json:"cpuTotal"`
	CPUUsedPercent    float64   `json:"cpuUsedPercent"`
	MemoryTotal       uint64    `json:"memoryTotal"`
	MemoryUsedPercent float64   `json:"memoryUsedPercent"`
	Load1             float64   `json:"load1"`
	CreatedAt         time.Time `json:"createdAt"`
}

var (
	nameRule     = regexp.MustCompile(`^[\p{Han}A-Za-z0-9_\-]{1,30}$`)
	hostnameRule = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9\-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9\-]{0,61}[A-Za-z0-9])?)*$`)
)

func validateEndpointAddr(raw string) error {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return errors.New("invalid address")
	}
	for _, r := range raw {
		if r <= 0x20 || r == 0x7f || strings.ContainsRune("/?#@'\"`\\", r) {
			return errors.New("invalid characters in address")
		}
	}
	host := raw
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	if net.ParseIP(host) != nil {
		return nil
	}
	if strings.Contains(host, ":") || !hostnameRule.MatchString(host) {
		return errors.New("invalid hostname or IP address")
	}
	return nil
}

func normalizeJoinAddr(raw string) (string, error) {
	addr := strings.TrimSpace(raw)
	if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
		addr = addr[1 : len(addr)-1]
	}
	if addr == "" {
		return "", errors.New("cannot determine node address")
	}
	for _, r := range addr {
		if r <= 0x20 || r == 0x7f || strings.ContainsRune("/?#@'\"`\\", r) {
			return "", errors.New("invalid node address")
		}
	}
	if ip := net.ParseIP(addr); ip != nil {
		if ip.IsLoopback() || ip.IsUnspecified() {
			return "", errors.New("loopback or unspecified address is not allowed for child node")
		}
		return ip.String(), nil
	}
	if strings.Contains(addr, ":") || strings.EqualFold(addr, "localhost") || !hostnameRule.MatchString(addr) {
		return "", errors.New("invalid node hostname")
	}
	return addr, nil
}

func groupNames() map[uint]string {
	var groups []model.Group
	_ = global.DB.Where("type = ?", "node").Find(&groups).Error
	out := map[uint]string{}
	for _, g := range groups {
		name := g.Name
		if g.IsDefault && (strings.EqualFold(name, "default") || name == "") {
			name = "默认"
		}
		out[g.ID] = name
	}
	return out
}

func defaultGroupID() uint {
	var g model.Group
	if err := global.DB.Where("type = ? AND is_default = ?", "node", true).First(&g).Error; err != nil {
		return 0
	}
	return g.ID
}

func view(n model.Node, names map[uint]string) NodeView {
	v := NodeView{
		ID: n.ID, Kind: n.Kind, Name: n.Name, Addr: n.Addr, Port: n.Port, GroupID: n.GroupID, GroupBelong: names[n.GroupID],
		Description: n.Description, IsFavorite: n.IsFavorite, IsXpack: true, IsBound: true,
		Status: n.Status, Message: n.Message, Version: n.Version, SystemVersion: n.SystemVersion, SyncStatus: "success",
		Protocol: n.Protocol, SecurityEntrance: n.Entrance, HasAPIKey: n.APIKey != "",
		CPUTotal: n.CPUTotal, CPUUsedPercent: n.CPUUsedPercent, MemoryTotal: n.MemoryTotal,
		MemoryUsedPercent: n.MemoryUsedPercent, Load1: n.Load1, CreatedAt: n.CreatedAt,
	}
	if n.Status == statusPending {
		v.Status = statusPending
	}
	return v
}

// localView is the master itself, always listed first.
func localView(names map[uint]string) NodeView {
	localMu.RLock()
	v := localInfo
	localMu.RUnlock()
	v.IsXpack, v.IsBound = true, true
	v.SyncStatus = "success"
	v.GroupID = defaultGroupID()
	v.GroupBelong = names[v.GroupID]
	if alias := strings.TrimSpace(UIGetSettings()["masterAlias"]); alias != "" {
		v.Description = alias
	} else {
		var dbAlias string
		if err := global.DB.Table("settings").Select("value").Where("key = ?", "MasterAlias").Scan(&dbAlias).Error; err == nil {
			v.Description = strings.TrimSpace(dbAlias)
		}
	}
	return v
}

// List returns the master plus every child node (kind "node") for the switcher.
func List() ([]NodeView, error) {
	var rows []model.Node
	if err := global.DB.Where("kind = ?", kindNode).Order("is_favorite DESC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	names := groupNames()
	out := []NodeView{localView(names)}
	for _, n := range rows {
		out = append(out, view(n, names))
	}
	return out, nil
}

// SimplePanelView matches the dashboard panel carousel item shape expected by home/index.vue.
type SimplePanelView struct {
	ID                uint    `json:"id"`
	Name              string  `json:"name"`
	Addr              string  `json:"addr"`
	Port              int     `json:"port"`
	Protocol          string  `json:"protocol"`
	Status            string  `json:"status"`
	Message           string  `json:"message"`
	Description       string  `json:"description"`
	Version           string  `json:"version"`
	SystemVersion     string  `json:"systemVersion"`
	SecurityEntrance  string  `json:"securityEntrance"`
	CPUTotal          int     `json:"cpuTotal"`
	CPUUsedPercent    float64 `json:"cpuUsedPercent"`
	MemoryTotal       uint64  `json:"memoryTotal"`
	MemoryUsedPercent float64 `json:"memoryUsedPercent"`
	Load1             float64 `json:"load1"`
}

func ListSimplePanels() ([]SimplePanelView, error) {
	var rows []model.Node
	if err := global.DB.Where("kind = ?", kindPanel).Order("is_favorite DESC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]SimplePanelView, 0, len(rows))
	for _, n := range rows {
		out = append(out, SimplePanelView{
			ID:                n.ID,
			Name:              n.Name,
			Addr:              panelURL(n),
			Port:              n.Port,
			Protocol:          n.Protocol,
			Status:            n.Status,
			Message:           n.Message,
			Description:       n.Description,
			Version:           n.Version,
			SystemVersion:     n.SystemVersion,
			SecurityEntrance:  n.Entrance,
			CPUTotal:          n.CPUTotal,
			CPUUsedPercent:    n.CPUUsedPercent,
			MemoryTotal:       n.MemoryTotal,
			MemoryUsedPercent: n.MemoryUsedPercent,
			Load1:             n.Load1,
		})
	}
	return out, nil
}

type SearchReq struct {
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	Info     string `json:"info"`
	GroupID  uint   `json:"groupID"`
	Kind     string `json:"kind"`
}

type SearchRes struct {
	Total int64      `json:"total"`
	Items []NodeView `json:"items"`
}

// Search pages the node (or panel) table; the master row leads the first page of nodes.
func Search(req SearchReq) (*SearchRes, error) {
	if req.Kind == "" {
		req.Kind = kindNode
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 {
		req.PageSize = 20
	}
	db := global.DB.Model(&model.Node{}).Where("kind = ?", req.Kind)
	if req.Info != "" {
		like := "%" + req.Info + "%"
		db = db.Where("name LIKE ? OR addr LIKE ? OR description LIKE ?", like, like, like)
	}
	if req.GroupID != 0 {
		db = db.Where("group_id = ?", req.GroupID)
	}
	res := &SearchRes{Items: []NodeView{}}
	if err := db.Count(&res.Total).Error; err != nil {
		return nil, err
	}
	names := groupNames()
	includeLocal := false
	var lv NodeView
	if req.Kind == kindNode {
		lv = localView(names)
		groupOK := req.GroupID == 0 || req.GroupID == lv.GroupID
		infoOK := req.Info == ""
		if !infoOK {
			q := strings.ToLower(req.Info)
			infoOK = strings.Contains(strings.ToLower(lv.Name), q) ||
				strings.Contains(strings.ToLower(lv.Addr), q) ||
				strings.Contains(strings.ToLower(lv.Description), q)
		}
		includeLocal = groupOK && infoOK
	}
	limit := req.PageSize
	offset := (req.Page - 1) * req.PageSize
	if includeLocal {
		res.Total++
		if req.Page == 1 {
			res.Items = append(res.Items, lv)
			limit = req.PageSize - 1
			offset = 0
		} else {
			offset = (req.Page-1)*req.PageSize - 1
		}
	}
	var rows []model.Node
	if limit > 0 {
		if err := db.Order("is_favorite DESC, id ASC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
			return nil, err
		}
	}
	for _, n := range rows {
		res.Items = append(res.Items, view(n, names))
	}
	return res, nil
}

func validateName(name string, exceptID uint) error {
	if name == "local" || !nameRule.MatchString(name) {
		return errors.New("invalid node name")
	}
	var count int64
	global.DB.Model(&model.Node{}).Where("name = ? AND id <> ?", name, exceptID).Count(&count)
	if count > 0 {
		return fmt.Errorf("name %s already exists", name)
	}
	return nil
}

type CreateReq struct {
	Name        string `json:"name"`
	Mode        string `json:"mode"`
	GroupID     uint   `json:"groupID"`
	Description string `json:"description"`
	AgentPort   int    `json:"agentPort"`
	// SSH installs (nested or flat when Mode == "ssh")
	SSH        *SSHInstall `json:"ssh"`
	User       string      `json:"user"`
	AuthMode   string      `json:"authMode"`
	Password   string      `json:"password"`
	PrivateKey string      `json:"privateKey"`
	PassPhrase string      `json:"passPhrase"`
	// panels
	Kind     string `json:"kind"`
	Addr     string `json:"addr"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Entrance string `json:"entrance"`
	APIKey   string `json:"apiKey"`
}

type CreateRes struct {
	ID    uint   `json:"id"`
	Token string `json:"token"`
}

// Create registers a node. With ssh set the master installs the agent itself; otherwise
// the node stays pending and a one-click command (built from Token) finishes the join.
func Create(req CreateReq) (*CreateRes, error) {
	if err := validateName(req.Name, 0); err != nil {
		return nil, err
	}
	if req.GroupID == 0 {
		req.GroupID = defaultGroupID()
	}
	if req.Kind == kindPanel {
		return createPanel(req)
	}
	if req.AgentPort <= 0 || req.AgentPort > 65535 {
		req.AgentPort = defaultAgentPort
	}
	if _, err := loadAuthority(); err != nil {
		return nil, err
	}
	if req.Mode == "ssh" && req.SSH == nil {
		req.SSH = &SSHInstall{
			Addr:       req.Addr,
			Port:       req.Port,
			User:       req.User,
			AuthMode:   req.AuthMode,
			Password:   req.Password,
			PrivateKey: req.PrivateKey,
			PassPhrase: req.PassPhrase,
		}
	}
	n := model.Node{
		Name: req.Name, Kind: kindNode, GroupID: req.GroupID, Description: req.Description, Port: req.AgentPort,
		ProxyID: newSecret(32), Status: statusPending,
	}
	if req.SSH != nil {
		req.SSH.Addr = strings.TrimSpace(req.SSH.Addr)
		req.SSH.User = strings.TrimSpace(req.SSH.User)
		if req.SSH.Addr == "" || req.SSH.User == "" {
			return nil, errors.New("ssh address and user are required")
		}
		if err := validateEndpointAddr(req.SSH.Addr); err != nil {
			return nil, err
		}
		authMode := req.SSH.AuthMode
		if authMode == "" {
			authMode = "password"
			req.SSH.AuthMode = authMode
		}
		switch authMode {
		case "password":
			if req.SSH.Password == "" {
				return nil, errors.New("ssh password is required")
			}
		case "key":
			if strings.TrimSpace(req.SSH.PrivateKey) == "" {
				return nil, errors.New("ssh private key is required")
			}
		default:
			return nil, errors.New("invalid ssh auth mode")
		}
		if req.SSH.Port <= 0 || req.SSH.Port > 65535 {
			req.SSH.Port = 22
		}
		n.Addr = req.SSH.Addr
		n.Status = statusInstalling
	} else {
		n.JoinToken = newSecret(40)
	}
	if err := global.DB.Create(&n).Error; err != nil {
		return nil, err
	}
	if req.SSH != nil {
		go installViaSSH(n.ID, *req.SSH)
	}
	return &CreateRes{ID: n.ID, Token: n.JoinToken}, nil
}

func createPanel(req CreateReq) (*CreateRes, error) {
	req.Addr = strings.TrimSpace(req.Addr)
	if req.Addr == "" || req.Port <= 0 || req.Port > 65535 {
		return nil, errors.New("panel address and valid port are required")
	}
	if err := validateEndpointAddr(req.Addr); err != nil {
		return nil, err
	}
	proto := req.Protocol
	if proto == "" {
		proto = "http"
	}
	if proto != "http" && proto != "https" {
		return nil, errors.New("protocol must be http or https")
	}
	key := ""
	if req.APIKey != "" {
		enc, err := encrypt.StringEncrypt(req.APIKey)
		if err != nil {
			return nil, err
		}
		key = enc
	}
	n := model.Node{
		Name: req.Name, Kind: kindPanel, GroupID: req.GroupID, Description: req.Description,
		Addr: req.Addr, Port: req.Port, Protocol: proto, Entrance: strings.Trim(req.Entrance, "/"), APIKey: key, Status: statusUnhealthy,
	}
	if err := global.DB.Create(&n).Error; err != nil {
		return nil, err
	}
	go checkPanel(n)
	return &CreateRes{ID: n.ID}, nil
}

type UpdateReq struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	GroupID     uint   `json:"groupID"`
	Description string `json:"description"`
	Addr        string `json:"addr"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
	Entrance    string `json:"entrance"`
	APIKey      string `json:"apiKey"`
}

func Update(req UpdateReq) error {
	var n model.Node
	if err := global.DB.First(&n, req.ID).Error; err != nil {
		return err
	}
	if err := validateName(req.Name, n.ID); err != nil {
		return err
	}
	update := map[string]interface{}{"name": req.Name, "description": req.Description}
	if req.GroupID != 0 {
		update["group_id"] = req.GroupID
	}
	if n.Kind == kindPanel {
		if req.Addr != "" {
			req.Addr = strings.TrimSpace(req.Addr)
			if err := validateEndpointAddr(req.Addr); err != nil {
				return err
			}
			update["addr"] = req.Addr
		}
		if req.Port != 0 {
			if req.Port < 1 || req.Port > 65535 {
				return errors.New("invalid panel port")
			}
			update["port"] = req.Port
		}
		if req.Protocol != "" {
			if req.Protocol != "http" && req.Protocol != "https" {
				return errors.New("protocol must be http or https")
			}
			update["protocol"] = req.Protocol
		}
		update["entrance"] = strings.Trim(req.Entrance, "/")
		if req.APIKey != "" {
			enc, err := encrypt.StringEncrypt(req.APIKey)
			if err != nil {
				return err
			}
			update["api_key"] = enc
		}
	}
	return global.DB.Model(&model.Node{}).Where("id = ?", n.ID).Updates(update).Error
}

// Delete removes nodes; a reachable agent is told to forget this master first so it
// refuses any later request.
func Delete(ids []uint) error {
	var rows []model.Node
	if err := global.DB.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return err
	}
	for _, n := range rows {
		if n.Kind == kindNode && n.Addr != "" && n.Status != statusPending {
			_, _ = agentCall(n, "POST", "/api/v2/xpack/node/unbind", 5*time.Second)
		}
	}
	if err := global.DB.Where("id IN ?", ids).Delete(&model.Node{}).Error; err != nil {
		return err
	}
	Default.resetTransport()
	return nil
}

// Test probes the node right now and stores the outcome.
func Test(id uint) error {
	var n model.Node
	if err := global.DB.First(&n, id).Error; err != nil {
		return err
	}
	if n.Addr == "" {
		return errors.New("node has not joined yet")
	}
	if err := probeNow(n); err != nil {
		return err
	}
	if n.Kind == kindPanel {
		checkPanel(n)
	} else {
		checkNode(n)
	}
	return nil
}

func SetFavorite(id uint, fav bool) error {
	return global.DB.Model(&model.Node{}).Where("id = ?", id).Update("is_favorite", fav).Error
}

func ChangeGroup(ids []uint, groupID uint) error {
	return global.DB.Model(&model.Node{}).Where("id IN ?", ids).Update("group_id", groupID).Error
}

// Restart asks the node's agent to restart itself.
func Restart(id uint) error {
	var n model.Node
	if err := global.DB.First(&n, id).Error; err != nil {
		return err
	}
	if n.Kind != kindNode || n.Addr == "" {
		return errors.New("not a joined node")
	}
	_, err := agentCall(n, "POST", "/api/v2/xpack/node/restart", 8*time.Second)
	return err
}

// --- one-click join (token authenticated, no session) ---

func tokenExpired(n *model.Node) bool {
	ref := n.UpdatedAt
	if ref.IsZero() {
		ref = n.CreatedAt
	}
	return time.Since(ref) > 24*time.Hour
}

func nodeByToken(token string) (*model.Node, error) {
	if len(token) < 20 {
		return nil, errors.New("invalid token")
	}
	var n model.Node
	if err := global.DB.Where("join_token = ? AND kind = ?", token, kindNode).First(&n).Error; err != nil {
		return nil, errors.New("invalid or used token")
	}
	if tokenExpired(&n) {
		return nil, errors.New("token expired")
	}
	return &n, nil
}

// GetJoinCommandByID returns (and refreshes if expired/empty) the one-click join command for a Pending node.
func GetJoinCommandByID(id uint, master string) (string, error) {
	var n model.Node
	if err := global.DB.First(&n, id).Error; err != nil {
		return "", err
	}
	if n.Kind != kindNode || (n.Status != statusPending && n.JoinToken == "") {
		return "", errors.New("node is not pending join")
	}
	token := n.JoinToken
	if token == "" || tokenExpired(&n) {
		token = newSecret(40)
		if err := global.DB.Model(&model.Node{}).Where("id = ?", n.ID).
			Updates(map[string]interface{}{"join_token": token, "updated_at": time.Now()}).Error; err != nil {
			return "", err
		}
	}
	target := strings.TrimRight(master, "/") + "/api/v2/core/nodejoin/script?token=" + url.QueryEscape(token)
	return fmt.Sprintf("curl -fsSLk %q | bash", target), nil
}

func JoinScript(masterURL, token string) (string, error) {
	if _, err := nodeByToken(token); err != nil {
		return "", err
	}
	return installScript(masterURL, token), nil
}

func JoinAgentPath(token string) (string, error) {
	if _, err := nodeByToken(token); err != nil {
		return "", err
	}
	return agentBinaryPath()
}

// JoinBundle consumes the token atomically before issuing node credentials.
func JoinBundle(token, addr string) (string, error) {
	n, err := nodeByToken(token)
	if err != nil {
		return "", err
	}
	cleanAddr, err := normalizeJoinAddr(addr)
	if err != nil {
		return "", err
	}
	res := global.DB.Model(&model.Node{}).
		Where("id = ? AND join_token = ?", n.ID, token).
		Updates(map[string]interface{}{"addr": cleanAddr, "join_token": "", "status": statusInstalling})
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected != 1 {
		return "", errors.New("invalid or used token")
	}
	script, err := bundleScript(n, cleanAddr)
	if err != nil {
		return "", err
	}
	return script, nil
}

// Start boots the background collector once the database is ready.
func Start() { Default.StartBackground() }
