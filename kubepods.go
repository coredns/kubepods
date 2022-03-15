package kubepods

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	core "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/request"
)

// KubePods is a plugin that creates records for a Kubernetes cluster's Pods.
type KubePods struct {
	Next  plugin.Handler
	Zones []string

	Fall fall.F
	ttl  uint32
	mode int

	autoPathSearch []string

	// Kubernetes API interface
	client     kubernetes.Interface
	controller cache.Controller
	indexer    cache.Indexer

	// concurrency control to stop controller
	stopLock sync.Mutex
	shutdown bool
	stopCh   chan struct{}
}

const (
	modeEchoIP = iota
	modeIP
	modeName
	modeNameAndIP
)

// New returns a initialized KubePods.
func New(zones []string) *KubePods {
	k := new(KubePods)
	k.Zones = zones
	k.ttl = defaultTTL
	k.stopCh = make(chan struct{})
	return k
}

const (
	// defaultTTL to apply to all answers.
	defaultTTL = 5
)

// Name implements the Handler interface.
func (k *KubePods) Name() string { return "kubepods" }

// ServeDNS implements the plugin.Handler interface.
func (k *KubePods) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	qname := state.Name()
	zone := plugin.Zones(k.Zones).Matches(qname)
	if zone == "" {
		return plugin.NextOrFailure(k.Name(), k.Next, ctx, w, r)
	}
	zone = state.QName()[len(qname)-len(zone):] // maintain case of original query
	state.Zone = zone

	// query for just the zone results in NODATA
	if len(zone) == len(qname) {
		return k.nodata(state)
	}

	// handle reverse lookup
	if state.QType() == dns.TypePTR {
		addr := dnsutil.ExtractAddressFromReverse(qname)
		if addr == "" {
			return k.nxdomain(ctx, state)
		}
		var records []dns.RR
		if k.mode == modeEchoIP {
			// In EchoIP mode, we cannot synthesize a PTR record because it's impossible to
			// know what namespace to use in the PTR target. So return an NXDOMAIN.
			return k.nxdomain(ctx, state)
		}
		objs, err := k.indexer.ByIndex("reverse", addr)
		if err != nil {
			return dns.RcodeServerFailure, err
		}
		if len(objs) == 0 {
			return k.nxdomain(ctx, state)
		}
		for _, obj := range objs {
			pod, ok := obj.(*core.Pod)
			if !ok {
				return dns.RcodeServerFailure, fmt.Errorf("unexpected %q from *Pod index", reflect.TypeOf(obj))
			}
			records = append(records, k.ptr(state.QName(), addr, pod)...)
		}

		writeResponse(w, r, records, nil, nil, dns.RcodeSuccess)
		return dns.RcodeSuccess, nil
	}

	// handle lookup
	podDomain := state.Name()[0 : len(qname)-len(zone)-1]
	if zone == "." {
		podDomain = state.Name()[0 : len(qname)-len(zone)]
	}
	podSegments := dns.SplitDomainName(podDomain)

	var items []interface{}

	switch len(podSegments) {
	case 2:
		// get the pod by key name from the indexer
		podKey := strings.Join([]string{podSegments[1], "/", podSegments[0]}, "")
		var err error
		if k.mode == modeEchoIP {
			ip := net.ParseIP(undashIP(podSegments[0]))
			if ip == nil {
				return k.nxdomain(ctx, state)
			}
			var records []dns.RR
			if ip.To4() == nil {
				records = []dns.RR{&dns.AAAA{AAAA: ip, Hdr: dns.RR_Header{Name: qname, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: k.ttl}}}
			} else {
				records = []dns.RR{&dns.A{A: ip, Hdr: dns.RR_Header{Name: qname, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: k.ttl}}}
			}
			writeResponse(w, r, records, nil, nil, dns.RcodeSuccess)
			return dns.RcodeSuccess, nil
		}

		if k.mode == modeIP || k.mode == modeNameAndIP {
			items, err = k.indexer.ByIndex("dashedip", podKey)
			if err != nil {
				return dns.RcodeServerFailure, err
			}
		}

		if k.mode == modeName || k.mode == modeNameAndIP {
			item, exists, err := k.indexer.GetByKey(podKey)
			if err != nil {
				return dns.RcodeServerFailure, err
			}
			if exists {
				items = append(items, item)
			}
		}
	case 1:
		// query only contains the namespace
		if k.mode == modeEchoIP {
			// in echo mode, every possible namespace domain exists
			return k.nodata(state)
		}
		items, err := k.indexer.ByIndex("namespace", podSegments[0])
		if err != nil {
			return dns.RcodeServerFailure, err
		}
		// if any pods exist in the namespace, return NODATA
		if len(items) > 0 {
			return k.nodata(state)
		}
	}

	if len(items) == 0 {
		return k.nxdomain(ctx, state)
	}

	var records []dns.RR
	for _, item := range items {
		pod, ok := item.(*core.Pod)
		if !ok {
			return dns.RcodeServerFailure, fmt.Errorf("unexpected %q from *Pod index", reflect.TypeOf(item))
		}

		// add response records
		if state.QType() == dns.TypeA {
			for _, podIP := range pod.Status.PodIPs {
				if strings.Contains(podIP.IP, ":") {
					continue
				}
				if netIP := net.ParseIP(podIP.IP); netIP != nil {
					records = append(records, &dns.A{A: netIP,
						Hdr: dns.RR_Header{Name: qname, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: k.ttl}})
				}
			}
		}
		if state.QType() == dns.TypeAAAA {
			for _, podIP := range pod.Status.PodIPs {
				if !strings.Contains(podIP.IP, ":") {
					continue
				}
				if netIP := net.ParseIP(podIP.IP); netIP != nil {
					records = append(records, &dns.AAAA{AAAA: netIP,
						Hdr: dns.RR_Header{Name: qname, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: k.ttl}})
				}
			}
		}
	}

	writeResponse(w, r, records, nil, nil, dns.RcodeSuccess)
	return dns.RcodeSuccess, nil
}

func (k *KubePods) nxdomain(ctx context.Context, state request.Request) (int, error) {
	if k.Fall.Through(state.Name()) {
		return plugin.NextOrFailure(k.Name(), k.Next, ctx, state.W, state.Req)
	}
	writeResponse(state.W, state.Req, nil, nil, []dns.RR{k.soa()}, dns.RcodeNameError)
	return dns.RcodeNameError, nil
}

func (k *KubePods) nodata(state request.Request) (int, error) {
	writeResponse(state.W, state.Req, nil, nil, []dns.RR{k.soa()}, dns.RcodeSuccess)
	return dns.RcodeSuccess, nil
}

func (k *KubePods) ptr(qname, qip string, pod *core.Pod) (ptrs []dns.RR) {
	if k.mode == modeName || k.mode == modeNameAndIP {
		ptr := &dns.PTR{
			Hdr: dns.RR_Header{Name: qname, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: k.ttl},
			Ptr: dnsutil.Join(pod.Name, pod.Namespace, k.Zones[0]),
		}
		ptrs = append(ptrs, ptr)
	}

	if k.mode == modeIP || k.mode == modeNameAndIP {
		for _, ip := range pod.Status.PodIPs {
			if qip != ip.IP {
				continue
			}
			ptr := &dns.PTR{
				Hdr: dns.RR_Header{Name: qname, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: k.ttl},
				Ptr: dnsutil.Join(dashIP(ip.IP), pod.Namespace, k.Zones[0]),
			}
			ptrs = append(ptrs, ptr)
		}
	}

	return ptrs
}

func writeResponse(w dns.ResponseWriter, r *dns.Msg, answer, extra, ns []dns.RR, rcode int) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Rcode = rcode
	m.Authoritative = true
	m.Answer = answer
	m.Extra = extra
	m.Ns = ns
	w.WriteMsg(m)
}

func (k *KubePods) soa() *dns.SOA {
	return &dns.SOA{
		Hdr:     dns.RR_Header{Name: k.Zones[0], Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: k.ttl},
		Ns:      dnsutil.Join("ns.dns", k.Zones[0]),
		Mbox:    dnsutil.Join("hostmaster.dns", k.Zones[0]),
		Serial:  uint32(time.Now().Unix()),
		Refresh: 7200,
		Retry:   1800,
		Expire:  86400,
		Minttl:  k.ttl,
	}
}

// Ready implements the ready.Readiness interface.
func (k *KubePods) Ready() bool {
	if k.mode == modeEchoIP {
		return true
	}
	return k.controller.HasSynced()
}
