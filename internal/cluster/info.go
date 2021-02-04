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

package cluster

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	WatchNamespaceEnvVar = "WATCH_NAMESPACE"
)

// GetOperatorNamespace returns the namespace the operator should be running in.
//
// This function was ported over from Operator SDK 0.17.0 and modified.
func GetOperatorNamespace() (string, error) {
	nsBytes, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("could not read namespace from mounted serviceaccount info")
		}
		return "", err
	}
	ns := strings.TrimSpace(string(nsBytes))
	return ns, nil
}

// GetWatchNamespace returns the namespace the operator should be watching for changes
//
// This function was ported over from Operator SDK 0.17.0
func GetWatchNamespace() (string, error) {
	ns, found := os.LookupEnv(WatchNamespaceEnvVar)
	if !found {
		return "", fmt.Errorf("%s must be set", WatchNamespaceEnvVar)
	}
	return ns, nil
}

func IsOpenShift() (bool, error) {
	kubeCfg, err := config.GetConfig()
	if err != nil {
		return false, err
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(kubeCfg)
	if err != nil {
		return false, err
	}
	apiList, err := discoveryClient.ServerGroups()
	if err != nil {
		return false, err
	}
	if findAPIGroup(apiList.Groups, "route.openshift.io") == nil {
		return false, nil
	} else {
		return true, nil
	}
}

func findAPIGroup(source []metav1.APIGroup, apiName string) *metav1.APIGroup {
	for i := 0; i < len(source); i++ {
		if source[i].Name == apiName {
			return &source[i]
		}
	}
	return nil
}

func findAPIResources(source []*metav1.APIResourceList, groupName string) []metav1.APIResource {
	for i := 0; i < len(source); i++ {
		if source[i].GroupVersion == groupName {
			return source[i].APIResources
		}
	}
	return nil
}

//IsWebhookConfigurationEnabled returns true if both of mutating and validating webhook configurations are enabled
func IsWebhookConfigurationEnabled() (bool, error) {
	kubeCfg, err := config.GetConfig()
	if err != nil {
		return false, err
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(kubeCfg)
	if err != nil {
		return false, err
	}
	_, apiResources, err := discoveryClient.ServerGroupsAndResources()
	if err != nil {
		return false, err
	}

	if admissionRegistrationResources := findAPIResources(apiResources, "admissionregistration.k8s.io/v1beta1"); admissionRegistrationResources != nil {
		isMutatingHookAvailable := false
		isValidatingMutatingHookAvailable := false
		for i := range admissionRegistrationResources {
			if admissionRegistrationResources[i].Name == "mutatingwebhookconfigurations" {
				isMutatingHookAvailable = true
			}

			if admissionRegistrationResources[i].Name == "validatingwebhookconfigurations" {
				isValidatingMutatingHookAvailable = true
			}
		}

		return isMutatingHookAvailable && isValidatingMutatingHookAvailable, nil
	}

	return false, nil
}
