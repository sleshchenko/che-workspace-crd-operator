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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/go-cmp/cmp"
	"os"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"strings"
	"time"

	controllerv1alpha1 "github.com/devfile/devworkspace-operator/apis/controller/v1alpha1"
	"github.com/devfile/devworkspace-operator/internal/cluster"
	"github.com/devfile/devworkspace-operator/pkg/common"
	"github.com/devfile/devworkspace-operator/pkg/config"
	"github.com/devfile/devworkspace-operator/pkg/timing"
	appsv1 "k8s.io/api/apps/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	devworkspace "github.com/devfile/api/pkg/apis/workspaces/v1alpha2"
	"github.com/devfile/devworkspace-operator/controllers/workspace/provision"
	"github.com/devfile/devworkspace-operator/controllers/workspace/restapis"
)

type currentStatus struct {
	// Map of condition types that are true for the current workspace. Key is valid condition, value is optional
	// message to be filled into condition's 'Message' field.
	Conditions map[devworkspace.WorkspaceConditionType]string
	// Current workspace phase
	Phase devworkspace.WorkspacePhase
}

// DevWorkspaceReconciler reconciles a DevWorkspace object
type DevWorkspaceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

/////// CRD-related RBAC roles
// +kubebuilder:rbac:groups=workspace.devfile.io,resources=*,verbs=*
// +kubebuilder:rbac:groups=controller.devfile.io,resources=*,verbs=*
/////// Required permissions for controller
// +kubebuilder:rbac:groups=apps;extensions,resources=deployments;replicasets,verbs=*
// +kubebuilder:rbac:groups="",resources=pods;serviceaccounts;secrets;configmaps;persistentvolumeclaims,verbs=*
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;create;watch;update;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterroles;clusterrolebindings,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups=oauth.openshift.io,resources=oauthclients,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;create
// +kubebuilder:rbac:groups=apps,resourceNames=devworkspace-controller,resources=deployments/finalizers,verbs=update
/////// Required permissions for workspace ServiceAccount
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups=apps;extensions,resources=replicasets,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps;extensions,resources=deployments,verbs=get;list;watch

func (r *DevWorkspaceReconciler) Reconcile(req ctrl.Request) (reconcileResult ctrl.Result, err error) {
	ctx := context.Background()
	reqLogger := r.Log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling Workspace")
	clusterAPI := provision.ClusterAPI{
		Client: r.Client,
		Scheme: r.Scheme,
		Logger: reqLogger,
	}

	// Fetch the Workspace instance
	workspace := &devworkspace.DevWorkspace{}
	err = r.Get(ctx, req.NamespacedName, workspace)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if workspace.DeletionTimestamp != nil {
		reqLogger.V(5).Info("Skipping reconcile of deleted resource")
		return reconcile.Result{}, nil
	}

	// Ensure workspaceID is set.
	if workspace.Status.WorkspaceId == "" {
		workspaceId, err := getWorkspaceId(workspace)
		if err != nil {
			return reconcile.Result{}, err
		}
		workspace.Status.WorkspaceId = workspaceId
		err = r.Status().Update(ctx, workspace)
		return reconcile.Result{Requeue: true}, err
	}

	if !workspace.Spec.Started {
		timing.ClearAnnotations(workspace)
		r.syncTimingToCluster(ctx, workspace, reqLogger)
		return r.stopWorkspace(workspace, reqLogger)
	}

	if workspace.Status.Phase == devworkspace.WorkspaceStatusFailed {
		// TODO: Figure out when workspace spec is changed and clear failed status to allow reconcile to continue
		reqLogger.Info("Workspace startup is failed; not attempting to update.")
		return reconcile.Result{}, nil
	}

	// Prepare handling workspace status and condition
	reconcileStatus := currentStatus{
		Conditions: map[devworkspace.WorkspaceConditionType]string{},
		Phase:      devworkspace.WorkspaceStatusStarting,
	}
	timing.SetTime(workspace, timing.WorkspaceStarted)
	defer func() (reconcile.Result, error) {
		r.syncTimingToCluster(ctx, workspace, reqLogger)
		return r.updateWorkspaceStatus(workspace, reqLogger, &reconcileStatus, reconcileResult, err)
	}()

	msg, err := r.validateCreatorTimestamp(workspace)
	if err != nil {
		reconcileStatus.Phase = devworkspace.WorkspaceStatusFailed
		reconcileStatus.Conditions[devworkspace.WorkspaceFailedStart] = msg
		return reconcile.Result{}, err
	}

	_, ok := workspace.Annotations[config.WorkspaceStopReasonAnnotation]
	if ok {
		delete(workspace.Annotations, config.WorkspaceStopReasonAnnotation)
		err = r.Update(context.TODO(), workspace)
		return reconcile.Result{Requeue: true}, err
	}

	restrictedAccess := workspace.Annotations[config.WorkspaceRestrictedAccessAnnotation]
	if restrictedAccess == "true" && config.ControllerCfg.GetWebhooksEnabled() != "true" {
		reqLogger.Info("Workspace is configured to have restricted access but webhooks are not enabled.")
		reconcileStatus.Phase = devworkspace.WorkspaceStatusFailed
		reconcileStatus.Conditions[devworkspace.WorkspaceFailedStart] = "Workspace has restricted-access annotation " +
			"applied but operator does not have webhooks enabled. " +
			"Remove restricted-access annotation or ask an administrator " +
			"to reconfigure Operator."
		return reconcile.Result{}, nil
	}

	// Step one: Create components, and wait for their states to be ready.
	timing.SetTime(workspace, timing.ComponentsCreated)
	componentsStatus := provision.SyncComponentsToCluster(workspace, clusterAPI)
	if !componentsStatus.Continue {
		if componentsStatus.FailStartup {
			reqLogger.Info("DevWorkspace start failed")
			reconcileStatus.Phase = devworkspace.WorkspaceStatusFailed
			if componentsStatus.Message != "" {
				reconcileStatus.Conditions[devworkspace.WorkspaceFailedStart] = componentsStatus.Message
			} else {
				reconcileStatus.Conditions[devworkspace.WorkspaceFailedStart] = "Could not find plugins for devworkspace"
			}
		} else {
			reqLogger.Info("Waiting on components to be ready")
		}
		return reconcile.Result{Requeue: componentsStatus.Requeue}, componentsStatus.Err
	}
	componentDescriptions := componentsStatus.ComponentDescriptions
	reconcileStatus.Conditions[devworkspace.WorkspaceReady] = ""
	timing.SetTime(workspace, timing.ComponentsReady)

	// Only add che rest apis if Theia editor is present in the devfile
	if restapis.IsCheRestApisRequired(workspace.Spec.Template.Components) {
		if !restapis.IsCheRestApisConfigured() {
			reconcileStatus.Phase = devworkspace.WorkspaceStatusFailed
			reconcileStatus.Conditions[devworkspace.WorkspaceFailedStart] = "Che REST API sidecar is not configured but required for the Theia plugin"
			return reconcile.Result{Requeue: false}, errors.New("che REST API sidecar is not configured but required for used Theia plugin")
		}
		// TODO: first half of provisioning rest-apis
		cheRestApisComponent := restapis.GetCheRestApisComponent(workspace.Name, workspace.Status.WorkspaceId, workspace.Namespace)
		// some of containers, like theia needs Che API Sidecar be availble just after start up. So, putting Che API Sidecar first before all
		componentDescriptions = append([]controllerv1alpha1.ComponentDescription{cheRestApisComponent}, componentDescriptions...)
	}

	pvcStatus := provision.SyncPVC(workspace, componentDescriptions, r.Client, reqLogger)
	if pvcStatus.Err != nil || !pvcStatus.Continue {
		return reconcile.Result{Requeue: true}, pvcStatus.Err
	}

	rbacStatus := provision.SyncRBAC(workspace, r.Client, reqLogger)
	if rbacStatus.Err != nil || !rbacStatus.Continue {
		return reconcile.Result{Requeue: true}, rbacStatus.Err
	}

	// Step two: Create routing, and wait for routing to be ready
	timing.SetTime(workspace, timing.RoutingCreated)
	routingStatus := provision.SyncRoutingToCluster(workspace, componentDescriptions, clusterAPI)
	if !routingStatus.Continue {
		if routingStatus.FailStartup {
			reqLogger.Info("DevWorkspace start failed")
			reconcileStatus.Phase = devworkspace.WorkspaceStatusFailed
			// TODO: Propagate failure reason from workspaceRouting
			reconcileStatus.Conditions[devworkspace.WorkspaceFailedStart] = "Failed to install network objects required for devworkspace"
			return reconcile.Result{}, routingStatus.Err
		}
		reqLogger.Info("Waiting on routing to be ready")
		return reconcile.Result{Requeue: routingStatus.Requeue}, routingStatus.Err
	}
	reconcileStatus.Conditions[devworkspace.WorkspaceRoutingReady] = ""
	timing.SetTime(workspace, timing.RoutingReady)

	statusOk, err := syncWorkspaceIdeURL(workspace, routingStatus.ExposedEndpoints, clusterAPI)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !statusOk {
		reqLogger.Info("Updating workspace status")
		return reconcile.Result{Requeue: true}, nil
	}

	// Step three: setup che-rest-apis configmap
	if restapis.IsCheRestApisRequired(workspace.Spec.Template.Components) {
		configMapStatus := restapis.SyncRestAPIsConfigMap(workspace, componentDescriptions, routingStatus.ExposedEndpoints, clusterAPI)
		if !configMapStatus.Continue {
			// FailStartup is not possible for generating the configmap
			reqLogger.Info("Waiting on che-rest-apis configmap to be ready")
			return reconcile.Result{Requeue: configMapStatus.Requeue}, configMapStatus.Err
		}
	}

	// Step four: Collect all workspace deployment contributions
	routingPodAdditions := routingStatus.PodAdditions
	var podAdditions []controllerv1alpha1.PodAdditions
	for _, component := range componentDescriptions {
		podAdditions = append(podAdditions, component.PodAdditions)
	}
	if routingPodAdditions != nil {
		podAdditions = append(podAdditions, *routingPodAdditions)
	}

	// Step five: Prepare workspace ServiceAccount
	saAnnotations := map[string]string{}
	if routingPodAdditions != nil {
		saAnnotations = routingPodAdditions.ServiceAccountAnnotations
	}
	serviceAcctStatus := provision.SyncServiceAccount(workspace, saAnnotations, clusterAPI)
	if !serviceAcctStatus.Continue {
		// FailStartup is not possible for generating the serviceaccount
		reqLogger.Info("Waiting for workspace ServiceAccount")
		return reconcile.Result{Requeue: serviceAcctStatus.Requeue}, serviceAcctStatus.Err
	}
	serviceAcctName := serviceAcctStatus.ServiceAccountName
	reconcileStatus.Conditions[devworkspace.WorkspaceServiceAccountReady] = ""

	// Step five: Create deployment and wait for it to be ready
	timing.SetTime(workspace, timing.DeploymentCreated)
	deploymentStatus := provision.SyncDeploymentToCluster(workspace, podAdditions, componentDescriptions, serviceAcctName, clusterAPI)
	if !deploymentStatus.Continue {
		if deploymentStatus.FailStartup {
			reqLogger.Info("Workspace start failed")
			reconcileStatus.Phase = devworkspace.WorkspaceStatusFailed
			reconcileStatus.Conditions[devworkspace.WorkspaceFailedStart] = fmt.Sprintf("Devworkspace spec is invalid: %s", deploymentStatus.Err)
			return reconcile.Result{}, deploymentStatus.Err
		}
		reqLogger.Info("Waiting on deployment to be ready")
		return reconcile.Result{Requeue: deploymentStatus.Requeue}, deploymentStatus.Err
	}
	reconcileStatus.Conditions[devworkspace.WorkspaceReady] = ""
	timing.SetTime(workspace, timing.DeploymentReady)

	serverReady, err := checkServerStatus(workspace)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !serverReady {
		return reconcile.Result{RequeueAfter: 1 * time.Second}, nil
	}
	timing.SetTime(workspace, timing.WorkspaceReady)
	timing.SummarizeStartup(workspace)
	reconcileStatus.Phase = devworkspace.WorkspaceStatusRunning
	return reconcile.Result{}, nil
}

func (r *DevWorkspaceReconciler) stopWorkspace(workspace *devworkspace.DevWorkspace, logger logr.Logger) (reconcile.Result, error) {
	workspaceDeployment := &appsv1.Deployment{}
	namespaceName := types.NamespacedName{
		Name:      common.DeploymentName(workspace.Status.WorkspaceId),
		Namespace: workspace.Namespace,
	}
	status := &currentStatus{}
	err := r.Get(context.TODO(), namespaceName, workspaceDeployment)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			status.Phase = devworkspace.WorkspaceStatusStopped
			return r.updateWorkspaceStatus(workspace, logger, status, reconcile.Result{}, nil)
		}
		return reconcile.Result{}, err
	}

	status.Phase = devworkspace.WorkspaceStatusStopping
	replicas := workspaceDeployment.Spec.Replicas
	if replicas == nil || *replicas > 0 {
		logger.Info("Stopping workspace")
		patch := client.MergeFrom(workspaceDeployment.DeepCopy())
		var replicasZero int32 = 0
		workspaceDeployment.Spec.Replicas = &replicasZero
		err = r.Patch(context.TODO(), workspaceDeployment, patch)
		if err != nil && !k8sErrors.IsConflict(err) {
			return reconcile.Result{}, err
		}
		return r.updateWorkspaceStatus(workspace, logger, status, reconcile.Result{}, nil)
	}

	if workspaceDeployment.Status.Replicas == 0 {
		logger.Info("Workspace stopped")
		status.Phase = devworkspace.WorkspaceStatusStopped
	}
	return r.updateWorkspaceStatus(workspace, logger, status, reconcile.Result{}, nil)
}

func (r *DevWorkspaceReconciler) syncTimingToCluster(
	ctx context.Context, workspace *devworkspace.DevWorkspace, reqLogger logr.Logger) {
	if timing.IsEnabled() {
		if err := r.Update(ctx, workspace); err != nil {
			if k8sErrors.IsConflict(err) {
				reqLogger.Info("Got conflict when trying to apply timing annotations to workspace")
			} else {
				reqLogger.Error(err, "Error trying to apply timing annotations to devworkspace")
			}
		}
	}
}

func getWorkspaceId(instance *devworkspace.DevWorkspace) (string, error) {
	uid, err := uuid.Parse(string(instance.UID))
	if err != nil {
		return "", err
	}
	return "workspace" + strings.Join(strings.Split(uid.String(), "-")[0:3], ""), nil
}

func (r *DevWorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// TODO: Set up indexing https://book.kubebuilder.io/cronjob-tutorial/controller-implementation.html#setup
	setupLog := ctrl.Log.WithName("devworkspace-controller-setup")
	operatorNamespace, err := cluster.GetOperatorNamespace()
	if err == nil {
		config.ConfigMapReference.Namespace = operatorNamespace
	} else {
		config.ConfigMapReference.Namespace = os.Getenv(cluster.WatchNamespaceEnvVar)
	}

	err = config.WatchControllerConfig(mgr)
	if err != nil {
		return err
	}

	// Check if we're running on OpenShift
	isOS, err := cluster.IsOpenShift()
	if err != nil {
		return err
	}
	config.ControllerCfg.SetIsOpenShift(isOS)

	err = config.ControllerCfg.Validate()
	if err != nil {
		setupLog.Error(err, "Controller configuration is invalid")
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&devworkspace.DevWorkspace{}).
		Owns(&appsv1.Deployment{}).
		Owns(&controllerv1alpha1.Component{}).
		Owns(&controllerv1alpha1.WorkspaceRouting{}).
		WithEventFilter(logObjects(ctrl.Log.WithName("predicates"))).
		Complete(r)
}


func logObjects(l logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(ev event.CreateEvent) bool {
			l.Info("CreateEvent is received", "kind", ev.Object.GetObjectKind().GroupVersionKind().Kind, "name",ev.Meta.GetName())
			return true
		},
		DeleteFunc: func(ev event.DeleteEvent) bool {
			l.Info("DeleteEvent is received", "kind", ev.Object.GetObjectKind(), "name",ev.Meta.GetName())
			return true
		},
		UpdateFunc: func(ev event.UpdateEvent) bool {
			l.Info("UpdateEvent is received", "kind", ev.ObjectOld.GetObjectKind().GroupVersionKind().Kind, "name", ev.MetaOld.GetName(), "type", reflect.TypeOf(ev.ObjectNew).String(),
				"diff", cmp.Diff(ev.ObjectNew, ev.ObjectOld, cmp.Options{}))

			return true
		},
		GenericFunc: func(ev event.GenericEvent) bool {
			l.Info("GenericEvent is received", "kind", ev.Object.GetObjectKind(), "name",ev.Meta.GetName())
			return true
		},
	}
}
