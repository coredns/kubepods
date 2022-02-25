package kubepods

import (
	"context"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"

	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestMetadata(t *testing.T) {
	k := New([]string{"cluster.local.", "in-addr.arpa.", "ip6.arpa."})
	k.mode = modeName
	k.client = fake.NewSimpleClientset()
	ctx := metadata.ContextWithMetadata(context.Background())
	addFixtures(ctx, k)
	k.setWatch(ctx)
	go k.controller.Run(k.stopCh)
	defer close(k.stopCh)
	// quick and dirty wait for sync
	for !k.controller.HasSynced() {
		time.Sleep(100 * time.Millisecond)
	}

	state := request.Request{
		Req:  &dns.Msg{Question: []dns.Question{{Name: "example.com.", Qtype: dns.TypeA}}},
		Zone: ".",
		W:    &test.ResponseWriter{RemoteIP: "1.2.3.4"},
	}

	k.Metadata(ctx, state)

	expect := map[string]string{
		"kubepods/client-namespace":          "namespace1",
		"kubepods/client-pod-name":           "pod1",
		"kubepods/client-pod-annotation-foo": "bar",
		"kubepods/client-pod-annotation-bar": "foo",
	}

	md := make(map[string]string)
	for _, l := range metadata.Labels(ctx) {
		md[l] = metadata.ValueFunc(ctx, l)()
	}
	if mapsDiffer(expect, md) {
		t.Errorf("Expected metadata %v and got %v", expect, md)
	}
}

func mapsDiffer(a, b map[string]string) bool {
	if len(a) != len(b) {
		return true
	}

	for k, va := range a {
		vb, ok := b[k]
		if !ok || va != vb {
			return true
		}
	}
	return false
}
