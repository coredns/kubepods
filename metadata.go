package kubepods

import (
	"context"

	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/request"

	core "k8s.io/api/core/v1"
)

// Metadata implements the metadata.Provider interface.
func (k *KubePods) Metadata(ctx context.Context, state request.Request) context.Context {
	if k.indexer == nil {
		return ctx
	}
	p, err := k.indexer.ByIndex("reverse", state.IP())
	if err != nil || len(p) == 0 {
		return ctx
	}

	pod, ok := p[0].(*core.Pod)
	if !ok {
		return ctx
	}

	metadata.SetValueFunc(ctx, "kubernetes/client-namespace", func() string {
		return pod.Namespace
	})

	metadata.SetValueFunc(ctx, "kubernetes/client-pod-name", func() string {
		return pod.Name
	})

	for key := range pod.Annotations {
		value := pod.Annotations[key]
		metadata.SetValueFunc(ctx, "kubernetes/client-pod-annotation-"+key, func() string {
			return value
		})
	}

	return ctx
}
