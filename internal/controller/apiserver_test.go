package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/utils/ptr"
)

func TestAPIServerEgress(t *testing.T) {
	tcp := corev1.ProtocolTCP
	slices := []discoveryv1.EndpointSlice{
		{
			AddressType: discoveryv1.AddressTypeIPv4,
			Endpoints:   []discoveryv1.Endpoint{{Addresses: []string{"172.18.0.2"}}, {Addresses: []string{"172.18.0.3"}}},
			Ports:       []discoveryv1.EndpointPort{{Name: ptr.To("https"), Protocol: &tcp, Port: ptr.To[int32](6443)}},
		},
		{
			AddressType: discoveryv1.AddressTypeIPv6,
			Endpoints:   []discoveryv1.Endpoint{{Addresses: []string{"fd00::2"}}},
			Ports:       []discoveryv1.EndpointPort{{Name: ptr.To("https"), Protocol: &tcp, Port: ptr.To[int32](6443)}},
		},
		// The same endpoint, in a slice that hasn't been cleaned up yet.
		{
			AddressType: discoveryv1.AddressTypeIPv4,
			Endpoints:   []discoveryv1.Endpoint{{Addresses: []string{"172.18.0.2"}}},
			Ports:       []discoveryv1.EndpointPort{{Name: ptr.To("https"), Protocol: &tcp, Port: ptr.To[int32](6443)}},
		},
	}
	rule := apiServerEgress(slices)
	var cidrs []string
	for _, peer := range rule.To {
		cidrs = append(cidrs, peer.IPBlock.CIDR)
	}
	if want := []string{"172.18.0.2/32", "172.18.0.3/32", "fd00::2/128"}; len(cidrs) != len(want) || cidrs[0] != want[0] || cidrs[1] != want[1] || cidrs[2] != want[2] {
		t.Errorf("destinations = %v, want %v", cidrs, want)
	}
	if len(rule.Ports) != 1 {
		t.Fatalf("rule has %d ports, want TCP 6443 once", len(rule.Ports))
	}
	if port := rule.Ports[0]; port.Port.IntValue() != 6443 || *port.Protocol != tcp {
		t.Errorf("rule allows %s %d, want TCP 6443", *port.Protocol, port.Port.IntValue())
	}
}
