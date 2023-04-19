/*
Copyright 2023.

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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ocmv1alpha1 "github.com/rh-mobb/ocm-operator/api/v1alpha1"
	"github.com/rh-mobb/ocm-operator/pkg/ocm"
)

const (
	defaultMachinePoolRequeue time.Duration = 30
)

var (
	ErrMachinePoolReservedLabel = errors.New(
		fmt.Sprintf(
			"problem with system reserved labels: %s, %s",
			ocm.LabelPrefixManaged,
			ocm.LabelPrefixName,
		),
	)
)

// MachinePoolReconciler reconciles a MachinePool object
type MachinePoolReconciler struct {
	client.Client

	Context    context.Context
	Scheme     *runtime.Scheme
	Connection *sdk.Connection
}

// MachinePoolRequest is an object that is unique to each reconciliation
// request.
type MachinePoolRequest struct {
	Context           context.Context
	ControllerRequest ctrl.Request
	CurrentState      *ocmv1alpha1.MachinePool
	DesiredState      *ocmv1alpha1.MachinePool
	Client            *ocm.MachinePoolClient
	Log               logr.Logger
	NodesReady        bool
}

func NewRequest(ctx context.Context, req ctrl.Request) MachinePoolRequest {
	return MachinePoolRequest{
		CurrentState:      &ocmv1alpha1.MachinePool{},
		DesiredState:      &ocmv1alpha1.MachinePool{},
		ControllerRequest: req,
		Context:           ctx,
		Log:               log.FromContext(ctx),
	}
}

//+kubebuilder:rbac:groups=ocm.mobb.redhat.com,resources=machinepools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ocm.mobb.redhat.com,resources=machinepools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ocm.mobb.redhat.com,resources=machinepools/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MachinePool object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *MachinePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Context = ctx

	// create the request
	request := NewRequest(ctx, req)

	// run through each phase of controller reconciliation
	for _, phase := range []MachinePoolPhaseFunc{
		r.GetDesiredState,
		r.GetCurrentState,
		r.CreateOrUpdate,
		r.WaitUntilReady,
	} {
		// run each phase function and return if we receive any errors
		result, err := phase(&request)
		if err != nil || result.Requeue {
			return result, reconcilerError(
				request.ControllerRequest,
				"phase reconciliation error",
				err,
			)
		}
	}

	return noRequeue(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MachinePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ocmv1alpha1.MachinePool{}).
		Complete(r)
}
