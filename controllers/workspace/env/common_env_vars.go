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

package env

import (
	"github.com/devfile/devworkspace-operator/pkg/config"
	corev1 "k8s.io/api/core/v1"
)

func CommonEnvironmentVariables(workspaceName, workspaceId, namespace, creator string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "DEVWORKSPACE_NAMESPACE",
			Value: namespace,
		},
		{
			Name:  "DEVWORKSPACE_NAME",
			Value: workspaceName,
		},
		{
			Name:  "DEVWORKSPACE_ID",
			Value: workspaceId,
		},
		{
			Name:  "DEVWORKSPACE_CREATOR",
			Value: creator,
		},
		{
			Name:  "DEVWORKSPACE_IDLE_TIMEOUT",
			Value: config.ControllerCfg.GetWorkspaceIdleTimeout(),
		},
	}
}
