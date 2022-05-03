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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	pkgv1 "github.com/yndd/ndd-core/apis/pkg/v1"
	"github.com/yndd/ndd-runtime/pkg/meta"
	corev1 "k8s.io/api/core/v1"
)

func (inv *inventory) renderService(podSpec pkgmetav1.PodSpec, c pkgmetav1.ContainerSpec, extra pkgmetav1.Extras, revision pkgv1.PackageRevision) *corev1.Service { // nolint:interfacer,gocyclo
	serviceName := inv.getServiceName(podSpec.Name, c.Container.Name, extra.Name)

	port := int32(443)

	if extra.Port != 0 {
		port = int32(extra.Port)
	}
	protocol := corev1.Protocol("TCP")
	if extra.Protocol != "" {
		protocol = corev1.Protocol(extra.Protocol)
	}
	targetPort := int(8443)
	if extra.TargetPort != 0 {
		targetPort = int(targetPort)
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: inv.crInfo.deployNamespace,
			Labels: map[string]string{
				getLabelKey(extra.Name): serviceName,
			},
			OwnerReferences: []metav1.OwnerReference{meta.AsController(meta.TypedReferenceTo(revision, pkgv1.ProviderRevisionGroupVersionKind))},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				getLabelKey(extra.Name): serviceName,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       extra.Name,
					Port:       port,
					TargetPort: intstr.FromInt(targetPort),
					Protocol:   protocol,
				},
			},
		},
	}
}
