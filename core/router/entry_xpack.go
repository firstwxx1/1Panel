//go:build xpack || enterprise

package router

func RouterGroups() []CommonRouter {
	return append(commonGroups(), &NodeRouter{})
}

var RouterGroupApp = RouterGroups()
