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

package controllers

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"

	//"github.com/yndd/ndda-network/internal/controllers/network"
	"github.com/yndd/ndd-config-srl/internal/controllers/target"
	//"github.com/yndd/ndd-config-srl/internal/controllers/srl"
	"github.com/yndd/ndd-target-runtime/pkg/shared"
)

// Setup package controllers.
func Setup(mgr ctrl.Manager, option controller.Options, nddcopts *shared.NddControllerOptions) (map[string]chan event.GenericEvent, error) {
	eventChans := make(map[string]chan event.GenericEvent)
	for _, setup := range []func(ctrl.Manager, controller.Options, *shared.NddControllerOptions) (string, chan event.GenericEvent, error){
		//srl.SetupDevice,
	} {
		gvk, eventChan, err := setup(mgr, option, nddcopts)
		if err != nil {
			return nil, err
		}
		eventChans[gvk] = eventChan
	}
	for _, setup := range []func(ctrl.Manager, controller.Options, *shared.NddControllerOptions) error{
		target.Setup,
	} {
		if err := setup(mgr, option, nddcopts); err != nil {
			return nil, err
		}
	}
	return eventChans, nil
}
