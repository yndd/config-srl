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

package srl

import (
	"context"
	"encoding/json"
	"reflect"

	//"strings"
	"time"

	"github.com/karimra/gnmic/target"
	gnmitypes "github.com/karimra/gnmic/types"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/ygot/ygot"
	"github.com/openconfig/ygot/ytypes"
	"github.com/pkg/errors"
	srlv1alpha1 "github.com/yndd/ndd-config-srl/apis/srl/v1alpha1"
	"github.com/yndd/ndd-config-srl/pkg/ygotsrl"
	nddv1 "github.com/yndd/ndd-runtime/apis/common/v1"
	"github.com/yndd/ndd-runtime/pkg/event"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-runtime/pkg/model"
	"github.com/yndd/ndd-runtime/pkg/reconciler/managed"
	"github.com/yndd/ndd-runtime/pkg/resource"
	"github.com/yndd/ndd-runtime/pkg/utils"
	targetv1 "github.com/yndd/ndd-target-runtime/apis/dvr/v1"
	"github.com/yndd/ndd-target-runtime/pkg/cachename"
	"github.com/yndd/ndd-target-runtime/pkg/rootpaths"
	"github.com/yndd/ndd-target-runtime/pkg/shared"
	"github.com/yndd/ndd-yang/pkg/yparser"
	"github.com/yndd/nddp-system/pkg/gvkresource"
	"github.com/yndd/nddp-system/pkg/ygotnddp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cevent "sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// Errors
	errUnexpectedDevice       = "the managed resource is not a Device resource"
	errKubeUpdateFailedDevice = "cannot update Device"
	errReadDevice             = "cannot read Device"
	errCreateDevice           = "cannot create Device"
	errUpdateDevice           = "cannot update Device"
	errDeleteDevice           = "cannot delete Device"
)

// SetupDevice adds a controller that reconciles Devices.
func Setup(mgr ctrl.Manager, nddopts *shared.NddControllerOptions) (string, chan cevent.GenericEvent, error) {
	//func SetupDevice(mgr ctrl.Manager, o controller.Options, nddcopts *shared.NddControllerOptions) error {

	name := managed.ControllerName(srlv1alpha1.DeviceGroupKind)

	events := make(chan cevent.GenericEvent)

	dm := &model.Model{
		StructRootType:  reflect.TypeOf((*ygotsrl.Device)(nil)),
		SchemaTreeRoot:  ygotsrl.SchemaTree["Device"],
		JsonUnmarshaler: ygotsrl.Unmarshal,
		EnumData:        ygotsrl.ΛEnum,
	}

	sm := &model.Model{
		StructRootType:  reflect.TypeOf((*ygotnddp.Device)(nil)),
		SchemaTreeRoot:  ygotnddp.SchemaTree["Device"],
		JsonUnmarshaler: ygotnddp.Unmarshal,
		EnumData:        ygotnddp.ΛEnum,
	}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(srlv1alpha1.DeviceGroupVersionKind),
		managed.WithPollInterval(nddopts.Poll),
		managed.WithExternalConnecter(&connectorDevice{
			log:   nddopts.Logger,
			kube:  mgr.GetClient(),
			usage: resource.NewTargetUsageTracker(mgr.GetClient(), &targetv1.TargetUsage{}),
			//deviceSchema: nddcopts.DeviceSchema,
			//nddpSchema:   nddcopts.NddpSchema,
			deviceModel: dm,
			systemModel: sm,
			newClientFn: target.NewTarget,
			gnmiAddress: nddopts.GnmiAddress},
		),
		managed.WithValidator(&validatorDevice{
			log:         nddopts.Logger,
			deviceModel: dm,
			systemModel: sm,
		},
		),
		managed.WithLogger(nddopts.Logger.WithValues("Srl3Device", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))))

	DeviceHandler := &EnqueueRequestForAllDevice{
		client: mgr.GetClient(),
		log:    nddopts.Logger,
		ctx:    context.Background(),
	}

	//return ctrl.NewControllerManagedBy(mgr).
	return srlv1alpha1.DeviceGroupKind, events, ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(nddopts.Copts).
		For(&srlv1alpha1.SrlConfig{}).
		Owns(&srlv1alpha1.SrlConfig{}).
		WithEventFilter(resource.IgnoreUpdateWithoutGenerationChangePredicate()).
		/*
			Watches(
				&source.Channel{Source: events},
				&handler.EnqueueRequestForObject{},
			).
		*/
		Watches(&source.Kind{Type: &srlv1alpha1.SrlConfig{}}, DeviceHandler).
		Watches(&source.Channel{Source: events}, DeviceHandler).
		Complete(r)
}

type validatorDevice struct {
	log         logging.Logger
	deviceModel *model.Model
	systemModel *model.Model
}

// GetCrStatus validates the status if the CR in the system
func (v *validatorDevice) GetCrStatus(ctx context.Context, mg resource.Managed, systemCfg *ygotnddp.Device) (managed.CrObservation, error) {
	log := v.log.WithValues("Resource", mg.GetName())
	log.Debug("validate GetCrStatus ...")

	if *systemCfg.Cache.Exhausted != 0 {
		return managed.CrObservation{
			Exhausted: true,
		}, nil
	}

	gvkName := gvkresource.GetGvkName(mg)

	gvk, exists := systemCfg.Gvk[gvkName]
	if !exists {
		return managed.CrObservation{
			Exists: false,
		}, nil
	}
	switch gvk.Status {
	case ygotnddp.NddpSystem_ResourceStatus_PENDING:
		return managed.CrObservation{
			Exists:  true,
			Pending: true,
		}, nil
	case ygotnddp.NddpSystem_ResourceStatus_FAILED:
		return managed.CrObservation{
			Exists:  true,
			Failed:  true,
			Message: *gvk.Reason,
		}, nil
	}

	return managed.CrObservation{
		Exists: true,
	}, nil
}

func (v *validatorDevice) ValidateCrSpecUpdate(ctx context.Context, mg resource.Managed, runningCfg []byte) (managed.CrSpecObservation, error) {
	log := v.log.WithValues("Resource", mg.GetName())
	log.Debug("validate ValidateCrSpecUpdate ...")

	cr, ok := mg.(*srlv1alpha1.SrlConfig)
	if !ok {
		return managed.CrSpecObservation{}, errors.New(errUnexpectedDevice)
	}

	// Validate if the spec has any issues when merged with the actual config
	runGoStruct, err := v.deviceModel.NewConfigStruct(runningCfg, true)
	if err != nil {
		return managed.CrSpecObservation{}, err
	}

	specGoStruct, err := v.deviceModel.NewConfigStruct(cr.Spec.Properties.Raw, false)
	if err != nil {
		return managed.CrSpecObservation{}, err
	}

	if err := ygot.MergeStructInto(runGoStruct, specGoStruct, &ygot.MergeOverwriteExistingFields{}); err != nil {
		return managed.CrSpecObservation{}, err
	}

	if err := runGoStruct.Validate(); err != nil {
		return managed.CrSpecObservation{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	return managed.CrSpecObservation{
		Success: true,
	}, nil
}

func (v *validatorDevice) ValidateCrSpecDelete(ctx context.Context, mg resource.Managed, runningCfg []byte) (managed.CrSpecObservation, error) {
	log := v.log.WithValues("Resource", mg.GetName())
	log.Debug("validate ValidateCrSpecDelete ...")

	cr, ok := mg.(*srlv1alpha1.SrlConfig)
	if !ok {
		return managed.CrSpecObservation{}, errors.New(errUnexpectedDevice)
	}

	// Validate if the spec has any issues when merged with the actual config
	runGoStruct, err := v.deviceModel.NewConfigStruct(runningCfg, true)
	if err != nil {
		return managed.CrSpecObservation{}, err
	}

	specGoStruct, err := v.deviceModel.NewConfigStruct(cr.Spec.Properties.Raw, false)
	if err != nil {
		return managed.CrSpecObservation{}, err
	}

	// convert the spec config into gnmi notification
	notifications, err := ygot.TogNMINotifications(specGoStruct, 0, ygot.GNMINotificationsConfig{UsePathElem: true})
	if err != nil {
		return managed.CrSpecObservation{}, err
	}

	rp := rootpaths.CreateRootConfigElement(v.deviceModel.SchemaTreeRoot)

	// calculate the rootpaths for the deletion
	for _, n := range notifications {
		for _, dp := range n.GetUpdate() {
			// lookup the schema entry for the via path defined node
			pathAndSchema := rootpaths.GetPathAndSchemaEntry(v.deviceModel.SchemaTreeRoot, dp.Path)
			rp.Add(pathAndSchema, dp.Val)
		}
	}

	// collect the results of the rootpath calculation and performa a delete for
	// all of these paths on the actual configuration
	for _, p := range rp.GetRootPaths() {
		err = ytypes.DeleteNode(v.deviceModel.SchemaTreeRoot, runGoStruct, p)
		if err != nil {
			return managed.CrSpecObservation{}, err
		}
	}

	ygot.PruneEmptyBranches(runGoStruct)

	if err := runGoStruct.Validate(&ytypes.LeafrefOptions{IgnoreMissingData: false}); err != nil {
		return managed.CrSpecObservation{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	return managed.CrSpecObservation{
		Success: true,
	}, nil
}

func (v *validatorDevice) GetCrSpecDiff(ctx context.Context, mg resource.Managed, systemCfg *ygotnddp.Device) (managed.CrSpecDiffObservation, error) {
	log := v.log.WithValues("Resource", mg.GetName())
	log.Debug("validate GetCrSpecDiff ...")

	cr, ok := mg.(*srlv1alpha1.SrlConfig)
	if !ok {
		return managed.CrSpecDiffObservation{}, errors.New(errUnexpectedDevice)
	}
	deletes := []*gnmi.Path{}
	updates := []*gnmi.Update{}
	gvkName := gvkresource.GetGvkName(mg)
	if gvk, exists := systemCfg.Gvk[gvkName]; exists {
		srcConfig, err := v.deviceModel.NewConfigStruct(cr.Spec.Properties.Raw, false)
		if err != nil {
			return managed.CrSpecDiffObservation{}, err
		}

		specConfig, err := v.deviceModel.NewConfigStruct([]byte(*gvk.Spec), false)
		if err != nil {
			return managed.CrSpecDiffObservation{}, err
		}

		// create a diff of the actual compared to the to-become-new config
		actualVsSpecDiff, err := ygot.Diff(specConfig, srcConfig, &ygot.DiffPathOpt{MapToSinglePath: true})
		if err != nil {
			return managed.CrSpecDiffObservation{}, err
		}

		deletes, updates = validateNotification(actualVsSpecDiff)

	}
	return managed.CrSpecDiffObservation{
		Deletes: deletes,
		Updates: updates,
	}, nil
}

func (v *validatorDevice) GetCrActualDiff(ctx context.Context, mg resource.Managed, runningCfg []byte) (managed.CrActualDiffObservation, error) {
	log := v.log.WithValues("Resource", mg.GetName())
	log.Debug("validate GetCrActualDiff ...")

	cr, ok := mg.(*srlv1alpha1.SrlConfig)
	if !ok {
		return managed.CrActualDiffObservation{}, errors.New(errUnexpectedDevice)
	}
	srcConfig, err := v.deviceModel.NewConfigStruct(runningCfg, false)
	if err != nil {
		return managed.CrActualDiffObservation{}, err
	}

	specConfig, err := v.deviceModel.NewConfigStruct(cr.Spec.Properties.Raw, false)
	if err != nil {
		return managed.CrActualDiffObservation{}, err
	}

	// skipping specValidation, this will probably result in missing leaf leafrefs
	srcConfigTmp, err := ygot.DeepCopy(srcConfig)
	if err != nil {
		return managed.CrActualDiffObservation{}, err
	}
	newConfig := srcConfigTmp.(*ygotsrl.Device) // Typecast
	// Merge spec into newconfig, which is right now jsut the actual config
	err = ygot.MergeStructInto(newConfig, specConfig)
	if err != nil {
		return managed.CrActualDiffObservation{}, err
	}
	// validate the new config
	//err = newConfig.Validate()
	//if err != nil {
	//	return &observe{hasData: false}, nil
	//}

	// create a diff of the actual compared to the to-become-new config
	actualVsSpecDiff, err := ygot.Diff(srcConfig, newConfig, &ygot.DiffPathOpt{MapToSinglePath: true})
	if err != nil {
		return managed.CrActualDiffObservation{}, err
	}

	deletes, updates := validateNotification(actualVsSpecDiff)

	return managed.CrActualDiffObservation{
		HasData:    true,
		IsUpToDate: len(deletes) == 0 && len(updates) == 0,
		Deletes:    deletes,
		Updates:    updates,
	}, nil

}

func (v *validatorDevice) GetRootPaths(ctx context.Context, mg resource.Managed) ([]string, error) {
	log := v.log.WithValues("Resource", mg.GetName())
	log.Debug("validate GetCrActualDiff ...")

	cr, ok := mg.(*srlv1alpha1.SrlConfig)
	if !ok {
		return nil, errors.New(errUnexpectedDevice)
	}

	srldevice := &ygotsrl.Device{}
	if err := v.deviceModel.JsonUnmarshaler(cr.Spec.Properties.Raw, srldevice); err != nil {
		return nil, err
	}

	gnmiNotifications, err := ygot.TogNMINotifications(srldevice, time.Now().UnixNano(), ygot.GNMINotificationsConfig{UsePathElem: true})
	if err != nil {
		return nil, err
	}

	crRootPaths, err := v.getRootPaths(gnmiNotifications[0])
	if err != nil {
		return nil, err
	}

	rootPaths := []string{}
	for _, crRootPath := range crRootPaths {
		//log.Debug("ValidateRootPaths rootPaths", "path", yparser.GnmiPath2XPath(crRootPath, true))
		rootPaths = append(rootPaths, yparser.GnmiPath2XPath(crRootPath, true))
	}
	return rootPaths, err
}

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connectorDevice struct {
	log         logging.Logger
	kube        client.Client
	usage       resource.Tracker
	deviceModel *model.Model
	systemModel *model.Model
	newClientFn func(c *gnmitypes.TargetConfig) *target.Target
	gnmiAddress string
}

// Connect produces an ExternalClient by:
// 1. Tracking that the managed resource is using a Target.
// 2. Getting the managed resource's Target with connection details
// A resource is mapped to a single target
func (c *connectorDevice) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	log := c.log.WithValues("resource", mg.GetName())
	//log.Debug("Connect")

	cr, ok := mg.(*srlv1alpha1.SrlConfig)
	if !ok {
		return nil, errors.New(errUnexpectedDevice)
	}
	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackTCUsage)
	}

	// find network node that is configured status
	t := &targetv1.Target{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetTargetReference().Name}, t); err != nil {
		return nil, errors.Wrap(err, errGetTarget)
	}

	if t.GetCondition(nddv1.ConditionKindReady).Status != corev1.ConditionTrue {
		return nil, errors.New(targetNotConfigured)
	}

	cfg := &gnmitypes.TargetConfig{
		Name:       cr.GetTargetReference().Name,
		Address:    c.gnmiAddress,
		Username:   utils.StringPtr("admin"),
		Password:   utils.StringPtr("admin"),
		Timeout:    10 * time.Second,
		SkipVerify: utils.BoolPtr(true),
		Insecure:   utils.BoolPtr(true),
		TLSCA:      utils.StringPtr(""), //TODO TLS
		TLSCert:    utils.StringPtr(""), //TODO TLS
		TLSKey:     utils.StringPtr(""),
		Gzip:       utils.BoolPtr(false),
	}

	cl := target.NewTarget(cfg)
	if err := cl.CreateGNMIClient(ctx); err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	tns := []string{t.GetName()}

	//return &externalDevice{client: cl, targets: tns, log: log, deviceSchema: c.deviceSchema, nddpSchema: c.nddpSchema, deviceModel: c.deviceModel, systemModel: c.systemModel}, nil
	return &externalDevice{client: cl, targets: tns, log: log, deviceModel: c.deviceModel, systemModel: c.systemModel}, nil
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type externalDevice struct {
	client      *target.Target
	targets     []string
	log         logging.Logger
	deviceModel *model.Model
	systemModel *model.Model
}

func (e *externalDevice) Close() {
	e.client.Close()
}

func (e *externalDevice) Create(ctx context.Context, mg resource.Managed, obs managed.ExternalObservation) error {
	log := e.log.WithValues("Resource", mg.GetName())
	log.Debug("Creating ...")

	updates, err := e.getGvkUpate(mg, obs, ygotnddp.NddpSystem_ResourceAction_CREATE)
	if err != nil {
		return errors.Wrap(err, errCreateDevice)
	}

	crTarget := cachename.GetNamespacedName(mg.GetNamespace(), mg.GetTargetReference().Name)

	req := &gnmi.SetRequest{
		Prefix:  &gnmi.Path{Target: crTarget, Origin: cachename.SystemCachePrefix},
		Replace: updates,
	}

	_, err = e.client.Set(ctx, req)
	if err != nil {
		return errors.Wrap(err, errCreateDevice)
	}

	return nil
}

func (e *externalDevice) Update(ctx context.Context, mg resource.Managed, obs managed.ExternalObservation) error {
	log := e.log.WithValues("Resource", mg.GetName())
	log.Debug("Updating ...")

	updates, err := e.getGvkUpate(mg, obs, ygotnddp.NddpSystem_ResourceAction_UPDATE)
	if err != nil {
		return errors.Wrap(err, errCreateDevice)
	}

	crTarget := cachename.GetNamespacedName(mg.GetNamespace(), mg.GetTargetReference().Name)

	req := &gnmi.SetRequest{
		Prefix:  &gnmi.Path{Target: crTarget, Origin: cachename.SystemCachePrefix},
		Replace: updates,
	}

	_, err = e.client.Set(ctx, req)
	if err != nil {
		return errors.Wrap(err, errUpdateDevice)
	}

	return nil
}

func (e *externalDevice) Delete(ctx context.Context, mg resource.Managed, obs managed.ExternalObservation) error {
	log := e.log.WithValues("Resource", mg.GetName())
	log.Debug("Deleting ...", "obs", obs)

	updates, err := e.getGvkUpate(mg, obs, ygotnddp.NddpSystem_ResourceAction_DELETE)
	if err != nil {
		return errors.Wrap(err, errCreateDevice)
	}

	crTarget := cachename.GetNamespacedName(mg.GetNamespace(), mg.GetTargetReference().Name)

	req := &gnmi.SetRequest{
		Prefix:  &gnmi.Path{Target: crTarget, Origin: cachename.SystemCachePrefix},
		Replace: updates,
	}

	_, err = e.client.Set(ctx, req)
	if err != nil {
		return errors.Wrap(err, errDeleteDevice)
	}

	return nil
}

func (e *externalDevice) GetSystemConfig(ctx context.Context, mg resource.Managed) (*ygotnddp.Device, error) {
	// get system device list
	crTarget := cachename.GetNamespacedName(mg.GetNamespace(), mg.GetTargetReference().Name)
	// gnmi get request
	reqSystemCache := &gnmi.GetRequest{
		Prefix:   &gnmi.Path{Target: crTarget, Origin: cachename.SystemCachePrefix},
		Path:     []*gnmi.Path{{}},
		Encoding: gnmi.Encoding_JSON,
	}

	// gnmi get response
	resp, err := e.client.Get(ctx, reqSystemCache)
	if err != nil {
		if er, ok := status.FromError(err); ok {
			switch er.Code() {
			case codes.Unavailable:
				// we use this to signal not ready
				return nil, nil
			}
		}
	}
	var systemCache interface{}
	if len(resp.GetNotification()) == 0 {
		return nil, nil
	}
	if len(resp.GetNotification()) != 0 && len(resp.GetNotification()[0].GetUpdate()) != 0 {
		// get value from gnmi get response
		systemCache, err = yparser.GetValue(resp.GetNotification()[0].GetUpdate()[0].Val)
		if err != nil {
			return nil, errors.Wrap(err, errJSONMarshal)
		}

		switch systemCache.(type) {
		case nil:
			// resource has no data
			return nil, nil
		}
	}

	systemData, err := json.Marshal(systemCache)
	if err != nil {
		return nil, err
	}
	goStruct, err := e.systemModel.NewConfigStruct(systemData, true)
	if err != nil {
		return nil, err
	}
	nddpDevice, ok := goStruct.(*ygotnddp.Device)
	if !ok {
		return nil, errors.New("wrong object nddp")
	}

	return nddpDevice, nil
}

func (e *externalDevice) GetRunningConfig(ctx context.Context, mg resource.Managed) ([]byte, error) {
	// get actual device config
	crTarget := cachename.GetNamespacedName(mg.GetNamespace(), mg.GetTargetReference().Name)
	// gnmi get request
	reqRunningConfig := &gnmi.GetRequest{
		Prefix:   &gnmi.Path{Target: crTarget, Origin: cachename.ConfigCachePrefix},
		Path:     []*gnmi.Path{{}},
		Encoding: gnmi.Encoding_JSON,
	}
	// gnmi get response
	resp, err := e.client.Get(ctx, reqRunningConfig)
	if err != nil {
		if er, ok := status.FromError(err); ok {
			switch er.Code() {
			case codes.Unavailable:
				// we use this to signal not ready
				return nil, nil
			}
		}
	}
	var runningConfig interface{}
	if len(resp.GetNotification()) == 0 {
		return nil, nil
	}
	if len(resp.GetNotification()) != 0 && len(resp.GetNotification()[0].GetUpdate()) != 0 {
		// get value from gnmi get response
		runningConfig, err = yparser.GetValue(resp.GetNotification()[0].GetUpdate()[0].Val)
		if err != nil {
			return nil, errors.Wrap(err, errJSONMarshal)
		}

		switch runningConfig.(type) {
		case nil:
			// no actual config
			return nil, nil
		}
	}
	return json.Marshal(runningConfig)
}
