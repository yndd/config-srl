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
	"fmt"

	"github.com/pkg/errors"
	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	pkgv1 "github.com/yndd/ndd-core/apis/pkg/v1"
	"github.com/yndd/ndd-runtime/pkg/logging"
	targetv1 "github.com/yndd/ndd-target-runtime/apis/dvr/v1"
	"github.com/yndd/ndd-target-runtime/pkg/resource"
	"github.com/yndd/ndd-target-runtime/pkg/ygotnddtarget"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	parentLabel = "dvr.ndd.yndd.io/controller"
)

// annotation -> map[<controllername>-<sfset-x>]<sfset-x-instance>

type inventory struct {
	log           logging.Logger
	client        resource.ClientApplicator
	crInfo        *crInfo
	newTargetList func() targetv1.TgList
	allocations   map[string]*targets // key is the real podName, with fq targetName
	annotation    map[string]string   // key is the controllerPodKey, value is podName
	podList       map[string][]string // key is the controllerPodKey, value is list of podNames
}

type targets struct {
	targets map[string]struct{} // key is fq target name
}

type crInfo struct {
	expectedVendorType   ygotnddtarget.E_NddTarget_VendorType
	controllerConfigName string
	deployNamespace      string
	revisionName         string
	revisionNamespace    string
	targetName           string
	targetNamespace      string
	crdNames             []string
	ctrlMetaCfg          *pkgmetav1.ControllerConfig
}

func newInventory(client resource.ClientApplicator, l logging.Logger, allocallocationInfo *crInfo) *inventory {
	tgl := func() targetv1.TgList { return &targetv1.TargetList{} }

	return &inventory{
		client:        client,
		log:           l,
		newTargetList: tgl,
		crInfo:        allocallocationInfo,
		allocations:   map[string]*targets{}, // key = podName
		annotation:    map[string]string{},
		podList:       map[string][]string{},
	}
}

// validateAnnotations validate based on the pkgMeta spec if controller
// pod keys exists in the annotation. This indicates that an allocation was
// existing.
// Also the inventory annotatiosn are initialized
func (inv *inventory) validateAnnotations(a map[string]string) bool {
	for _, podSpec := range inv.crInfo.ctrlMetaCfg.Spec.Pods {
		podName, ok := a[inv.getControllerPodKey(podSpec.Name)]
		if !ok {
			// reset the annotation in the inventory
			inv.annotation = map[string]string{}
			return false
		}
		// initialize the annotation in the inventory focussed on the controller pod Key
		inv.annotation[inv.getControllerPodKey(podSpec.Name)] = podName
	}
	return true
}

// getAnnotations returns the annotations from the iventory
func (inv *inventory) getAnnotations() map[string]string {
	return inv.annotation
}

// getAnnotationKeys returns the annoation keys
func (inv *inventory) getAnnotationKeys() []string {
	var keys []string
	for key := range inv.annotation {
		keys = append(keys, key)
	}
	return keys
}

// getPods initializes the pods in the inventory and returns if they are found
func (inv *inventory) getPods(ctx context.Context) (bool, error) {
	log := inv.log.WithValues("target", inv.crInfo.targetName)
	for _, podSpec := range inv.crInfo.ctrlMetaCfg.Spec.Pods {

		opts := []client.ListOption{
			client.InNamespace(inv.crInfo.deployNamespace),
			client.MatchingLabels(inv.getRevisionLabel(podSpec)),
		}
		log.Debug("get pods option", "podSpec", podSpec.Name, "options", opts)

		podList := &corev1.PodList{}
		if err := inv.client.List(ctx, podList, opts...); err != nil {
			// error getting Pod
			log.Debug(errGetPod, "error", err)
			// resource not found
			return false, err
		}
		if len(podList.Items) == 0 {
			// no pods found
			return false, nil
		}
		pl := []string{}
		for _, pod := range podList.Items {
			pl = append(pl, pod.Name)
		}
		inv.podList[inv.getControllerPodKey(podSpec.Name)] = pl
	}
	return true, nil
}

// validatePods validates based on the annotation if the
// podkeys and podNames exists
func (inv *inventory) validatePods(ctx context.Context) (bool, error) {
	log := inv.log.WithValues("target", inv.crInfo.targetName, "annotation", inv.annotation, "podList", inv.podList)
	log.Debug("validatePods...", "annotation", inv.annotation, "")

	for controllerPodKey, podName := range inv.annotation {
		podList, ok := inv.podList[controllerPodKey]
		if !ok {
			// podNameKey not found
			log.Debug("validatePods podNameKey not found", "controllerPodKey", controllerPodKey)
			return false, nil
		}
		found := false
		for _, podListname := range podList {
			if podListname == podName {
				found = true
				break
			}
		}
		if !found {
			// podName not found
			log.Debug("validatePods podName not found", "podName", podName)
			return false, nil
		}
	}
	return true, nil
}

// updateInventory updates the inventory
func (inv *inventory) updateInventory(ctx context.Context) error {
	log := inv.log.WithValues()
	log.Debug("updateInventory...")

	// list targets and get annotation list
	targetlist := inv.newTargetList()
	if err := inv.client.List(ctx, targetlist); resource.IgnoreNotFound(err) != nil {
		log.Debug(errGetTargetList, "error", err)
		return err
	}

	// provide a list of all the targets per deployment
	for _, t := range targetlist.GetTargets() {
		tspec, err := t.GetSpec()
		if err != nil {
			return err
		}
		// if expectedVendorType is unset we dont care about it and can proceed,
		// if it is set we should see if the Target CR vendor type matches the
		// expected vendorType
		if inv.crInfo.expectedVendorType != ygotnddtarget.NddTarget_VendorType_undefined {
			// expected vendor type is set, so we compare expected and configured vendor Type

			// if the expected vendor type does not match we return as the CR is not
			// relevant to proceed
			if inv.crInfo.expectedVendorType != tspec.VendorType {
				// this is not an expected vendor Type so we skip this
				break
			}
		}
		// based on the allocations, check if the controllerPodKey exist

		for controllerPodKey, podName := range t.GetAnnotations() {
			for _, podSpec := range inv.crInfo.ctrlMetaCfg.Spec.Pods {
				if controllerPodKey == inv.getControllerPodKey(podSpec.Name) {
					if _, ok := inv.allocations[podName]; !ok {
						inv.allocations[podName] = &targets{}
						inv.allocations[podName].targets = make(map[string]struct{})
					}
					inv.allocations[podName].targets[getFqTargetName(t.GetNamespace(), t.GetName())] = struct{}{}
				}
			}
		}
	}

	log.Debug("updateInventory", "allocations", inv.allocations)
	return nil
}

func (inv *inventory) validatePod(ctx context.Context) (*podObservation, error) {
	log := inv.log.WithValues()
	log.Debug("validatePod...")

	// list pods
	podList := &corev1.PodList{}
	if err := inv.client.List(ctx, podList,
		client.MatchingLabels(map[string]string{parentLabel: inv.crInfo.ctrlMetaCfg.Name})); resource.IgnoreNotFound(err) != nil {
		log.Debug(errGetPodList, "error", err)
		return &podObservation{}, err
	}

	found := false
	for _, podName := range inv.annotation {
		found = false
		for _, pod := range podList.Items {
			if pod.Name == podName {
				found = true
				break
			}
		}
		if !found {
			break
		}
	}

	if !found {
		return &podObservation{}, nil
	}
	return &podObservation{exists: true}, nil
}

func (inv *inventory) allocate() error {
	log := inv.log.WithValues("target", inv.crInfo.targetName)
	log.Debug("allocate...")
	// find least loaded pod
	// loop over the sfSetKeys
	for _, podSpec := range inv.crInfo.ctrlMetaCfg.Spec.Pods {
		leastLoaded := 9999
		var leastLoadedPodName string

		// podList contains the available pods we can use to allocate from
		podList, ok := inv.podList[inv.getControllerPodKey(podSpec.Name)]
		if !ok {
			log.Debug("controllerPodkey not found in podList", "controllerPodkey", inv.getControllerPodKey(podSpec.Name))
			return fmt.Errorf("controllerPodkey: %s not found in podList", inv.getControllerPodKey(podSpec.Name))
		}

		// loop over the available pods and check the allocations
		for _, podName := range podList {
			t, ok := inv.allocations[podName]
			if !ok {
				// 0 allocations -> we can pick this one
				log.Debug("allocated with no targets found", "podName", podName)
				leastLoadedPodName = podName
				break
			}
			if len(t.targets) < leastLoaded {
				log.Debug("allocation improved", "leastloaded", leastLoaded, "targets", len(t.targets), "podName", podName)
				leastLoaded = len(t.targets)
				leastLoadedPodName = podName
			}
		}

		if leastLoadedPodName != "" {
			log.Debug("no podName allocate")
			return errors.New("no podName allocated")
		}
		inv.annotation[inv.getControllerPodKey(podSpec.Name)] = leastLoadedPodName
	}
	return nil
}

type podObservation struct {
	exists bool
}

// deploy deploys the k8s resources that are managed by this controller
// clusterrole, clusterrolebindings
// statefulset/deployments, serviceaccounts
// certificates, services, webhooks
func (inv *inventory) deploy(ctx context.Context, crds []extv1.CustomResourceDefinition) error {
	log := inv.log.WithValues("target", inv.crInfo.targetName)
	log.Debug("getCrds...")
	newPackageRevision := func() pkgv1.PackageRevision { return &pkgv1.ProviderRevision{} }

	// this is used to automatically cleanup the created resources when deleting the provider revision
	// By assigning the owner reference to the newly created resources we take care of this
	r := newPackageRevision()
	if err := inv.client.Get(ctx, types.NamespacedName{
		Namespace: inv.crInfo.revisionNamespace,
		Name:      inv.crInfo.revisionName}, r); err != nil {
		return err
	}

	for _, podSpec := range inv.crInfo.ctrlMetaCfg.Spec.Pods {
		for _, c := range podSpec.Containers {
			for _, cr := range inv.renderClusterRoles(podSpec, c, r, crds) {
				cr := cr // Pin range variable so we can take its address.
				log.WithValues("role-name", cr.GetName())
				if err := inv.client.Apply(ctx, &cr); err != nil {
					return errors.Wrap(err, errApplyClusterRoles)
				}
				log.Debug("Applied RBAC ClusterRole")
			}

			crb := inv.renderClusterRoleBinding(podSpec, c, r)
			if err := inv.client.Apply(ctx, crb); err != nil {
				return errors.Wrap(err, errApplyClusterRoleBinding)
			}
			log.Debug("Applied RBAC ClusterRole")

			for _, extra := range c.Extras {
				if extra.Certificate {
					// deploy a certificate
					cert := inv.renderCertificate(podSpec, c, extra, r)
					if err := inv.client.Apply(ctx, cert); err != nil {
						return errors.Wrap(err, errApplyCertificate)
					}
				}
				if extra.Service {
					// deploy a webhook service
					s := inv.renderService(podSpec, c, extra, r)
					if err := inv.client.Apply(ctx, s); err != nil {
						return errors.Wrap(err, errApplyService)
					}
				}
				if extra.Webhook {
					// deploy a mutating webhook
					whMutate := inv.renderWebhookMutate(podSpec, c, extra, r, crds)
					if err := inv.client.Apply(ctx, whMutate); err != nil {
						return errors.Wrap(err, errApplyMutatingWebhook)
					}
					// deploy a validating webhook
					whValidate := inv.renderWebhookValidate(podSpec, c, extra, r, crds)
					if err := inv.client.Apply(ctx, whValidate); err != nil {
						return errors.Wrap(err, errApplyValidatingWebhook)
					}
				}
			}
		}
		switch podSpec.Type {
		case pkgmetav1.DeploymentTypeDeployment:
		case pkgmetav1.DeploymentTypeStatefulset:
			s := inv.renderStatefulSet(podSpec, r)
			if err := inv.client.Apply(ctx, s); err != nil {
				return errors.Wrap(err, errApplyStatfullSet)
			}
			sa := inv.renderServiceAccount(podSpec, r)
			if err := inv.client.Apply(ctx, sa); err != nil {
				return errors.Wrap(err, errApplyServiceAccount)
			}
		}
	}
	return nil
}

// getCrds retrieves the crds from the k8s api based on the crNames
// coming from the flags
func (inv *inventory) getCrds(ctx context.Context, crdNames []string) ([]extv1.CustomResourceDefinition, error) {
	log := inv.log.WithValues("target", inv.crInfo.targetName, "crdNames", crdNames)
	log.Debug("getCrds...")

	crds := []extv1.CustomResourceDefinition{}
	for _, crdName := range crdNames {
		crd := &extv1.CustomResourceDefinition{}
		// namespace is not relevant for the crd, hence it is not supplied
		if err := inv.client.Get(ctx, types.NamespacedName{Name: crdName}, crd); err != nil {
			return nil, errors.Wrap(err, errGetCrd)
		}
		crds = append(crds, *crd)
	}

	return crds, nil
}
