package controller

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// apiServerPolicyName is the NetworkPolicy that lets a policy's APIServerAccess pods reach the API server.
const apiServerPolicyName = "kupra-api-server"

// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch

// apiServerSlices selects the EndpointSlices of the kubernetes Service, which list the API server's addresses.
var apiServerSlices = []client.ListOption{
	client.InNamespace(metav1.NamespaceDefault),
	client.MatchingLabels{discoveryv1.LabelServiceName: "kubernetes"},
}

// apiServerPolicy returns a NetworkPolicy that lets the selected pods reach the API server.
func (r *PreviewReconciler) apiServerPolicy(ctx context.Context, namespace string, selector metav1.LabelSelector) (*networkingv1.NetworkPolicy, error) {
	reader := r.Endpoints
	if reader == nil {
		reader = r.APIReader
	}
	var slices discoveryv1.EndpointSliceList
	if err := reader.List(ctx, &slices, apiServerSlices...); err != nil {
		return nil, err
	}
	rule := apiServerEgress(slices.Items)
	if len(rule.To) == 0 {
		return nil, errors.New("the kubernetes Service has no endpoints")
	}
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: apiServerPolicyName, Namespace: namespace},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: selector,
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress:      []networkingv1.NetworkPolicyEgressRule{rule},
		},
	}, nil
}

// apiServerEgress allows the addresses and ports of the kubernetes Service's endpoints. Network policies apply to traffic to a Service's endpoints, not to the Service's own address.
func apiServerEgress(slices []discoveryv1.EndpointSlice) networkingv1.NetworkPolicyEgressRule {
	var rule networkingv1.NetworkPolicyEgressRule
	cidrs := map[string]bool{}
	ports := map[string]bool{}
	for _, s := range slices {
		for _, e := range s.Endpoints {
			for _, address := range e.Addresses {
				ip, err := netip.ParseAddr(address)
				if err != nil {
					continue
				}
				cidr := netip.PrefixFrom(ip, ip.BitLen()).String()
				if !cidrs[cidr] {
					cidrs[cidr] = true
					rule.To = append(rule.To, networkingv1.NetworkPolicyPeer{IPBlock: &networkingv1.IPBlock{CIDR: cidr}})
				}
			}
		}
		for _, p := range s.Ports {
			if p.Port == nil {
				continue
			}
			key := fmt.Sprintf("%s/%d", ptr.Deref(p.Protocol, ""), *p.Port)
			if !ports[key] {
				ports[key] = true
				rule.Ports = append(rule.Ports, networkingv1.NetworkPolicyPort{Protocol: p.Protocol, Port: ptr.To(intstr.FromInt32(*p.Port))})
			}
		}
	}
	return rule
}

// watchAPIServer caches the kubernetes Service's EndpointSlices for the reconciler, and returns a source that reconciles every preview when they change.
func (r *PreviewReconciler) watchAPIServer(mgr ctrl.Manager) (source.Source, error) {
	endpoints, err := cache.New(mgr.GetConfig(), cache.Options{
		Scheme:            mgr.GetScheme(),
		Mapper:            mgr.GetRESTMapper(),
		DefaultNamespaces: map[string]cache.Config{metav1.NamespaceDefault: {}},
		ByObject: map[client.Object]cache.ByObject{
			&discoveryv1.EndpointSlice{}: {Label: labels.SelectorFromSet(labels.Set{discoveryv1.LabelServiceName: "kubernetes"})},
		},
	})
	if err != nil {
		return nil, err
	}
	if err := mgr.Add(endpoints); err != nil {
		return nil, err
	}
	r.Endpoints = endpoints
	return source.Kind(endpoints, &discoveryv1.EndpointSlice{}, handler.TypedEnqueueRequestsFromMapFunc(
		func(ctx context.Context, _ *discoveryv1.EndpointSlice) []reconcile.Request {
			return r.previews(ctx)
		},
	)), nil
}
