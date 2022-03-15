package kubepods

import (
	core "k8s.io/api/core/v1"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
)

// AutoPath implements AutoPather
func (k *KubePods) AutoPath(state request.Request) []string {
	zone := plugin.Zones(k.Zones).Matches(state.Name())
	if zone == "" {
		return nil
	}

	if k.mode == modeEchoIP {
		return nil
	}

	ip := state.IP()

	obj, err := k.indexer.ByIndex("reverse", ip)
	if err != nil {
		return nil
	}
	if len(obj) == 0 {
		return nil
	}
	pod, ok := obj[0].(*core.Pod)
	if !ok {
		return nil
	}

	search := make([]string, 3)
	if zone == "." {
		search[0] = pod.Namespace + ".svc."
		search[1] = "svc."
		search[2] = "."
	} else {
		search[0] = pod.Namespace + ".svc." + zone
		search[1] = "svc." + zone
		search[2] = zone
	}

	search = append(search, k.autoPathSearch...)
	search = append(search, "") // search without domain
	return search
}
