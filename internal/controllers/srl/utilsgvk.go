package srl

import (
	"encoding/json"

	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	srlv1alpha1 "github.com/yndd/config-srl/apis/srl/v1alpha1"
	"github.com/yndd/ndd-runtime/pkg/reconciler/managed"
	"github.com/yndd/ndd-runtime/pkg/resource"
	"github.com/yndd/ndd-yang/pkg/yparser"
	"github.com/yndd/nddp-system/pkg/gvkresource"
	"github.com/yndd/nddp-system/pkg/ygotnddp"
)

// getGvkUpate returns an update to the system cache using the k8s api naming convetion and the nddp system
// gvk -> group, version, kind, namespace, name
func (e *externalDevice) getGvkUpate(mg resource.Managed, obs managed.ExternalObservation, action ygotnddp.E_NddpSystem_ResourceAction) ([]*gnmi.Update, error) {
	e.log.Debug("getGvkUpate")

	// get gvk Name
	gvkName := gvkresource.GetGvkName(mg)

	// get spec in string format
	spec, err := getSpec(mg)
	if err != nil {
		return nil, err
	}

	updates := map[string]*ygotnddp.NddpSystem_Gvk_Update{}
	if obs.Updates != nil {
		if updates, err = getUpdates(obs.Updates); err != nil {
			return nil, err
		}
	}

	deletes := []string{}
	if obs.Deletes != nil {
		deletes = getPaths(obs.Deletes)
	}

	// get nddpData from gvkname, action, paths and spec
	gvkData := &ygotnddp.NddpSystem_Gvk{
		Name:    ygot.String(gvkName),
		Action:  action,
		Path:    mg.GetRootPaths(),
		Status:  ygotnddp.NddpSystem_ResourceStatus_PENDING,
		Reason:  ygot.String(""),
		Spec:    spec,
		Delete:  deletes,
		Update:  updates,
		Attempt: ygot.Uint32(0),
	}

	nddpJson, err := ygot.EmitJSON(gvkData, &ygot.EmitJSONConfig{
		Format: ygot.RFC7951,
	})
	if err != nil {
		return nil, err
	}

	//return update
	return []*gnmi.Update{
		{
			Path: &gnmi.Path{
				Elem: []*gnmi.PathElem{
					{Name: "gvk", Key: map[string]string{"name": gvkName}},
				},
			},
			Val: &gnmi.TypedValue{Value: &gnmi.TypedValue_JsonVal{JsonVal: []byte(nddpJson)}},
		},
	}, nil
}

// getSpec return the spec as a string
func getSpec(mg resource.Managed) (*string, error) {
	cr, ok := mg.(*srlv1alpha1.SrlConfig)
	if !ok {
		return nil, errors.New(errUnexpectedDevice)
	}
	spec, err := json.Marshal(cr.Spec.Properties)
	if err != nil {
		return nil, errors.Wrap(err, errJSONMarshal)
	}
	return ygot.String(string(spec)), nil
}

// getPaths returns a slice of string from the gnmi path slice
func getPaths(gnmiPaths []*gnmi.Path) []string {
	paths := []string{}
	for _, p := range gnmiPaths {
		paths = append(paths, yparser.GnmiPath2XPath(p, true))
	}
	return paths
}

// getPaths returns a map of updates with the rootPath as key
func getUpdates(gnmiUpdates []*gnmi.Update) (map[string]*ygotnddp.NddpSystem_Gvk_Update, error) {
	updates := map[string]*ygotnddp.NddpSystem_Gvk_Update{}
	for _, u := range gnmiUpdates {
		xpath := yparser.GnmiPath2XPath(u.GetPath(), true)
		v, err := yparser.GetValue(u.GetVal())
		if err != nil {
			return nil, err
		}
		val, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		updates[xpath] = &ygotnddp.NddpSystem_Gvk_Update{
			Path: ygot.String(xpath),
			Val:  ygot.String(string(val)),
		}
	}
	return updates, nil
}
