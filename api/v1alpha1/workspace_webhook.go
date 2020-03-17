/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var workspacelog = logf.Log.WithName("workspace-resource")

func (r *Workspace) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-workspace-che-eclipse-org-my-domain-v1alpha1-workspace,mutating=true,failurePolicy=fail,groups=workspace.che.eclipse.org.my.domain,resources=workspaces,verbs=create;update,versions=v1alpha1,name=mworkspace.kb.io

var _ webhook.Defaulter = &Workspace{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Workspace) Default() {
	workspacelog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:verbs=create;update,path=/validate-workspace-che-eclipse-org-my-domain-v1alpha1-workspace,mutating=false,failurePolicy=fail,groups=workspace.che.eclipse.org.my.domain,resources=workspaces,versions=v1alpha1,name=vworkspace.kb.io

var _ webhook.Validator = &Workspace{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Workspace) ValidateCreate() error {
	workspacelog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Workspace) ValidateUpdate(old runtime.Object) error {
	workspacelog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Workspace) ValidateDelete() error {
	workspacelog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
