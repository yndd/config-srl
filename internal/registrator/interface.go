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

package registrator

import (
	"context"
	"time"

	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-target-runtime/pkg/resource"
)

const (
	defaultTimout                    = 1 * time.Second
	defaultRegistrationCheckInterval = 5 * time.Second
	defaultMaxServiceFail            = 3
)

// Option can be used to manipulate Register config.
type Option func(Registrator)

// TargetController defines the interfaces for the target controller
type Registrator interface {
	//options
	// add a logger to the Registrator
	WithLogger(log logging.Logger)
	// add a k8s client to the Registrator
	WithClient(c resource.ClientApplicator)
	// add a serviceInfo to the Registrator
	WithServiceInfo(name, ip string, port int)
	// Register
	Register(ctx context.Context)
	// DeRegister
	DeRegister(ctx context.Context)
}

// WithLogger adds a logger to the Registrator
func WithLogger(l logging.Logger) Option {
	return func(o Registrator) {
		o.WithLogger(l)
	}
}

// WithClient adds a k8s client to the Registrator.
func WithClient(c resource.ClientApplicator) Option {
	return func(o Registrator) {
		o.WithClient(c)
	}
}

// WithServiceConfig adds the service configuration to the Registrator
func WithServiceConfig(name, ip string, port int) Option {
	return func(o Registrator) {
		o.WithServiceInfo(name, ip, port)
	}
}

type serviceConfig struct {
	name string // service name e.g. provider or worker
	port int    // service port
	ip   string // service IP
}
