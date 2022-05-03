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
	"strings"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	pkgv1 "github.com/yndd/ndd-core/apis/pkg/v1"
	"github.com/yndd/ndd-runtime/pkg/meta"
	"github.com/yndd/ndd-runtime/pkg/utils"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func (inv *inventory) renderWebhookMutate(podSpec pkgmetav1.PodSpec, c pkgmetav1.ContainerSpec, extra pkgmetav1.Extras, revision pkgv1.PackageRevision, crds []extv1.CustomResourceDefinition) *admissionv1.MutatingWebhookConfiguration { // nolint:interfacer,gocyclo
	certificateName := inv.getCertificateName(podSpec.Name, c.Container.Name, extra.Name)
	serviceName := inv.getServiceName(podSpec.Name, c.Container.Name, extra.Name)

	// TODO multiple crds
	crd := crds[0]
	v := getVersions(crd.Spec.Versions)

	//+kubebuilder:webhook:path=/mutate-srl3-nddp-yndd-io,mutating=true,failurePolicy=fail,sideEffects=None,groups=srl3.nddp.yndd.io,resources="*",verbs=create;update,versions=v1alpha1,name=mutate.srl3.nddp.yndd.io,admissionReviewVersions=v1

	failurePolicy := admissionv1.Fail
	sideEffect := admissionv1.SideEffectClassNone
	return &admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: strings.Join([]string{inv.crInfo.controllerConfigName, podSpec.Name, c.Container.Name, "mutating-webhook-configuration"}, "-"),
			Annotations: map[string]string{
				"cert-manager.io/inject-ca-from": strings.Join([]string{inv.crInfo.deployNamespace, certificateName}, "/"),
			},
			OwnerReferences: []metav1.OwnerReference{meta.AsController(meta.TypedReferenceTo(revision, pkgv1.ProviderRevisionGroupVersionKind))},
		},
		Webhooks: []admissionv1.MutatingWebhook{
			{
				Name:                    getMutatingWebhookName(crd.Spec.Names.Singular, crd.Spec.Group),
				AdmissionReviewVersions: []string{"v1"},
				ClientConfig: admissionv1.WebhookClientConfig{
					Service: &admissionv1.ServiceReference{
						Name:      serviceName,
						Namespace: inv.crInfo.deployNamespace,
						Path:      utils.StringPtr(strings.Join([]string{"/mutate", strings.ReplaceAll(crd.Spec.Group, ".", "-"), v[0], crd.Spec.Names.Singular}, "-")),
					},
				},
				Rules: []admissionv1.RuleWithOperations{
					{
						Rule: admissionv1.Rule{
							APIGroups:   []string{crd.Spec.Group},
							APIVersions: v,
							Resources:   []string{crd.Spec.Names.Plural},
						},
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
							admissionv1.Update,
						},
					},
				},
				FailurePolicy: &failurePolicy,
				SideEffects:   &sideEffect,
			},
		},
	}
}

func (inv *inventory) renderWebhookValidate(podSpec pkgmetav1.PodSpec, c pkgmetav1.ContainerSpec, extra pkgmetav1.Extras, revision pkgv1.PackageRevision, crds []extv1.CustomResourceDefinition) *admissionv1.ValidatingWebhookConfiguration { // nolint:interfacer,gocyclo
	certificateName := inv.getCertificateName(podSpec.Name, c.Container.Name, extra.Name)
	serviceName := inv.getServiceName(podSpec.Name, c.Container.Name, extra.Name)

	// TODO multiple crds
	crd := crds[0]
	v := getVersions(crd.Spec.Versions)

	failurePolicy := admissionv1.Fail
	sideEffect := admissionv1.SideEffectClassNone
	return &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: strings.Join([]string{inv.crInfo.controllerConfigName, podSpec.Name, c.Container.Name, "validating-webhook-configuration"}, "-"),
			Annotations: map[string]string{
				"cert-manager.io/inject-ca-from": strings.Join([]string{inv.crInfo.deployNamespace, certificateName}, "/"),
			},
			OwnerReferences: []metav1.OwnerReference{meta.AsController(meta.TypedReferenceTo(revision, pkgv1.ProviderRevisionGroupVersionKind))},
		},
		Webhooks: []admissionv1.ValidatingWebhook{
			{
				Name:                    getValidatingWebhookName(crd.Spec.Names.Singular, crd.Spec.Group),
				AdmissionReviewVersions: []string{"v1"},
				ClientConfig: admissionv1.WebhookClientConfig{
					Service: &admissionv1.ServiceReference{
						Name:      serviceName,
						Namespace: inv.crInfo.deployNamespace,
						Path:      utils.StringPtr(strings.Join([]string{"/validate", strings.ReplaceAll(crd.Spec.Group, ".", "-"), v[0], crd.Spec.Names.Singular}, "-")),
					},
				},
				Rules: []admissionv1.RuleWithOperations{
					{
						Rule: admissionv1.Rule{
							APIGroups:   []string{crd.Spec.Group},
							APIVersions: v,
							Resources:   []string{crd.Spec.Names.Plural},
						},
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
							admissionv1.Update,
						},
					},
				},
				FailurePolicy: &failurePolicy,
				SideEffects:   &sideEffect,
			},
		},
	}
}

func getVersions(crdVersions []extv1.CustomResourceDefinitionVersion) []string {
	versions := []string{}
	for _, crdVersion := range crdVersions {
		found := false
		for _, version := range versions {
			if crdVersion.Name == version {
				found = true
				break
			}

		}
		if !found {
			versions = append(versions, crdVersion.Name)
		}
	}
	return versions
}
