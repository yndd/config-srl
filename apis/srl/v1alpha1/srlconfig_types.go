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

package v1alpha1

import (
	"reflect"

	nddv1 "github.com/yndd/ndd-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// A ConfigSpec defines the desired state of a Device.
type ConfigSpec struct {
	nddv1.ResourceSpec `json:",inline"`
	//+kubebuilder:pruning:PreserveUnknownFields
	//+kubebuilder:validation:Required
	Properties runtime.RawExtension `json:"properties,omitempty"`
}

// A ConfigStatus represents the observed state of a Device.
type ConfigStatus struct {
	nddv1.ResourceStatus `json:",inline"`
}

// +kubebuilder:object:root=true

// SrlConfig is the Schema for the Device API
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="TARGET",type="string",JSONPath=".status.conditions[?(@.kind=='TargetFound')].status"
// +kubebuilder:printcolumn:name="STATUS",type="string",JSONPath=".status.conditions[?(@.kind=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNC",type="string",JSONPath=".status.conditions[?(@.kind=='Synced')].status"
// +kubebuilder:printcolumn:name="ROOTPATH",type="string",JSONPath=".status.conditions[?(@.kind=='RootPathValidationSuccess')].status"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:categories={ndd,nddp}
type SrlConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigSpec   `json:"spec,omitempty"`
	Status ConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SrlConfigList contains a list of Devices
type SrlConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SrlConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SrlConfig{}, &SrlConfigList{})
}

// Device type metadata.
var (
	DeviceKindKind         = reflect.TypeOf(SrlConfig{}).Name()
	DeviceGroupKind        = schema.GroupKind{Group: Group, Kind: DeviceKindKind}.String()
	DeviceKindAPIVersion   = DeviceKindKind + "." + GroupVersion.String()
	DeviceGroupVersionKind = GroupVersion.WithKind(DeviceKindKind)
)
