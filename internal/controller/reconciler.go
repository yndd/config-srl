/*
Copyright 2021 NDD.

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

package controller

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"
	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	"github.com/yndd/ndd-runtime/pkg/event"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-runtime/pkg/meta"
	targetv1 "github.com/yndd/ndd-target-runtime/apis/dvr/v1"
	"github.com/yndd/ndd-target-runtime/pkg/resource"
	"github.com/yndd/ndd-target-runtime/pkg/shared"
	"github.com/yndd/ndd-target-runtime/pkg/ygotnddtarget"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// finalizer
	finalizerName = "finalizer.controller.srl.config.ndd.yndd.io"
	// timers
	defaultpollInterval = 1 * time.Minute
	shortWait           = 1 * time.Minute
	// errors
	errGetTarget               = "cannot get target cr"
	errGetTargetList           = "cannot get target cr list"
	errGetPod                  = "cannot get pod cr"
	errGetPodList              = "cannot get pod cr list"
	errGetPkgMeta              = "cannot get package meta cr"
	errGetCrd                  = "cannot get crd"
	errUpdateStatus            = "cannot update status"
	errApplyStatfullSet        = "cannot apply statefulset"
	errApplyCertificate        = "cannot apply certificate"
	errApplyService            = "cannot apply service"
	errApplyMutatingWebhook    = "cannot apply mutating webhook"
	errApplyValidatingWebhook  = "cannot apply validating webhook"
	errApplyClusterRoles       = "cannot apply clusterrole"
	errApplyClusterRoleBinding = "cannot apply clusterrolebinding"
	errApplyServiceAccount     = "cannot apply service account"
	//event
	reasonCreatedStatefullSet event.Reason = "CreatedStatefullSet"
	reasonAllocatedPod        event.Reason = "AllocatedPod"
)

// ReconcilerOption is used to configure the Reconciler.
type ReconcilerOption func(*Reconciler)

// Reconciler reconciles packages.
type Reconciler struct {
	client    resource.ClientApplicator
	finalizer resource.Finalizer

	expectedVendorType   ygotnddtarget.E_NddTarget_VendorType
	namespace            string
	controllerConfigName string
	revision             string
	revisionNamespace    string
	crdNames             []string
	pollInterval         time.Duration

	newTarget func() targetv1.Tg
	//newProviderRevision func() pkgv1.PackageRevision
	log    logging.Logger
	record event.Recorder
}

// WithNameSpace specifies the namespace of the controller
func WithNamespace(n string) ReconcilerOption {
	return func(r *Reconciler) {
		r.namespace = n
	}
}

// WithExpectedVendorType specifies the vendorType the reconciler cares about
func WithExpectedVendorType(t ygotnddtarget.E_NddTarget_VendorType) ReconcilerOption {
	return func(r *Reconciler) {
		r.expectedVendorType = t
	}
}

// WithControllerConfigName specifies the config name the controller uses
func WithControllerConfigName(n string) ReconcilerOption {
	return func(r *Reconciler) {
		r.controllerConfigName = n
	}
}

// WithRevision specifies the revision
func WithRevision(name, namespace string) ReconcilerOption {
	return func(r *Reconciler) {
		r.revision = name
		r.revisionNamespace = namespace
	}
}

// WithCrdNames specifies the crdNames in the reconciler
func WithCrdNames(n []string) ReconcilerOption {
	return func(r *Reconciler) {
		r.crdNames = n
	}
}

// WithLogger specifies how the Reconciler logs messages.
func WithLogger(l logging.Logger) ReconcilerOption {
	return func(r *Reconciler) {
		r.log = l
	}
}

// WithRecorder specifies how the Reconciler records events.
func WithRecorder(er event.Recorder) ReconcilerOption {
	return func(r *Reconciler) {
		r.record = er
	}
}

// SetupProvider adds a controller that reconciles Providers.
func Setup(mgr ctrl.Manager, o controller.Options, nddopts *shared.NddControllerOptions) error {
	name := "config-controller/" + strings.ToLower(targetv1.TargetGroupKind)

	r := NewReconciler(mgr,
		WithNamespace(nddopts.Namespace),
		WithControllerConfigName(nddopts.ControllerConfigName),
		WithRevision(nddopts.Revision, nddopts.RevisionNamespace),
		WithExpectedVendorType(ygotnddtarget.NddTarget_VendorType_nokia_srl),
		WithCrdNames(nddopts.CrdNames),
		WithLogger(nddopts.Logger),
		WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&targetv1.Target{}).
		Complete(r)
}

// NewReconciler creates a new package reconciler.
func NewReconciler(m ctrl.Manager, opts ...ReconcilerOption) *Reconciler {
	tg := func() targetv1.Tg { return &targetv1.Target{} }
	//pr := func() pkgv1.PackageRevision { return &pkgv1.ProviderRevision{} }

	r := &Reconciler{
		client: resource.ClientApplicator{
			Client:     m.GetClient(),
			Applicator: resource.NewAPIPatchingApplicator(m.GetClient()),
		},
		pollInterval: defaultpollInterval,
		log:          logging.NewNopLogger(),
		record:       event.NewNopRecorder(),
		newTarget:    tg,
		//newProviderRevision: pr,
		finalizer: resource.NewAPIFinalizer(m.GetClient(), finalizerName),
	}

	for _, f := range opts {
		f(r)
	}

	return r
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) { // nolint:gocyclo
	log := r.log.WithValues("NameSpaceName", req.NamespacedName)
	log.Debug("Target reconciler start...")

	// get the target info
	t := r.newTarget()
	if err := r.client.Get(ctx, req.NamespacedName, t); err != nil {
		// There's no need to requeue if we no longer exist. Otherwise we'll be
		// requeued implicitly because we return an error.
		log.Debug(errGetTarget, "error", err)
		return reconcile.Result{}, errors.Wrap(resource.IgnoreNotFound(err), errGetTarget)
	}

	record := r.record.WithAnnotations("external-name", meta.GetExternalName(t))

	// get the spec using the ygot schema
	tspec, err := t.GetSpec()
	if err != nil {
		log.Debug("Cannot get spec", "error", err)
		return reconcile.Result{Requeue: true}, errors.Wrap(err, "cannot get spec")
	}

	// get the provider revision to get the information for the controller to operate
	pkgMeta := &pkgmetav1.Provider{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: r.controllerConfigName}, pkgMeta); err != nil {
		log.Debug(errGetPkgMeta, "error", err)
		return reconcile.Result{Requeue: true}, errors.Wrap(err, errGetPkgMeta)
	}

	log.Debug("controller config", "config", pkgMeta.Spec)

	// if expectedVendorType is unset we dont care about it and can proceed,
	// if it is set we should see if the Target CR vendor type matches the
	// expected vendorType
	if r.expectedVendorType != ygotnddtarget.NddTarget_VendorType_undefined {
		// expected vendor type is set, so we compare expected and configured vendor Type

		// if the expected vendor type does not match we return as the CR is not
		// relevant to proceed
		if r.expectedVendorType != tspec.VendorType {
			log.Debug("unexpected vendor type", "crVendorType", tspec.VendorType, "expectedVendorType", r.expectedVendorType)
			// stop the reconcile process as we should not be processing this cr; the vendor type is not expected
			return reconcile.Result{}, nil
		}
	}

	// get annotations from the target cr
	a := t.GetAnnotations()
	// initialize the dynamic inventory
	inv := newInventory(r.client, r.log, &crInfo{
		expectedVendorType:   r.expectedVendorType,
		controllerConfigName: r.controllerConfigName,
		revisionName:         r.revision,
		revisionNamespace:    r.revisionNamespace,
		deployNamespace:      r.namespace,
		targetNamespace:      t.GetNamespace(),
		targetName:           t.GetName(),
		crdNames:             r.crdNames,
		pkgInfo:              pkgMeta,
	})
	// validateAnnotations validate based on the pkgMeta spec if controller
	// pod keys exists in the annotation. This indicates that an allocation was
	// existing.
	annotationExists := inv.validateAnnotations(a)

	if meta.WasDeleted(t) {
		// cr got deleted
		if annotationExists {
			// TODO check if there are still targets left -> if so scale back in
		}
		// Delete finalizer after the object is deleted
		if err := r.finalizer.RemoveFinalizer(ctx, t); err != nil {
			log.Debug("Cannot remove target cr finalizer", "error", err)
			return reconcile.Result{Requeue: true}, errors.Wrap(err, "cannot remove finalizer")
		}
		return reconcile.Result{Requeue: false}, errors.Wrap(r.client.Update(ctx, t), "cannot remove finalizer")
	}

	// Add a finalizer
	if err := r.finalizer.AddFinalizer(ctx, t); err != nil {
		log.Debug("cannot add finalizer", "error", err)
		return reconcile.Result{Requeue: true}, errors.Wrap(r.client.Update(ctx, t), "cannot add finalizer")
	}

	// getCrds retrieves the crds from the k8s api based on the crNames
	// coming from the flags
	crds, err := inv.getCrds(ctx, r.crdNames)
	if err != nil {
		log.Debug("cannot get crds", "error", err)
		return reconcile.Result{Requeue: true}, errors.Wrap(err, "cannot get crds")
	}

	// we always deploy since this allows us to handle updates of the deploySpec
	if err := inv.deploy(ctx, crds); err != nil {
		log.Debug("cannot deploy", "error", err)
		return reconcile.Result{}, errors.Wrap(err, "cannot deploy")
	}

	// check the pod list
	podsExists, err := inv.getPods(ctx)
	if err != nil {
		log.Debug("cannot get pods", "error", err)
		return reconcile.Result{}, errors.Wrap(err, "cannot get pods")
	}
	if !podsExists {
		log.Debug("no pods exist", "annotation", a)
		return reconcile.Result{RequeueAfter: shortWait}, nil
	}

	if !annotationExists {
		log.Debug("annotation does not exist")
		// ANNOTATION DOES NOT EXIST -> allocate the target to the deployments and/or deploy proxy/provider

		// update inventory to get overview on all targets and how they are allocated
		if err := inv.updateInventory(ctx); err != nil {
			log.Debug("cannot update inventory", "error", err)
			return reconcile.Result{}, errors.Wrap(resource.IgnoreNotFound(err), "cannot update inventory")
		}

		// allocate the target to the pods
		if err := inv.allocate(); err != nil {
			log.Debug("allocate", "error", err)
			return reconcile.Result{}, errors.Wrap(resource.IgnoreNotFound(err), "allocate")
		}

		// add the annotations to the target
		meta.AddAnnotations(t, inv.getAnnotations())
		log.Debug("target allocation successfull")
		record.Event(t, event.Normal(reasonCreatedStatefullSet, "Created statefullset"))
		return reconcile.Result{RequeueAfter: r.pollInterval}, errors.Wrap(r.client.Update(ctx, t), "cannot update annotations")
	}
	log.Debug("annotation exists")
	// ANNOTATION EXISTS -> validate if the deployments exists, if yes all ok;
	// if not delete the annotation/finalizer and reconcile to allocate the deployment

	// validatePods -> validate if the pod that was allocated exists and is deployed
	podsValidated, err := inv.validatePods(ctx)
	if err != nil {
		log.Debug("cannot validate deployment", "error", err)
		return reconcile.Result{Requeue: true}, errors.Wrap(err, "cannot validate pods")
	}
	if !podsValidated {
		log.Debug("validateDeployments annotation exists but no pod strange, how did we get here", "annotation", a)
		// remove the annotations from the target
		meta.RemoveAnnotations(t, inv.getAnnotationKeys()...)
		// remove finalizer
		if err := r.finalizer.RemoveFinalizer(ctx, t); err != nil {
			log.Debug("cannot remove target cr finalizer", "error", err)
			return reconcile.Result{Requeue: true}, errors.Wrap(r.client.Update(ctx, t), "cannot remove target cr finalizer")
		}
		// reconcile again immediately to create the deployment
		return reconcile.Result{RequeueAfter: shortWait}, nil
	}

	log.Debug("target allocation and validation successfull")
	return reconcile.Result{RequeueAfter: r.pollInterval}, nil
}
