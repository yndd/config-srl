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
	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	pkgv1 "github.com/yndd/ndd-core/apis/pkg/v1"
	"github.com/yndd/ndd-runtime/pkg/meta"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kindClusterRole = "ClusterRole"
)

// renderClusterRoles returns ClusterRoles
func (inv *inventory) renderClusterRoleBinding(podSpec pkgmetav1.PodSpec, c pkgmetav1.ContainerSpec, revision pkgv1.PackageRevision) *rbacv1.ClusterRoleBinding {
	roleName := inv.getRoleName(podSpec.Name, c.Container.Name)
	cbrName := systemClusterProviderRoleName(roleName)

	ref := meta.AsController(meta.TypedReferenceTo(revision, pkgv1.ProviderRevisionGroupVersionKind))

	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            cbrName,
			OwnerReferences: []metav1.OwnerReference{ref},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     kindClusterRole,
			Name:     cbrName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Namespace: inv.crInfo.deployNamespace,
				Name:      inv.getControllerPodKey(podSpec.Name),
			},
		},
	}
}
