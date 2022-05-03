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
	"sort"

	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	pkgv1 "github.com/yndd/ndd-core/apis/pkg/v1"
	v1 "github.com/yndd/ndd-core/apis/pkg/v1"
	"github.com/yndd/ndd-runtime/pkg/meta"
	rbacv1 "k8s.io/api/rbac/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	verbsEdit   = []string{rbacv1.VerbAll}
	verbsView   = []string{"get", "list", "watch"}
	verbsSystem = []string{"get", "list", "watch", "update", "patch", "create", "delete"}
)

const (
	nameProviderPrefix = "ndd:provider:"
	//nameProviderMetricPrefix = "ndd:provider:metrics:"
	nameSuffixSystem = ":system"

	//valTrue = "true"

	suffixStatus = "/status"

	pluralEvents            = "events"
	pluralConfigmaps        = "configmaps"
	pluralSecrets           = "secrets"
	pluralLeases            = "leases"
	pluralServices          = "services"
	pluralNetworkNodes      = "networknodes"
	pluralNetworkNodeUsages = "networknodeusages"
	pluralDeployments       = "deployments"
	pluralStatefulsets      = "statefulsets"
	pluralPods              = "pods"
	pluralCrds              = "customresourcedefinitions"
)

var rulesSystemExtra = []rbacv1.PolicyRule{}

/*
var rulesSystemExtra = []rbacv1.PolicyRule{
	{
		APIGroups: []string{"*"},
		Resources: []string{pluralPods},
		Verbs:     verbsEdit,
	},
}
*/

// renderClusterRoles returns ClusterRoles
func (inv *inventory) renderClusterRoles(podSpec pkgmetav1.PodSpec, c pkgmetav1.ContainerSpec, revision pkgv1.PackageRevision, crds []extv1.CustomResourceDefinition) []rbacv1.ClusterRole {
	// Our list of CRDs has no guaranteed order, so we sort them in order to
	// ensure we don't reorder our RBAC rules on each update.
	sort.Slice(crds, func(i, j int) bool { return crds[i].GetName() < crds[j].GetName() })

	groups := make([]string, 0)            // Allows deterministic iteration over groups.
	resources := make(map[string][]string) // Resources by group.
	for _, crd := range crds {
		if _, ok := resources[crd.Spec.Group]; !ok {
			resources[crd.Spec.Group] = make([]string, 0)
			groups = append(groups, crd.Spec.Group)
		}
		resources[crd.Spec.Group] = append(resources[crd.Spec.Group],
			crd.Spec.Names.Plural,
			crd.Spec.Names.Plural+suffixStatus,
		)
	}

	rules := []rbacv1.PolicyRule{}
	for _, g := range groups {
		rules = append(rules, rbacv1.PolicyRule{
			APIGroups: []string{g},
			Resources: resources[g],
		})
	}

	roleName := inv.getRoleName(podSpec.Name, c.Container.Name)
	system := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: systemClusterProviderRoleName(roleName)},
		Rules:      append(append(withVerbs(rules, verbsSystem), rulesSystemExtra...), podSpec.PermissionRequests...),
	}

	roles := []rbacv1.ClusterRole{*system}
	for i := range roles {
		var ref metav1.OwnerReference

		ref = meta.AsController(meta.TypedReferenceTo(revision, v1.ProviderRevisionGroupVersionKind))

		roles[i].SetOwnerReferences([]metav1.OwnerReference{ref})
	}
	return roles
}

func withVerbs(r []rbacv1.PolicyRule, verbs []string) []rbacv1.PolicyRule {
	verbal := make([]rbacv1.PolicyRule, len(r))
	for i := range r {
		verbal[i] = r[i]
		verbal[i].Verbs = verbs
	}
	return verbal
}
