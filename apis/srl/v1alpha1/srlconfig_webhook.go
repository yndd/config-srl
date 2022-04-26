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

	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/yndd/ndd-config-srl/pkg/ygotsrl"
	"github.com/yndd/ndd-runtime/pkg/model"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var srlconfiglog = logf.Log.WithName("srlconfig-resource-webhook")
var m = &model.Model{
	ModelData:       make([]*gnmi.ModelData, 0),
	StructRootType:  reflect.TypeOf((*ygotsrl.Device)(nil)),
	SchemaTreeRoot:  ygotsrl.SchemaTree["Device"],
	JsonUnmarshaler: ygotsrl.Unmarshal,
	EnumData:        ygotsrl.ΛEnum,
}

func (r *SrlConfig) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-srl-config-ndd-yndd-io-v1alpha1-srlconfig,mutating=true,failurePolicy=fail,sideEffects=None,groups=srl.config.ndd.yndd.io,resources="*",verbs=create;update,versions=v1alpha1,name=mutate.srl.config.ndd.yndd.io,admissionReviewVersions=v1
var _ webhook.Defaulter = &SrlConfig{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *SrlConfig) Default() {
	srlconfiglog.Info("webhook default", "name", r.Name)

	/*
		b, err := json.Marshal(r.Spec.Properties)
		if err != nil {
			srl3devicelog.Info("default", "marshal error", err.Error())
		}

		goStruct, err := m.NewConfigStruct(b, false)
		if err != nil {
			srl3devicelog.Info("default", "unmarshal error", err.Error())
		}
		deviceStruct, ok := goStruct.(*ygotsrl.Device)
		if !ok {
			srl3devicelog.Info("default", "casting error", err.Error())
		}
		deviceStruct.PopulateDefaults()

		json, err := ygot.EmitJSON(deviceStruct, &ygot.EmitJSONConfig{
			Format:         ygot.RFC7951,
			RFC7951Config:  &ygot.RFC7951JSONConfig{},
			SkipValidation: true,
		})
		if err != nil {
			srl3devicelog.Info("default", "emit Json error", err.Error())
		}
		srl3devicelog.Info("default", "json", json)
	*/

}

//+kubebuilder:webhook:path=/validate-srl-config-ndd-yndd-io-v1alpha1-srlconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=srl.config.ndd.yndd.io,resources="*",verbs=create;update,versions=v1alpha1,name=validate.srl.config.ndd.yndd.io,admissionReviewVersions=v1

var _ webhook.Validator = &SrlConfig{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *SrlConfig) ValidateCreate() error {
	srlconfiglog.Info("validate create", "name", r.Name)
	//srl3devicelog.Info("validate create", "name", r.Name, "device spec", string(r.Spec.Properties.Raw))

	var allErrs field.ErrorList

	// TBD check if network node reference exists

	// validate the spec
	if err := ValidateSpec(r.Spec.Properties.Raw); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "srl3.nddp.yndd.io", Kind: "Srl3Device"},
		r.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *SrlConfig) ValidateUpdate(old runtime.Object) error {
	srlconfiglog.Info("validate update", "name", r.Name)
	//srl3devicelog.Info("validate update", "name", r.Name, "device spec", string(r.Spec.Properties.Raw))
	var allErrs field.ErrorList

	// TODO check if the node reference changed

	// validate the spec
	if err := ValidateSpec(r.Spec.Properties.Raw); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "srl3.nddp.yndd.io", Kind: "Srl3Device"},
		r.Name, allErrs)

}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *SrlConfig) ValidateDelete() error {
	//srl3devicelog.Info("validate delete", "name", r.Name, "device spec", string(r.Spec.Properties.Raw))
	srlconfiglog.Info("validate delete", "name", r.Name)
	return nil
}
