//go:build xpack || enterprise

package helper

import (
	"strconv"

	"github.com/1Panel-dev/1Panel/core/app/auth"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/buserr"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/xpack/providers"
)

var _ providers.MCPNodeProvider = (*authHelper)(nil)

func (a *authHelper) ResolveMCPNode(owner auth.APIKeyOwner, id string) (providers.MCPNode, error) {
	if _, err := a.ResolveAPIKeyOwner(nil, owner.Type, owner.ID); err != nil {
		return providers.MCPNode{}, err
	}
	if id == "local" || id == "" {
		return providers.MCPNode{ID: "local", Name: "local", Status: "Healthy", IsLocal: true}, nil
	}
	uid, err := strconv.ParseUint(id, 10, 64)
	if err != nil || uid == 0 || strconv.FormatUint(uid, 10) != id || global.DB == nil {
		return providers.MCPNode{}, buserr.New("ErrNoSuchNode")
	}
	var n model.Node
	if err := global.DB.Where("id = ? AND kind = ?", uint(uid), "node").First(&n).Error; err != nil {
		return providers.MCPNode{}, buserr.New("ErrNoSuchNode")
	}
	if n.Name == "" || n.Name == "local" {
		return providers.MCPNode{}, buserr.New("ErrNoSuchNode")
	}
	status := n.Status
	if status == "" {
		status = "Pending"
	}
	return providers.MCPNode{
		ID:      strconv.FormatUint(uint64(n.ID), 10),
		Name:    n.Name,
		Status:  status,
		IsLocal: false,
	}, nil
}

func (a *authHelper) ListMCPNodes(owner auth.APIKeyOwner) ([]providers.MCPNode, error) {
	localNode, err := a.ResolveMCPNode(owner, "local")
	if err != nil {
		return nil, err
	}
	out := []providers.MCPNode{localNode}
	if global.DB == nil {
		return out, nil
	}
	var rows []model.Node
	if err := global.DB.Where("kind = ?", "node").Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, n := range rows {
		if n.ID == 0 || n.Name == "" || n.Name == "local" {
			continue
		}
		status := n.Status
		if status == "" {
			status = "Pending"
		}
		out = append(out, providers.MCPNode{
			ID:      strconv.FormatUint(uint64(n.ID), 10),
			Name:    n.Name,
			Status:  status,
			IsLocal: false,
		})
	}
	return out, nil
}
