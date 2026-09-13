package netpolicy

import (
	"fmt"
	"strings"
)

type Policy struct {
	base    string
	allowed map[string]struct{}
	hosts   []string
}

func NewControlled(publicHost string) (Policy, error) {
	base := normalize(publicHost)
	if base == "" || strings.ContainsAny(base, "/: ") {
		return Policy{}, fmt.Errorf("valid controlled public hostname is required")
	}
	if blockedProductionBase(base) {
		return Policy{}, fmt.Errorf("production service hostname %q is not allowed", base)
	}
	hosts := ControlledHosts(base)
	allowed := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		allowed[host] = struct{}{}
	}
	return Policy{base: base, allowed: allowed, hosts: hosts}, nil
}

func ControlledHosts(publicHost string) []string {
	base := normalize(publicHost)
	return []string{
		"loc-a." + base,
		"loc-b." + base,
		"device-loc." + base,
	}
}

func (p Policy) AllowedHost(host string) bool {
	_, ok := p.allowed[normalize(host)]
	return ok
}

func (p Policy) Hosts() []string {
	return append([]string(nil), p.hosts...)
}

func normalize(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func blockedProductionBase(host string) bool {
	for _, suffix := range []string{"apple.com", "icloud.com"} {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}
