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

package target

import (
	"strings"

	"github.com/yndd/ndd-runtime/pkg/event"
	"github.com/yndd/ndd-runtime/pkg/resource"
	"github.com/yndd/ndd-runtime/pkg/shared"
	targetv1 "github.com/yndd/target/apis/target/v1"
	"github.com/yndd/target/pkg/reconciler/target"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupDevice adds a controller that reconciles Devices.
func Setup(mgr ctrl.Manager, nddopts *shared.NddControllerOptions) error {
	name := "target.yndd.io/" + strings.ToLower(targetv1.TargetKind)

	r := target.NewReconciler(mgr,
		target.WithPollInterval(nddopts.Poll),
		target.WithAddress(nddopts.GnmiAddress),
		target.WithExpectedVendorType(targetv1.VendorTypeNokiaSRL),
		target.WithLogger(nddopts.Logger.WithValues("Target", name)),
		target.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(nddopts.Copts).
		For(&targetv1.Target{}).
		WithEventFilter(resource.IgnoreUpdateWithoutGenerationChangePredicate()).
		Complete(r)
}
