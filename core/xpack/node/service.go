package node

import (
	"errors"
	"fmt"
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

var nameRule = regexp.MustCompile(`^[\p{Han}A-Za-z0-9_\-]{1,30}$`)

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
	var alias string
	if err := global.DB.Table("settings").Select("value").Where("key = ?", "MasterAlias").Scan(&alias).Error; err == nil {
		v.Description = ""
	}
	return v
}

// List returns the master plus every child node (kind "node") for the switcher.
func List() ([]NodeView, error) {
	var rows []model.Node
	if err := global.DB.Where("kind = ?", kindNode).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	names := groupNames()
	out := []NodeView{localView(names)}
	for _, n := range rows {
		out = append(out, view(n, names))
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
	var rows []model.Node
	if err := db.Order("id").Limit(req.PageSize).Offset((req.Page - 1) * req.PageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	names := groupNames()
	if req.Kind == kindNode && req.Page == 1 && req.Info == "" && (req.GroupID == 0 || req.GroupID == defaultGroupID()) {
		res.Items = append(res.Items, localView(names))
		res.Total++
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
	GroupID     uint   `json:"groupID"`
	Description string `json:"description"`
	AgentPort   int    `json:"agentPort"`
	// SSH installs
	SSH *SSHInstall `json:"ssh"`
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
	n := model.Node{
		Name: req.Name, Kind: kindNode, GroupID: req.GroupID, Description: req.Description, Port: req.AgentPort,
		ProxyID: newSecret(32), Status: statusPending,
	}
	if req.SSH != nil {
		if req.SSH.Addr == "" || req.SSH.User == "" {
			return nil, errors.New("ssh address and user are required")
		}
		if req.SSH.Port == 0 {
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
	if req.Addr == "" || req.Port <= 0 {
		return nil, errors.New("panel address and port are required")
	}
	key := ""
	if req.APIKey != "" {
		enc, err := encrypt.StringEncrypt(req.APIKey)
		if err != nil {
			return nil, err
		}
		key = enc
	}
	proto := req.Protocol
	if proto != "https" {
		proto = "http"
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
			update["addr"] = req.Addr
		}
		if req.Port > 0 {
			update["port"] = req.Port
		}
		if req.Protocol != "" {
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

func nodeByToken(token string) (*model.Node, error) {
	if len(token) < 20 {
		return nil, errors.New("invalid token")
	}
	var n model.Node
	if err := global.DB.Where("join_token = ? AND kind = ?", token, kindNode).First(&n).Error; err != nil {
		return nil, errors.New("invalid or used token")
	}
	if time.Since(n.CreatedAt) > 24*time.Hour {
		return nil, errors.New("token expired")
	}
	return &n, nil
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

// JoinBundle consumes the token: the node is addressed by the IP it connected from.
func JoinBundle(token, addr string) (string, error) {
	n, err := nodeByToken(token)
	if err != nil {
		return "", err
	}
	if addr == "" {
		return "", errors.New("cannot determine node address")
	}
	script, err := bundleScript(n, addr)
	if err != nil {
		return "", err
	}
	err = global.DB.Model(&model.Node{}).Where("id = ?", n.ID).
		Updates(map[string]interface{}{"addr": addr, "join_token": "", "status": statusInstalling}).Error
	return script, err
}

// Start boots the background collector once the database is ready.
func Start() { Default.StartBackground() }
