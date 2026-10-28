//go:build xpack || enterprise

package helper

import "github.com/1Panel-dev/1Panel/core/xpack/node"

func NewIMultiNodeProvider() *node.Provider {
	return node.Default
}
