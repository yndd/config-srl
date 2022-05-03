//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright 2022 NDD.

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
// Code generated by ndd-gen. DO NOT EDIT.

package v1alpha1

import nddv1 "github.com/yndd/ndd-runtime/apis/common/v1"

// GetActive of this Srl3Device.
func (mg *SrlConfig) GetDeploymentPolicy() nddv1.DeploymentPolicy {
	return mg.Spec.DeploymentPolicy
}

// GetCondition of this Srl3Device.
func (mg *SrlConfig) GetCondition(ck nddv1.ConditionKind) nddv1.Condition {
	return mg.Status.GetCondition(ck)
}

// GetDeletionPolicy of this Srl3Device.
func (mg *SrlConfig) GetDeletionPolicy() nddv1.DeletionPolicy {
	return mg.Spec.DeletionPolicy
}

// GetTargetReference of this Srl3Device.
func (mg *SrlConfig) GetTargetReference() *nddv1.Reference {
	return mg.Spec.TargetReference
}

// SetRootPaths of this Srl3Device.
func (mg *SrlConfig) GetRootPaths() []string {
	return mg.Status.RootPaths
}

// SetHierPaths of this Srl3Device.
func (mg *SrlConfig) GetHierPaths() map[string][]string {
	return mg.Status.HierPaths
}

// SetActive of this Srl3Device.
func (mg *SrlConfig) SetDeploymentPolicy(b nddv1.DeploymentPolicy) {
	mg.Spec.DeploymentPolicy = b
}

// SetConditions of this Srl3Device.
func (mg *SrlConfig) SetConditions(c ...nddv1.Condition) {
	mg.Status.SetConditions(c...)
}

// SetDeletionPolicy of this Srl3Device.
func (mg *SrlConfig) SetDeletionPolicy(r nddv1.DeletionPolicy) {
	mg.Spec.DeletionPolicy = r
}

// SetTargetReference of this Srl3Device.
func (mg *SrlConfig) SetTargetReference(r *nddv1.Reference) {
	mg.Spec.TargetReference = r
}

// SetRootPaths of this Srl3Device.
func (mg *SrlConfig) SetRootPaths(n []string) {
	mg.Status.RootPaths = n
}

// SetHierPaths of this Srl3Device.
func (mg *SrlConfig) SetHierPaths(n map[string][]string) {
	mg.Status.HierPaths = n
}
