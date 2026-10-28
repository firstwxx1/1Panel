//go:build xpack || enterprise

package router

func RouterGroups() []CommonRouter {
	return append(commonGroups(), &WafRouter{}, &NodeRouter{}, &MonitorRouter{}, &TamperRouter{})
}

var RouterGroupApp = RouterGroups()
