//go:build xpack || enterprise

package server

import "github.com/1Panel-dev/1Panel/core/xpack/node"

func InitOthersBeforeHook() {
}

func InitOthersAfterHook() {
	node.Start()
}
