package kubepods

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/kubeapi"
)

const pluginName = "kubepods"

var log = clog.NewWithPlugin(pluginName)

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	k, err := parse(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}

	if k.mode != modeEchoIP {
		k.setWatch(context.Background())
		c.OnStartup(startWatch(k, dnsserver.GetConfig(c)))
		c.OnShutdown(stopWatch(k))
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		k.Next = next
		return k
	})

	return nil
}

func parse(c *caddy.Controller) (*KubePods, error) {
	var (
		kns *KubePods
		err error
	)

	i := 0
	for c.Next() {
		if i > 0 {
			return nil, plugin.ErrOnce
		}
		i++

		kns, err = parseStanza(c)
		if err != nil {
			return kns, err
		}
	}
	return kns, nil
}

// parseStanza parses a kubepods stanza
func parseStanza(c *caddy.Controller) (*KubePods, error) {
	kps := New(plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys))
	kps.mode = modeName
	for c.NextBlock() {
		switch c.Val() {
		// TODO: operation modes
		case "fallthrough":
			kps.Fall.SetZonesFromArgs(c.RemainingArgs())
		case "names":
			args := c.RemainingArgs()
			if len(args) != 1 {
				return nil, c.ArgErr()
			}
			switch args[0] {
			case "echo-ip":
				kps.mode = modeEchoIP
			case "ip":
				kps.mode = modeIP
			case "name":
				kps.mode = modeName
			case "name-and-ip":
				kps.mode = modeNameAndIP
			}
		case "ttl":
			args := c.RemainingArgs()
			if len(args) == 0 {
				return nil, c.ArgErr()
			}
			t, err := strconv.Atoi(args[0])
			if err != nil {
				return nil, err
			}
			if t < 0 || t > 3600 {
				return nil, c.Errf("ttl must be in range [0, 3600]: %d", t)
			}
			kps.ttl = uint32(t)
		default:
			return nil, c.Errf("unknown property '%s'", c.Val())
		}
	}

	return kps, nil
}

func (k *KubePods) setWatch(ctx context.Context) {
	// define Pod controller and reverse lookup indexer
	k.indexer, k.controller = cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(o meta.ListOptions) (runtime.Object, error) {
				return k.client.CoreV1().Pods(core.NamespaceAll).List(ctx, o)
			},
			WatchFunc: func(o meta.ListOptions) (watch.Interface, error) {
				return k.client.CoreV1().Pods(core.NamespaceAll).Watch(ctx, o)
			},
		},
		&core.Pod{},
		0,
		cache.ResourceEventHandlerFuncs{},
		cache.Indexers{
			// reverse for reverse lookups
			"reverse": func(obj interface{}) ([]string, error) {
				pod, ok := obj.(*core.Pod)
				if !ok {
					return nil, errors.New("unexpected obj type")
				}
				var idx []string
				for _, addr := range pod.Status.PodIPs {
					idx = append(idx, addr.IP)
				}
				return idx, nil
			},
			// namespace for lookups without pod name
			"namespace": func(obj interface{}) ([]string, error) {
				pod, ok := obj.(*core.Pod)
				if !ok {
					return nil, errors.New("unexpected obj type")
				}
				return []string{pod.Namespace}, nil
			},
			// dashedip for lookups with dashed IP as name
			"dashedip": func(obj interface{}) ([]string, error) {
				pod, ok := obj.(*core.Pod)
				if !ok {
					return nil, errors.New("unexpected obj type")
				}
				var idx []string
				for _, addr := range pod.Status.PodIPs {
					idx = append(idx, strings.Join([]string{pod.Namespace, dashIP(addr.IP)}, "/"))
				}
				return idx, nil
			},
		},
	)
}

func startWatch(k *KubePods, config *dnsserver.Config) func() error {
	return func() error {
		// retrieve client from kubeapi plugin
		var err error
		k.client, err = kubeapi.Client(config)
		if err != nil {
			return err
		}

		// start the informer
		go k.controller.Run(k.stopCh)
		return nil
	}
}

func stopWatch(k *KubePods) func() error {
	return func() error {
		k.stopLock.Lock()
		defer k.stopLock.Unlock()
		if !k.shutdown {
			close(k.stopCh)
			k.shutdown = true
			return nil
		}
		return fmt.Errorf("shutdown already in progress")
	}
}

func undashIP(name string) (ip string) {
	if strings.Count(name, "-") == 3 && !strings.Contains(name, "--") {
		ip = strings.ReplaceAll(name, "-", ".")
	} else {
		ip = strings.ReplaceAll(name, "-", ":")
	}
	return ip
}

func dashIP(ip string) (name string) {
	if strings.Count(ip, ".") == 3 && !strings.Contains(ip, "::") {
		name = strings.ReplaceAll(ip, ".", "-")
	} else {
		name = strings.ReplaceAll(ip, ":", "-")
	}
	return name
}
