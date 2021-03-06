/*
Copyright 2021 Ndd.

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
	"context"

	"github.com/yndd/app-runtime/pkg/app"
	srlv1alpha1 "github.com/yndd/config-srl/apis/srl/v1alpha1"
	"github.com/yndd/ndd-runtime/pkg/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func InitConfig(c resource.ClientApplicator) app.Object {
	return &config{
		client: c,
	}
}

type config struct {
	// k8s client
	client resource.ClientApplicator
}

func (x *config) List(ctx context.Context, opts []client.ListOption) (resource.ManagedList, error) {
	ol := &srlv1alpha1.SrlConfigList{}
	if err := x.client.List(ctx, ol, opts...); err != nil {
		return nil, err
	}
	return ol, nil
}
