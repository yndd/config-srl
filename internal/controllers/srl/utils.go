package srl

import (
	"fmt"

	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/yndd/ndd-yang/pkg/yparser"
	"github.com/yndd/target/pkg/rootpaths"
)

/*
type observe struct {
	hasData  bool
	upToDate bool
	deletes  []*gnmi.Path
	updates  []*gnmi.Update
}
*/

func (v *validatorDevice) getRootPaths(x *gnmi.Notification) ([]*gnmi.Path, error) {
	schema := v.deviceModel.SchemaTreeRoot
	rootConfigElement := rootpaths.ConfigElementHierarchyFromGnmiUpdate(schema, x)
	result := rootConfigElement.GetRootPaths()

	return result, nil
}

/*
// 1. validate the repsonse to check if it contains the right # elements, data
func (e *externalDevice) processObserve(runningCfg, specCfg []byte) (*observe, error) {
	deletes, updates, err := e.goStructDiff(runningCfg, specCfg)
	if err != nil {
		return &observe{hasData: false}, err
	}
	return &observe{
		hasData:  true,
		upToDate: len(deletes) == 0 && len(updates) == 0,
		deletes:  deletes,
		updates:  updates,
	}, nil
}
*/

/*
func (e *externalDevice) goStructDiff(runningCfg, specCfg []byte) ([]*gnmi.Path, []*gnmi.Update, error) {
	srcConfig, err := e.deviceModel.NewConfigStruct(runningCfg, false)
	if err != nil {
		return nil, nil, err
	}

	specConfig, err := e.deviceModel.NewConfigStruct(specCfg, false)
	if err != nil {
		return nil, nil, err
	}

	// skipping specValidation, this will probably result in missing leaf leafrefs
	srcConfigTmp, err := ygot.DeepCopy(srcConfig)
	if err != nil {
		return nil, nil, err
	}
	newConfig := srcConfigTmp.(*ygotsrl.Device) // Typecast

	// Merge spec into newconfig, which is right now jsut the actual config
	err = ygot.MergeStructInto(newConfig, specConfig)
	if err != nil {
		return nil, nil, err
	}

	// validate the new config
	//err = newConfig.Validate()
	//if err != nil {
	//	return &observe{hasData: false}, nil
	//}

	// create a diff of the actual compared to the to-become-new config
	actualVsSpecDiff, err := ygot.Diff(srcConfig, newConfig, &ygot.DiffPathOpt{MapToSinglePath: true})
	if err != nil {
		return nil, nil, err
	}

	deletes, updates := validateNotification(actualVsSpecDiff)

	return deletes, updates, nil
}
*/

func validateNotification(n *gnmi.Notification) ([]*gnmi.Path, []*gnmi.Update) {
	updates := make([]*gnmi.Update, 0)
	for _, u := range n.GetUpdate() {
		fmt.Printf("validateNotification diff update old path: %s, value: %v\n", yparser.GnmiPath2XPath(u.GetPath(), true), u.GetVal())
		// workaround since the diff can return double pathElem
		var changed bool
		changed, u.Path = validatePath(u.GetPath())
		if changed {
			u.Val = &gnmi.TypedValue{Value: &gnmi.TypedValue_JsonVal{JsonVal: []byte("{}")}}
		}
		fmt.Printf("validateNotification diff update new path: %s, value: %v\n", yparser.GnmiPath2XPath(u.GetPath(), true), u.GetVal())
		updates = append(updates, u)
	}

	deletes := make([]*gnmi.Path, 0)
	for _, p := range n.GetDelete() {
		fmt.Printf("validateNotification diff delete old path: %s\n", yparser.GnmiPath2XPath(p, true))
		// workaround since the diff can return double pathElem
		_, p = validatePath(p)
		fmt.Printf("validateNotification diff delete new path: %s\n", yparser.GnmiPath2XPath(p, true))
		deletes = append(deletes, p)
	}
	return deletes, updates
}

// workaround for the diff handling
func validatePath(p *gnmi.Path) (bool, *gnmi.Path) {
	if len(p.GetElem()) <= 1 {
		return false, p
	}
	// when the 2nd last pathElem has a key and the last PathElem is an entry in the Key we should trim the last entry from the path
	// e.g. /interface[name=ethernet-1/49]/subinterface[index=1]/ipv4/address[ip-prefix=100.64.0.0/31]/ip-prefix, value: string_val:"100.64.0.0/31"
	// e.g. /interface[name=ethernet-1/49]/subinterface[index=1]/ipv4/address[ip-prefix=100.64.0.0/31]/ip-prefix, value: string_val:"100.64.0.0/31"
	if len(p.GetElem()[len(p.GetElem())-2].GetKey()) > 0 {
		if _, ok := p.GetElem()[len(p.GetElem())-2].GetKey()[p.GetElem()[len(p.GetElem())-1].GetName()]; ok {
			p.Elem = p.Elem[:len(p.GetElem())-1]
			return true, p
		}
	}
	return false, p
}
