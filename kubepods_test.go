package kubepods

import (
	"context"
	"testing"
	"time"

	"github.com/miekg/dns"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
)

func TestServeDNSModeName(t *testing.T) {
	k := New([]string{"cluster.local.", "in-addr.arpa.", "ip6.arpa."})
	k.mode = modeName

	var externalCases = []test.Case{
		{
			Qname: "pod1.namespace1.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.A("pod1.namespace1.cluster.local.	5	IN	A	1.2.3.4"),
				test.A("pod1.namespace1.cluster.local.	5	IN	A	5.6.7.8"),
			},
		},
		{
			Qname: "pod1.namespace1.cluster.local.", Qtype: dns.TypeAAAA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.AAAA("pod1.namespace1.cluster.local.	5	IN	AAAA	1:2:3::4"),
				test.AAAA("pod1.namespace1.cluster.local.	5	IN	AAAA	5:6:7::8"),
			},
		},
		{
			Qname: "pod2.namespace2.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.A("pod2.namespace2.cluster.local.	5	IN	A	5.6.7.10"),
				test.A("pod2.namespace2.cluster.local.	5	IN	A	5.6.7.9"),
			},
		},
		{
			Qname: "4.3.2.1.in-addr.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.PTR("4.3.2.1.in-addr.arpa.	5	IN	PTR	pod1.namespace1.cluster.local."),
			},
		},
		{
			Qname: "4.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.0.2.0.0.0.1.0.0.0.ip6.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.PTR("4.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.0.2.0.0.0.1.0.0.0.ip6.arpa.	5	IN	PTR	pod1.namespace1.cluster.local."),
			},
		},
		{
			Qname: "cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "namespace1.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "nonexistent-namespace.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "nonexistent-pod.nonexistent-namespace.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "nonexistent-pod.nonexistent-namespace.cluster.local.", Qtype: dns.TypeMX,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
	}

	k.client = fake.NewSimpleClientset()
	ctx := context.Background()
	addFixtures(ctx, k)

	k.setWatch(ctx)
	go k.controller.Run(k.stopCh)
	defer close(k.stopCh)

	// quick and dirty wait for sync
	for !k.controller.HasSynced() {
		time.Sleep(100 * time.Millisecond)
	}

	runTests(t, ctx, k, externalCases)
}

func TestServeDNSModeIP(t *testing.T) {
	k := New([]string{"cluster.local.", "in-addr.arpa.", "ip6.arpa."})
	k.mode = modeIP

	var externalCases = []test.Case{
		{
			Qname: "1-2-3-4.namespace1.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.A("1-2-3-4.namespace1.cluster.local.	5	IN	A	1.2.3.4"),
				test.A("1-2-3-4.namespace1.cluster.local.	5	IN	A	5.6.7.8"),
			},
		},
		{
			Qname: "1-2-3--4.namespace1.cluster.local.", Qtype: dns.TypeAAAA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.AAAA("1-2-3--4.namespace1.cluster.local.	5	IN	AAAA	1:2:3::4"),
				test.AAAA("1-2-3--4.namespace1.cluster.local.	5	IN	AAAA	5:6:7::8"),
			},
		},
		{
			Qname: "5-6-7-10.namespace2.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.A("5-6-7-10.namespace2.cluster.local.	5	IN	A	5.6.7.10"),
				test.A("5-6-7-10.namespace2.cluster.local.	5	IN	A	5.6.7.9"),
			},
		},
		{
			Qname: "4.3.2.1.in-addr.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.PTR("4.3.2.1.in-addr.arpa.	5	IN	PTR	1-2-3-4.namespace1.cluster.local."),
			},
		},
		{
			Qname: "4.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.0.2.0.0.0.1.0.0.0.ip6.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.PTR("4.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.0.2.0.0.0.1.0.0.0.ip6.arpa.	5	IN	PTR	1-2-3--4.namespace1.cluster.local."),
			},
		},
		{
			Qname: "1-2-3-5.namespace1.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "pod1.namespace1.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "namespace1.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "nonexistent-namespace.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "nonexistent-pod.nonexistent-namespace.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "nonexistent-pod.nonexistent-namespace.cluster.local.", Qtype: dns.TypeMX,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
	}

	k.client = fake.NewSimpleClientset()
	ctx := context.Background()
	addFixtures(ctx, k)

	k.setWatch(ctx)
	go k.controller.Run(k.stopCh)
	defer close(k.stopCh)

	// quick and dirty wait for sync
	for !k.controller.HasSynced() {
		time.Sleep(100 * time.Millisecond)
	}

	runTests(t, ctx, k, externalCases)
}

func TestServeDNSModeNameAndIP(t *testing.T) {
	k := New([]string{"cluster.local.", "in-addr.arpa.", "ip6.arpa."})
	k.mode = modeNameAndIP

	var externalCases = []test.Case{
		{
			Qname: "pod1.namespace1.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.A("pod1.namespace1.cluster.local.	5	IN	A	1.2.3.4"),
				test.A("pod1.namespace1.cluster.local.	5	IN	A	5.6.7.8"),
			},
		},
		{
			Qname: "1-2-3-4.namespace1.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.A("1-2-3-4.namespace1.cluster.local.	5	IN	A	1.2.3.4"),
				test.A("1-2-3-4.namespace1.cluster.local.	5	IN	A	5.6.7.8"),
			},
		},
		{
			Qname: "pod1.namespace1.cluster.local.", Qtype: dns.TypeAAAA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.AAAA("pod1.namespace1.cluster.local.	5	IN	AAAA	1:2:3::4"),
				test.AAAA("pod1.namespace1.cluster.local.	5	IN	AAAA	5:6:7::8"),
			},
		},
		{
			Qname: "pod2.namespace2.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.A("pod2.namespace2.cluster.local.	5	IN	A	5.6.7.10"),
				test.A("pod2.namespace2.cluster.local.	5	IN	A	5.6.7.9"),
			},
		},
		{
			Qname: "4.3.2.1.in-addr.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.PTR("4.3.2.1.in-addr.arpa.	5	IN	PTR	1-2-3-4.namespace1.cluster.local."),
				test.PTR("4.3.2.1.in-addr.arpa.	5	IN	PTR	pod1.namespace1.cluster.local."),
			},
		},
		{
			Qname: "4.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.0.2.0.0.0.1.0.0.0.ip6.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.PTR("4.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.0.2.0.0.0.1.0.0.0.ip6.arpa.	5	IN	PTR	1-2-3--4.namespace1.cluster.local."),
				test.PTR("4.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.0.2.0.0.0.1.0.0.0.ip6.arpa.	5	IN	PTR	pod1.namespace1.cluster.local."),
			},
		},
		{
			Qname: "1-2-3-5.namespace1.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "namespace1.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "nonexistent-namespace.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "nonexistent-pod.nonexistent-namespace.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "nonexistent-pod.nonexistent-namespace.cluster.local.", Qtype: dns.TypeMX,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
	}

	k.client = fake.NewSimpleClientset()
	ctx := context.Background()
	addFixtures(ctx, k)

	k.setWatch(ctx)
	go k.controller.Run(k.stopCh)
	defer close(k.stopCh)

	// quick and dirty wait for sync
	for !k.controller.HasSynced() {
		time.Sleep(100 * time.Millisecond)
	}

	runTests(t, ctx, k, externalCases)
}

func TestServeDNSModeEchoIP(t *testing.T) {
	k := New([]string{"cluster.local.", "in-addr.arpa.", "ip6.arpa."})
	k.mode = modeEchoIP

	var externalCases = []test.Case{
		{
			Qname: "1-2-3-4.namespace1.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.A("1-2-3-4.namespace1.cluster.local.	5	IN	A	1.2.3.4"),
			},
		},
		{
			Qname: "1-2-3--4.namespace1.cluster.local.", Qtype: dns.TypeAAAA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.AAAA("1-2-3--4.namespace1.cluster.local.	5	IN	AAAA	1:2:3::4"),
			},
		},
		{
			Qname: "5-6-7-10.namespace2.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.A("5-6-7-10.namespace2.cluster.local.	5	IN	A	5.6.7.10"),
			},
		},
		{
			Qname: "4.3.2.1.in-addr.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "4.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.3.0.0.0.2.0.0.0.1.0.0.0.ip6.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "1-2-3-5.namespace1.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.A("1-2-3-5.namespace1.cluster.local.	5	IN	A	1.2.3.5"),
			}},
		{
			Qname: "pod1.namespace1.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "namespace1.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "nonexistent-namespace.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeSuccess,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "nonexistent-pod.nonexistent-namespace.cluster.local.", Qtype: dns.TypeA,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
		{
			Qname: "nonexistent-pod.nonexistent-namespace.cluster.local.", Qtype: dns.TypeMX,
			Rcode: dns.RcodeNameError,
			Ns:    []dns.RR{k.soa()},
		},
	}

	ctx := context.Background()
	runTests(t, ctx, k, externalCases)
}

func addFixtures(ctx context.Context, k *KubePods) {
	pod1 := &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name:        "pod1",
			Namespace:   "namespace1",
			Annotations: map[string]string{"foo": "bar", "bar": "foo"},
		},
		Status: core.PodStatus{
			PodIPs: []core.PodIP{
				{IP: "1.2.3.4"},
				{IP: "1:2:3::4"},
				{IP: "5.6.7.8"},
				{IP: "5:6:7::8"},
			},
		},
	}
	pod2 := &core.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name:      "pod2",
			Namespace: "namespace2",
		},
		Status: core.PodStatus{
			PodIPs: []core.PodIP{
				{IP: "5.6.7.9"},
				{IP: "5.6.7.10"},
			},
		},
	}
	k.client.CoreV1().Pods(pod1.Namespace).Create(ctx, pod1, meta.CreateOptions{})
	k.client.CoreV1().Pods(pod2.Namespace).Create(ctx, pod2, meta.CreateOptions{})
}

func runTests(t *testing.T, ctx context.Context, k *KubePods, cases []test.Case) {
	for i, tc := range cases {
		r := tc.Msg()
		w := dnstest.NewRecorder(&test.ResponseWriter{})

		_, err := k.ServeDNS(ctx, w, r)
		if err != tc.Error {
			t.Errorf("Test %d: %v", i, err)
			return
		}

		if w.Msg == nil {
			t.Errorf("Test %d: nil message", i)
			continue
		}
		if err := test.SortAndCheck(w.Msg, tc); err != nil {
			t.Errorf("Test %d: %v", i, err)
		}
	}
}
