/*
Copyright 2021 Wim Henderickx.

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

	gapi "github.com/karimra/gnmic/api"
	gutils "github.com/karimra/gnmic/utils"

	gnmictarget "github.com/karimra/gnmic/target"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmi/proto/gnmi_ext"
	"github.com/pkg/errors"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-yang/pkg/yparser"
	targetv1 "github.com/yndd/target/apis/target/v1"
	"github.com/yndd/target/pkg/target"
)

const (
	State         = "STATE"
	Configuration = "CONFIG"
	encoding      = "JSON_IETF"
	//errors
	errGnmiCreateGetRequest = "gnmi create get request error"
	errGnmiGet              = "gnmi get error "
	errGnmiSet              = "gnmi set error "

	//
	swVersionPath = "/platform/control[slot=A]/software-version"
	chassisPath   = "/platform/chassis"
)

/*
func init() {
	target.Register(ygotnddtarget.NddTarget_VendorType_nokia_srl, func() target.Target {
		fmt.Println("init srl in target.Targets")
		return new(srl)
	})
}
*/

func New() target.Target {
	return &srl{}
}

type srl struct {
	target *gnmictarget.Target
	log    logging.Logger
}

func (t *srl) Init(opts ...target.TargetOption) error {
	for _, o := range opts {
		o(t)
	}
	return nil
}

func (t *srl) WithTarget(target *gnmictarget.Target) {
	t.target = target
}

func (t *srl) WithLogging(log logging.Logger) {
	t.log = log
}

func (t *srl) GNMICap(ctx context.Context) (*gnmi.CapabilityResponse, error) {
	t.log.Debug("verifying capabilities ...")

	ext := new(gnmi_ext.Extension)
	return t.target.Capabilities(ctx, ext)
}

func (t *srl) Discover(ctx context.Context) (*targetv1.DiscoveryInfo, error) {
	t.log.Debug("Discover SRL details ...")
	discoveryInfo, err := t.getDiscoveryInfo(ctx)
	if err != nil {
		return nil, err
	}
	t.log.Debug("SRL %s discovered: %+v", t.target.Config.Name, *discoveryInfo)
	return discoveryInfo, nil
}

func (t *srl) GetConfig(ctx context.Context) (interface{}, error) {
	var err error
	var req *gnmi.GetRequest
	var rsp *gnmi.GetResponse

	req, err = gapi.NewGetRequest(
		gapi.Path(""),
		gapi.DataType("config"),
		gapi.Encoding("json_ietf"),
	)
	if err != nil {
		t.log.Debug(errGnmiCreateGetRequest, "error", err)
		return nil, errors.Wrap(err, errGnmiCreateGetRequest)
	}
	rsp, err = t.target.Get(ctx, req)
	if err != nil {
		t.log.Debug(errGnmiGet, "error", err)
		return nil, errors.Wrap(err, errGnmiGet)
	}
	//
	// expects a GetResponse with a single notification which
	// in turn has a single update.
	ns := rsp.GetNotification()
	switch len(ns) {
	case 1:
		upds := ns[0].GetUpdate()
		switch len(upds) {
		case 1:
			return yparser.GetValue(upds[0].GetVal())
		default:
			return nil, errors.New("unexpected number of updates in GetResponse Notification")
		}
	default:
		return nil, errors.New("unexpected number of Notifications in GetResponse")
	}
}

func (t *srl) GNMIGet(ctx context.Context, opts ...gapi.GNMIOption) (*gnmi.GetResponse, error) {
	var err error
	var req *gnmi.GetRequest
	var rsp *gnmi.GetResponse

	req, err = gapi.NewGetRequest(opts...)
	if err != nil {
		t.log.Debug(errGnmiCreateGetRequest, "error", err)
		return nil, errors.Wrap(err, errGnmiCreateGetRequest)
	}
	rsp, err = t.target.Get(ctx, req)
	if err != nil {
		t.log.Debug(errGnmiGet, "error", err)
		return nil, errors.Wrap(err, errGnmiGet)
	}
	return rsp, nil
}

func (t *srl) GNMISet(ctx context.Context, u []*gnmi.Update, p []*gnmi.Path) (*gnmi.SetResponse, error) {
	resp, err := t.target.Set(ctx, &gnmi.SetRequest{
		Update: u,
		Delete: p,
	})
	if err != nil {
		t.log.Debug(errGnmiSet, "error", err)
		return nil, err
	}
	//d.log.Debug("set response:", "resp", resp)
	return resp, nil
}

func (t *srl) getDiscoveryInfo(ctx context.Context) (*targetv1.DiscoveryInfo, error) {
	req, err := gapi.NewGetRequest(
		gapi.Path(swVersionPath),
		gapi.Path(chassisPath),
		gapi.Encoding("ascii"),
		gapi.DataType("state"),
	)
	if err != nil {
		return nil, err
	}
	resp, err := t.target.Get(ctx, req)
	if err != nil {
		return nil, err
	}
	devDetails := &targetv1.DiscoveryInfo{
		VendorType: targetv1.VendorTypeNokiaSRL,
	}
	for _, notif := range resp.GetNotification() {
		for _, upd := range notif.GetUpdate() {
			p := gutils.GnmiPathToXPath(upd.GetPath(), true)
			switch p {
			case "platform/control/software-version":
				if devDetails.SwVersion == "" {
					devDetails.SwVersion = upd.GetVal().GetStringVal()
				}
			case "platform/chassis/type":
				devDetails.Platform = upd.GetVal().GetStringVal()
			case "platform/chassis/serial-number":
				devDetails.SerialNumber = upd.GetVal().GetStringVal()
			case "platform/chassis/hw-mac-address":
				devDetails.MacAddress = upd.GetVal().GetStringVal()
			}
		}
	}
	return devDetails, nil
}
