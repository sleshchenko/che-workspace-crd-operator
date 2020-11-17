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

package config

import (
	"fmt"
	"os"
)

type ControllerEnv struct{}

const (
	webHooksSecretNameEnvVar      = "WEBHOOK_SECRET_NAME"
	webHooksCertificateNameEnvVar = "WEBHOOK_CERTIFICATE_NAME"
)

func GetWebhooksSecretName() (string, error) {
	env := os.Getenv(webHooksSecretNameEnvVar)
	if env == "" {
		return "", fmt.Errorf("environment variable %s is unset", webHooksSecretNameEnvVar)
	}
	return env, nil
}

func GetWebhooksCertName() (string, error) {
	env := os.Getenv(webHooksCertificateNameEnvVar)
	if env == "" {
		return "", fmt.Errorf("environment variable %s is unset", webHooksCertificateNameEnvVar)
	}
	return env, nil
}