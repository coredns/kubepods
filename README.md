# kubepods

## Name

*kubepods* - create records for Kubernetes Pods.

## Description

*kubepods* synthesizes A, AAAA, and PTR records for Pod addresses.

By default, this plugin requires ...
* The [_kubeapi_ plugin](http://github.com/coredns/kubeapi) to make a connection
to the Kubernetes API.
* CoreDNS's Service Account has list/watch permission to the Pods API.

This plugin can only be used once per Server Block.

## Syntax

```
kubepods [ZONES...] {
    names MODE
    ttl TTL
    fallthrough [ZONES...]
}
```

* `names` **MODE** sets the record naming scheme to **MODE**.  The following modes are available:
  * `name` - Default. Use the Pod's name and namespace. e.g. `pod1.default.pod.cluster.local.`
  * `ip` - Use the Pod's IP addresses and namespace. e.g. `1-2-3-4.default.pod.cluster.local.`
  * `name-and-ip` - Use both modes `name` and `ip`, as above.
  * `echo-ip` - Like `ip`, but do not validate the Pod's existence and just echo the IP in the query to the response.
    Included only for backward compatibility, this implements the _deprecated and insecure_ Pod records specification
    from Kubernetes DNS-Based Service Discovery.  In this mode PTR records cannot be synthesized. This mode is considered
    insecure because it does not validate the existence of a Pod matching the IP. No connection to the API is required
    in this mode.
* `ttl` allows you to set a custom TTL for responses. The default is 5 seconds.  The minimum TTL allowed is
  0 seconds, and the maximum is capped at 3600 seconds. Setting TTL to 0 will prevent records from being cached.
  All endpoint queries and headless service queries will result in an NXDOMAIN.
* `fallthrough` **[ZONES...]** If a query for a record in the zones for which the plugin is authoritative
  results in NXDOMAIN, normally that is what the response will be. However, if you specify this option,
  the query will instead be passed on down the plugin chain, which can include another plugin to handle
  the query. If **[ZONES...]** is omitted, then fallthrough happens for all zones for which the plugin
  is authoritative. If specific zones are listed (for example `in-addr.arpa` and `ip6.arpa`), then only
  queries for those zones will be subject to fallthrough.

## External Plugin

To use this plugin, compile CoreDNS with this plugin added to the `plugin.cfg`.  It should be positioned before
the _kubernetes_ plugin if _kubepods_ is using the same zone or a superzone of _kubernetes_.  This plugin also requires
the _kubeapi_ plugin, which should be added to the end of `plugin.cfg`.

## Metadata

The kubepods plugin will publish the following metadata if the *metadata* plugin is also enabled:

* `kubepods/client-namespace`: the client pod's namespace
* `kubepods/client-pod-name`: the client pod's name
* `kubepods/client-pod-annotation-X`: the client pod's annotations, where `X` is the annotation name

This metadata is not available in `echo-ip` mode.

## Ready

This plugin reports that it is ready to the _ready_ plugin once it has received the complete list of Pods
from the Kubernetes API.

## Examples

Use Pods' names and IP addresses to answer forward and reverse lookups in the zone `pod.cluster.local.`.
Fallthrough to the next plugin for reverse lookups that don't match any Pod addresses.

```
kubeapi
kubepods pod.cluster.local in-addr.arpa ip6.arpa {
  fallthrough in-addr.arpa ip6.arpa
}
```
