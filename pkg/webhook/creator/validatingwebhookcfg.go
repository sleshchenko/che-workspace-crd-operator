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
package creator

import (
	"github.com/che-incubator/che-workspace-operator/pkg/webhook/server"
	"k8s.io/api/admissionregistration/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	validateWebhookCfgName       = "workspace.che.eclipse.org"
	validateWebhookPath          = "/validate-workspaces"
	validateWebhookFailurePolicy = v1beta1.Fail
)

func buildValidateWebhookCfg() *v1beta1.ValidatingWebhookConfiguration {
	validateWebhookFailurePolicy := validateWebhookFailurePolicy
	validateWebhookPath := validateWebhookPath
	return &v1beta1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: validateWebhookCfgName,
		},
		Webhooks: []v1beta1.ValidatingWebhook{
			{
				Name:          "validate-workspaces.che-workspace-controller.svc",
				FailurePolicy: &validateWebhookFailurePolicy,
				ClientConfig: v1beta1.WebhookClientConfig{
					Service: &v1beta1.ServiceReference{
						Name:      "workspace-controller",
						Namespace: "che-workspace-controller",
						Path:      &validateWebhookPath,
					},
					CABundle: server.CABundle,
				},
				Rules: []v1beta1.RuleWithOperations{
					{
						Operations: []v1beta1.OperationType{v1beta1.Connect},
						Rule: v1beta1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"pods/exec"},
						},
					},
				},
			},
		},
	}
}

