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
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"

	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-target-runtime/pkg/resource"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultDCName     = "kind-dc1"
	defaultConsulPort = "8500"
)

type consulConfig struct {
	namespace  string // namespace in which consul is deployed
	address    string // address of the consul client
	datacenter string // default kind-dc1
	username   string
	password   string
	token      string
}

// consul implements the Registrator interface
type consul struct {
	serviceConfig *serviceConfig
	consulConfig  *consulConfig
	// kubernetes
	client resource.ClientApplicator
	// consul
	consulClient *api.Client
	stopCh chan struct{} // used to stop the registration
	log logging.Logger
}

func NewConsulRegistrator(ctx context.Context, namespace, dcName string, opts ...Option) (Registrator, error) {

	// if the namespace is not provided we initialize to consul namespace
	if namespace == "" {
		namespace = "consul"
	}

	r := &consul{
		serviceConfig: &serviceConfig{},
		consulConfig: &consulConfig{
			namespace:  namespace,
			datacenter: dcName,
		},
		stopCh: make(chan struct{}),
	}

	for _, opt := range opts {
		opt(r)
	}

	if err := r.init(ctx); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *consul) WithLogger(l logging.Logger) {
	r.log = l
}

func (r *consul) WithClient(rc resource.ClientApplicator) {
	r.client = rc
}

func (r *consul) WithServiceInfo(name, id, ip string, port int) {
	r.serviceConfig.name = name
	r.serviceConfig.id = id
	r.serviceConfig.ip = ip
	r.serviceConfig.port = port
}

func (r *consul) init(ctx context.Context) error {
	log := r.log.WithValues("Service", r.serviceConfig, "Namespace", r.consulConfig.namespace)
	log.Debug("consul init, trying to find daemonset...")

CONSULDAEMONSETPOD:
	// get all the pods in the consul namespace
	opts := []client.ListOption{
		client.InNamespace(r.consulConfig.namespace),
	}
	pods := &corev1.PodList{}
	if err := r.client.List(ctx, pods, opts...); err != nil {
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
				r.consulConfig.address = strings.Join([]string{pod.Status.PodIP, defaultConsulPort}, ":")
				r.consulConfig.datacenter = defaultDCName
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
	log.Debug("consul daemonset found", "address", r.consulConfig.address, "datacenter", r.consulConfig.datacenter)

	return nil
}

func (r *consul) Register(ctx context.Context) {
	go r.registerService(ctx)
}

func (r *consul) DeRegister(ctx context.Context) {
	log := r.log.WithValues("Service", r.serviceConfig, "Namespace", r.consulConfig.namespace)
	log.Debug("Deregister...")

	close(r.stopCh)
}

func (r *consul) registerService(ctx context.Context) error {
	log := r.log.WithValues("Service", r.serviceConfig, "Consul", r.consulConfig)
	log.Debug("Register...")

	clientConfig := &api.Config{
		Address:    r.consulConfig.address,
		Scheme:     "http",
		Datacenter: r.consulConfig.datacenter,
		Token:      r.consulConfig.token,
	}
	if r.consulConfig.username != "" && r.consulConfig.password != "" {
		clientConfig.HttpAuth = &api.HttpBasicAuth{
			Username: r.consulConfig.username,
			Password: r.consulConfig.password,
		}
	}
INITCONSUL:
	var err error
	if r.consulClient, err = api.NewClient(clientConfig); err != nil {
		log.Debug("failed to connect to consul", "error", err)
		time.Sleep(1 * time.Second)
		goto INITCONSUL
	}
	self, err := r.consulClient.Agent().Self()
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
		ID:      r.serviceConfig.id,
		Name:    r.serviceConfig.name,
		Address: r.serviceConfig.ip,
		Port:    r.serviceConfig.port,
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
		GRPC:                           r.serviceConfig.ip + ":" + strconv.Itoa(r.serviceConfig.port),
		GRPCUseTLS:                     true,
		Interval:                       defaultRegistrationCheckInterval.String(),
		TLSSkipVerify:                  true,
		DeregisterCriticalServiceAfter: (defaultMaxServiceFail * defaultRegistrationCheckInterval).String(),
	})
	ttlCheckID = ttlCheckID + ":1"

	b, _ := json.Marshal(service)
	log.Debug("consul register service", "service", string(b))

	if err := r.consulClient.Agent().ServiceRegister(service); err != nil {
		log.Debug("consul register service failed", "error", err)
		return err
	}

	if err := r.consulClient.Agent().UpdateTTL(ttlCheckID, "", api.HealthPassing); err != nil {
		log.Debug("consul failed to pass TTL check", "error", err)
	}
	ticker := time.NewTicker(defaultRegistrationCheckInterval / 2)
	for {
		select {
		case <-ticker.C:
			err = r.consulClient.Agent().UpdateTTL(ttlCheckID, "", api.HealthPassing)
			if err != nil {
				log.Debug("consul failed to pass TTL check", "error", err)
			}
		case <-ctx.Done():
			r.consulClient.Agent().UpdateTTL(ttlCheckID, ctx.Err().Error(), api.HealthCritical)
			ticker.Stop()
			goto INITCONSUL
		case <-r.stopCh:
			r.log.Debug("deregister...")
			r.consulClient.Agent().ServiceDeregister(os.Getenv("POD_NAME"))
			ticker.Stop()
			return nil
		}
	}
}