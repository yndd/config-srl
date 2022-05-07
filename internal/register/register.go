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

package register

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-target-runtime/pkg/resource"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultTimout                    = 1 * time.Second
	defaultRegistrationCheckInterval = 5 * time.Second
	defaultMaxServiceFail            = 3
)

// Option can be used to manipulate Register config.
type Option func(Register)

// TargetController defines the interfaces for the target controller
type Register interface {
	//options
	// add a logger to Register
	WithLogger(log logging.Logger)
	// add a k8s client to Register
	WithClient(c resource.ClientApplicator)
	// add a consulNamespace to Register
	WithConsulNamespace(string)
	// add a register kinf to Register
	WithRegisterKind(string)
	// RegisterService
	RegisterService(ctx context.Context)
}

// WithLogger adds a logger to the register
func WithLogger(l logging.Logger) Option {
	return func(o Register) {
		o.WithLogger(l)
	}
}

// WithClient adds a k8s client to the register.
func WithClient(c resource.ClientApplicator) Option {
	return func(o Register) {
		o.WithClient(c)
	}
}

// WithConsulNamespce adds the consul namespace to the register
func WithConsulNamespace(ns string) Option {
	return func(o Register) {
		o.WithConsulNamespace(ns)
	}
}

// WithRegisterKind adds the kind to the register
func WithRegisterKind(n string) Option {
	return func(o Register) {
		o.WithRegisterKind(n)
	}
}

type registerConfig struct {
	kind       string // provider or worker
	namespace  string // namespace in which consul is deployed
	address    string // address of the consul client
	datacenter string // default kind-dc1
	username   string
	password   string
	token      string
}

// registerImpl implements the register interface
type registerImpl struct {
	config *registerConfig
	// kubernetes
	client resource.ClientApplicator
	// consul
	consulClient *api.Client
	// server

	//ctx context.Context
	//cfn context.CancelFunc
	log logging.Logger
}

func New(ctx context.Context, opts ...Option) (Register, error) {
	c := &registerImpl{
		config: &registerConfig{},
	}

	for _, opt := range opts {
		opt(c)
	}

	if err := c.initializeConsul(ctx); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *registerImpl) WithLogger(l logging.Logger) {
	c.log = l
}

func (c *registerImpl) WithClient(rc resource.ClientApplicator) {
	c.client = rc
}

func (c *registerImpl) WithConsulNamespace(ns string) {
	c.config.namespace = ns
}

func (c *registerImpl) WithRegisterKind(n string) {
	c.config.kind = n
}

func (c *registerImpl) initializeConsul(ctx context.Context) error {
	log := c.log.WithValues("Namespace", c.config.namespace)
	log.Debug("consul daemonset not found")

CONSULDAEMONSETPOD:
	// get all the pods in the consul namespace
	opts := []client.ListOption{
		client.InNamespace(c.config.namespace),
	}
	pods := &corev1.PodList{}
	if err := c.client.List(ctx, pods, opts...); err != nil {
		return err
	}

	found := false
	for _, pod := range pods.Items {
		log.Debug("consul pod",
			"consul pod kind", pod.OwnerReferences[0].Kind,
			"consul pod phase", pod.Status.Phase,
			"consul pod node name", pod.Spec.NodeName,
			"consul pod node ip", pod.Status.HostIP,
			"consul pod ip", pod.Status.PodIP,
			"pod node naame", os.Getenv("NODE_NAME"),
			"pod node ip", os.Getenv("Node_IP"),
		)
		if len(pod.OwnerReferences) == 0 {
			// pod has no owner
			continue
		}
		switch pod.OwnerReferences[0].Kind {
		case "DaemonSet":
			if pod.Status.Phase == "Running" &&
				pod.Status.PodIP != "" &&
				pod.Spec.NodeName == os.Getenv("NODE_NAME") {
				//pod.Status.HostIP == os.Getenv("Node_IP") {
				found = true
				c.config.address = strings.Join([]string{pod.Status.PodIP, "8500"}, ":")
				c.config.datacenter = "kind-dc1"
				break
			}
		default:
			// could be ReplicaSet, StatefulSet, etc, but not releant here
			continue
		}
	}
	if !found {
		// daemonset not found
		log.Debug("consul daemonset not found")
		time.Sleep(defaultTimout)
		goto CONSULDAEMONSETPOD
	}
	log.Debug("consul daemonset found", "address", c.config.address, "datacenter", c.config.datacenter)

	return nil
}

func (c *registerImpl) RegisterService(ctx context.Context) {
	go c.registerService(ctx)
}

func (c *registerImpl) registerService(ctx context.Context) error {
	log := c.log.WithValues("address", c.config.address)

	clientConfig := &api.Config{
		Address:    c.config.address,
		Scheme:     "http",
		Datacenter: c.config.datacenter,
		Token:      c.config.token,
	}
	if c.config.username != "" && c.config.password != "" {
		clientConfig.HttpAuth = &api.HttpBasicAuth{
			Username: c.config.username,
			Password: c.config.password,
		}
	}
INITCONSUL:
	var err error
	if c.consulClient, err = api.NewClient(clientConfig); err != nil {
		log.Debug("failed to connect to consul", "error", err)
		time.Sleep(1 * time.Second)
		goto INITCONSUL
	}
	self, err := c.consulClient.Agent().Self()
	if err != nil {
		log.Debug("failed to connect to consul", "error", err)
		time.Sleep(1 * time.Second)
		goto INITCONSUL
	}
	if cfg, ok := self["Config"]; ok {
		b, _ := json.Marshal(cfg)
		log.Debug("consul agent config:", "agent config", string(b))
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	service := &api.AgentServiceRegistration{
		ID:      os.Getenv("POD_NAME"),
		Name:    c.config.kind,
		Address: os.Getenv("POD_IP"),
		Port:    pkgmetav1.GnmiServerPort,
		//Tags:    p.Cfg.ServiceRegistration.Tags,
		Checks: api.AgentServiceChecks{
			{
				TTL:                            defaultRegistrationCheckInterval.String(),
				DeregisterCriticalServiceAfter: (defaultMaxServiceFail * defaultRegistrationCheckInterval).String(),
			},
		},
	}

	ttlCheckID := "service:" + os.Getenv("POD_NAME")

	service.Checks = append(service.Checks, &api.AgentServiceCheck{
		GRPC:                           os.Getenv("POD_IP") + ":" + strconv.Itoa(pkgmetav1.GnmiServerPort),
		GRPCUseTLS:                     true,
		Interval:                       defaultRegistrationCheckInterval.String(),
		TLSSkipVerify:                  true,
		DeregisterCriticalServiceAfter: (defaultMaxServiceFail * defaultRegistrationCheckInterval).String(),
	})
	ttlCheckID = ttlCheckID + ":1"

	b, _ := json.Marshal(service)
	log.Debug("consul register service", "service", string(b))

	if err := c.consulClient.Agent().ServiceRegister(service); err != nil {
		log.Debug("consul register service failed", "error", err)
		return err
	}

	if err := c.consulClient.Agent().UpdateTTL(ttlCheckID, "", api.HealthPassing); err != nil {
		log.Debug("consul failed to pass TTL check", "error", err)
	}
	ticker := time.NewTicker(defaultRegistrationCheckInterval / 2)
	for {
		select {
		case <-ticker.C:
			err = c.consulClient.Agent().UpdateTTL(ttlCheckID, "", api.HealthPassing)
			if err != nil {
				log.Debug("consul failed to pass TTL check", "error", err)
			}
		case <-ctx.Done():
			c.consulClient.Agent().UpdateTTL(ttlCheckID, ctx.Err().Error(), api.HealthCritical)
			ticker.Stop()
			goto INITCONSUL
		}
	}
	//return nil
}
