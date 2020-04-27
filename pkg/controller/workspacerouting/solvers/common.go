//
// Copyright (c) 2019-2020 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//

package solvers

import (
	"github.com/che-incubator/che-workspace-operator/pkg/apis/workspace/v1alpha1"
	"github.com/che-incubator/che-workspace-operator/pkg/common"
	"github.com/che-incubator/che-workspace-operator/pkg/config"
	routeV1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type WorkspaceMetadata struct {
	WorkspaceId   string
	Namespace     string
	PodSelector   map[string]string
	RoutingSuffix string
}

func getDiscoverableServicesForEndpoints(endpoints map[string][]v1alpha1.Endpoint, meta WorkspaceMetadata) []corev1.Service {
	var services []corev1.Service
	for _, machineEndpoints := range endpoints {
		for _, endpoint := range machineEndpoints {
			if endpoint.Attributes[v1alpha1.DISCOVERABLE_ATTRIBUTE] == "true" {
				// Create service with name matching endpoint
				// TODO: This could cause a reconcile conflict if multiple workspaces define the same discoverable endpoint
				// Also endpoint names may not be valid as service names
				servicePort := corev1.ServicePort{
					Name:       common.EndpointName(endpoint.Name),
					Protocol:   corev1.ProtocolTCP,
					Port:       int32(endpoint.Port),
					TargetPort: intstr.FromInt(int(endpoint.Port)),
				}
				services = append(services, corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      common.EndpointName(endpoint.Name),
						Namespace: meta.Namespace,
						Labels: map[string]string{
							config.WorkspaceIDLabel: meta.WorkspaceId,
						},
					},
					Spec: corev1.ServiceSpec{
						Ports:    []corev1.ServicePort{servicePort},
						Selector: meta.PodSelector,
						Type:     corev1.ServiceTypeClusterIP,
					},
				})
			}
		}
	}
	return services
}

func getServicesForEndpoints(endpoints map[string][]v1alpha1.Endpoint, meta WorkspaceMetadata) []corev1.Service {
	var services []corev1.Service
	var servicePorts []corev1.ServicePort
	for _, machineEndpoints := range endpoints {
		for _, endpoint := range machineEndpoints {
			servicePort := corev1.ServicePort{
				Name:       common.EndpointName(endpoint.Name),
				Protocol:   corev1.ProtocolTCP,
				Port:       int32(endpoint.Port),
				TargetPort: intstr.FromInt(int(endpoint.Port)),
			}
			servicePorts = append(servicePorts, servicePort)
		}
	}

	services = append(services, corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.ServiceName(meta.WorkspaceId),
			Namespace: meta.Namespace,
			Labels: map[string]string{
				config.WorkspaceIDLabel: meta.WorkspaceId,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports:    servicePorts,
			Selector: meta.PodSelector,
			Type:     corev1.ServiceTypeClusterIP,
		},
	})

	return services
}

func getRoutingForSpec(endpoints map[string][]v1alpha1.Endpoint, meta WorkspaceMetadata) ([]v1beta1.Ingress, []routeV1.Route) {
	var ingresses []v1beta1.Ingress
	var routes []routeV1.Route
	for _, machineEndpoints := range endpoints {
		for _, endpoint := range machineEndpoints {
			if endpoint.Attributes[v1alpha1.PUBLIC_ENDPOINT_ATTRIBUTE] != "true" {
				continue
			}
			if config.ControllerCfg.IsOpenShift() {
				routes = append(routes, getRouteForEndpoint(endpoint, meta))
			} else {
				ingresses = append(ingresses, getIngressForEndpoint(endpoint, meta))
			}
		}
	}
	return ingresses, routes
}

func getRouteForEndpoint(endpoint v1alpha1.Endpoint, meta WorkspaceMetadata) routeV1.Route {
	targetEndpoint := intstr.FromInt(int(endpoint.Port))
	endpointName := common.EndpointName(endpoint.Name)
	return routeV1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.RouteName(meta.WorkspaceId, endpointName),
			Namespace: meta.Namespace,
			Labels: map[string]string{
				config.WorkspaceIDLabel: meta.WorkspaceId,
			},
			Annotations: map[string]string{
				config.WorkspaceEndpointNameAnnotation: endpoint.Name,
			},
		},
		Spec: routeV1.RouteSpec{
			Host: common.EndpointHostname(meta.WorkspaceId, endpointName, endpoint.Port, meta.RoutingSuffix),
			To: routeV1.RouteTargetReference{
				Kind: "Service",
				Name: common.ServiceName(meta.WorkspaceId),
			},
			Port: &routeV1.RoutePort{
				TargetPort: targetEndpoint,
			},
			TLS: &routeV1.TLSConfig{
				Termination:                   routeV1.TLSTerminationEdge,
				InsecureEdgeTerminationPolicy: routeV1.InsecureEdgeTerminationPolicyRedirect,
			},
		},
	}
}

func getIngressForEndpoint(endpoint v1alpha1.Endpoint, meta WorkspaceMetadata) v1beta1.Ingress {
	targetEndpoint := intstr.FromInt(int(endpoint.Port))
	endpointName := common.EndpointName(endpoint.Name)
	hostname := common.EndpointHostname(meta.WorkspaceId, endpointName, endpoint.Port, meta.RoutingSuffix)
	annotations := map[string]string{
		config.WorkspaceEndpointNameAnnotation: endpoint.Name,
	}
	for k, v := range ingressAnnotations {
		annotations[k] = v
	}

	return v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.RouteName(meta.WorkspaceId, endpointName),
			Namespace: meta.Namespace,
			Labels: map[string]string{
				config.WorkspaceIDLabel: meta.WorkspaceId,
			},
			Annotations: annotations,
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{
				{
					Host: hostname,
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Backend: v1beta1.IngressBackend{
										ServiceName: common.ServiceName(meta.WorkspaceId),
										ServicePort: targetEndpoint,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
