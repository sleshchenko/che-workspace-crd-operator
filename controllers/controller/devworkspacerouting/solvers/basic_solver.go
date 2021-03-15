//
// Copyright (c) 2019-2021 Red Hat, Inc.
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
	controllerv1alpha1 "github.com/devfile/devworkspace-operator/apis/controller/v1alpha1"
	"github.com/devfile/devworkspace-operator/pkg/constants"
	"github.com/devfile/devworkspace-operator/pkg/infrastructure"
)

var routeAnnotations = func(endpointName string) map[string]string {
	return map[string]string{
		"haproxy.router.openshift.io/rewrite-target": "/",
		constants.WorkspaceEndpointNameAnnotation:    endpointName,
	}
}

var nginxIngressAnnotations = func(endpointName string) map[string]string {
	return map[string]string{
		"kubernetes.io/ingress.class":                "nginx",
		"nginx.ingress.kubernetes.io/rewrite-target": "/",
		"nginx.ingress.kubernetes.io/ssl-redirect":   "false",
		constants.WorkspaceEndpointNameAnnotation:    endpointName,
	}
}

// Basic solver exposes endpoints without any authentication
// According to the current cluster there is different behavior:
// Kubernetes: use Ingresses without TLS
// OpenShift: use Routes with TLS enabled
type BasicSolver struct{}

var _ RoutingSolver = (*BasicSolver)(nil)

func (s *BasicSolver) FinalizerRequired(*controllerv1alpha1.DevWorkspaceRouting) bool {
	return false
}

func (s *BasicSolver) Finalize(*controllerv1alpha1.DevWorkspaceRouting) error {
	return nil
}

func (s *BasicSolver) GetSpecObjects(routing *controllerv1alpha1.DevWorkspaceRouting, workspaceMeta WorkspaceMetadata) (RoutingObjects, error) {
	routingObjects := RoutingObjects{}

	spec := routing.Spec
	services := getServicesForEndpoints(spec.Endpoints, workspaceMeta)
	services = append(services, GetDiscoverableServicesForEndpoints(spec.Endpoints, workspaceMeta)...)
	routingObjects.Services = services
	if infrastructure.IsOpenShift() {
		routingObjects.Routes = getRoutesForSpec(spec.Endpoints, workspaceMeta)
	} else {
		routingObjects.Ingresses = getIngressesForSpec(spec.Endpoints, workspaceMeta)
	}

	return routingObjects, nil
}

func (s *BasicSolver) GetExposedEndpoints(
	endpoints map[string]controllerv1alpha1.EndpointList,
	routingObj RoutingObjects) (exposedEndpoints map[string]controllerv1alpha1.ExposedEndpointList, ready bool, err error) {
	return getExposedEndpoints(endpoints, routingObj)
}
