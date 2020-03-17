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
	"context"
	"github.com/che-incubator/che-workspace-operator/pkg/controller/ownerref"
	"k8s.io/api/admissionregistration/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var log = logf.Log.WithName("webhook.creator")

//SetUp set up mutate webhook that manager creator annotations for workspaces
func SetUp(webhookServer *webhook.Server, ctx context.Context) error {
	log.Info("Configuring creator mutating webhook")
	c, err := createClient()
	if err != nil {
		return err
	}

	mutateWebhookCfg := buildMutateWebhookCfg()

	ownRef, err := ownerref.FindControllerOwner(ctx, c)
	if err != nil {
		return err
	}
	//TODO For some reasons it's still possible to update reference by user
	//TODO Investigate if we can block it. The same issue is valid for Deployment owner
	mutateWebhookCfg.SetOwnerReferences([]metav1.OwnerReference{*ownRef})

	if err := c.Create(ctx, mutateWebhookCfg); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		// Webhook Configuration already exists, we want to update it
		// as we do not know if any fields might have changed.
		existingCfg := &v1beta1.MutatingWebhookConfiguration{}
		err := c.Get(ctx, types.NamespacedName{
			Name:      mutateWebhookCfg.Name,
			Namespace: mutateWebhookCfg.Namespace,
		}, existingCfg)

		mutateWebhookCfg.ResourceVersion = existingCfg.ResourceVersion
		err = c.Update(ctx, mutateWebhookCfg)
		if err != nil {
			return err
		}
		log.Info("Updated creator mutating webhook configuration")
	} else {
		log.Info("Created creator mutating webhook configuration")
	}

	webhookServer.Register(mutateWebhookPath, &webhook.Admission{Handler: &WorkspaceAnnotator{}})

	validateWebhookCfg := buildValidateWebhookCfg()
	validateWebhookCfg.SetOwnerReferences([]metav1.OwnerReference{*ownRef})
	res v1beta1.MutatingWebhookConfiguration[]
    c.List(ctx,res , )

	if err := c.Create(ctx, validateWebhookCfg); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		// Webhook Configuration already exists, we want to update it
		// as we do not know if any fields might have changed.
		existingCfg := &v1beta1.ValidatingWebhookConfiguration{}
		err := c.Get(ctx, types.NamespacedName{
			Name:      validateWebhookCfg.Name,
			Namespace: validateWebhookCfg.Namespace,
		}, existingCfg)

		validateWebhookCfg.ResourceVersion = existingCfg.ResourceVersion
		err = c.Update(ctx, validateWebhookCfg)
		if err != nil {
			return err
		}
		log.Info("Updated creator validating webhook configuration")
	} else {
		log.Info("Created creator validating webhook configuration")
	}

	webhookServer.Register(validateWebhookPath, &webhook.Admission{Handler: &WorkspaceValidator{}})

	return nil
}

//TODO It's copied/pasted from embedded_registry. Consider move it to util class
func createClient() (client.Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	c, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, err
	}

	return c, nil
}
