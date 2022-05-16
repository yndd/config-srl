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

package reconciler

import (
	"os"
	"strconv"
	"strings"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/pkg/errors"
	"github.com/pkg/profile"
	"github.com/spf13/cobra"

	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	srlv1alpha1 "github.com/yndd/ndd-config-srl/apis/srl/v1alpha1"
	"github.com/yndd/ndd-config-srl/internal/controllers/srl"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-runtime/pkg/ratelimiter"
	"github.com/yndd/ndd-target-runtime/pkg/shared"
	"github.com/yndd/reconciler-controller/pkg/reconcilercontroller"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	//+kubebuilder:scaffold:imports
)

var (
	metricsAddr               string
	probeAddr                 string
	enableLeaderElection      bool
	concurrency               int
	pollInterval              time.Duration
	namespace                 string
	podname                   string
	grpcServerAddress         string
	grpcQueryAddress          string
	autoPilot                 bool
	serviceDiscovery          string
	serviceDiscoveryNamespace string // todo initialization
	controllerConfigName      string
)

// startCmd represents the start command for the network device driver
var startCmd = &cobra.Command{
	Use:          "start",
	Short:        "start srl config reconciler",
	Long:         "start srl config reconciler",
	Aliases:      []string{"start"},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		zlog := zap.New(zap.UseDevMode(debug), zap.JSONEncoder())
		if debug {
			// Only use a logr.Logger when debug is on
			ctrl.SetLogger(zlog)
		}
		logger := logging.NewLogrLogger(zlog.WithName("reconciler"))

		if profiler {
			defer profile.Start().Stop()
			go func() {
				http.ListenAndServe(":8000", nil)
			}()
		}

		rc, err := reconcilercontroller.New(cmd.Context(), ctrl.GetConfigOrDie(), &reconcilercontroller.Options{
			Logger:                    logger,
			Scheme:                    scheme,
			GrpcBindAddress:           strconv.Itoa(pkgmetav1.GnmiServerPort),
			ServiceDiscovery:          pkgmetav1.ServiceDiscoveryType(serviceDiscovery),
			ServiceDiscoveryNamespace: serviceDiscoveryNamespace,
			ControllerConfigName:      controllerConfigName,
		})
		if err != nil {
			return errors.Wrap(err, "Cannot create reconciler controller")
		}
		if err := rc.Start(); err != nil {
			return errors.Wrap(err, "Cannot start reconciler controller")
		}

		zlog.Info("create manager")
		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:             scheme,
			MetricsBindAddress: metricsAddr,
			WebhookServer: &webhook.Server{
				Port: 9443,
			},
			Port:                   7443,
			HealthProbeBindAddress: probeAddr,
			LeaderElection:         enableLeaderElection,
			LeaderElectionID:       "c66ce353.ndd.yndd.io",
		})
		if err != nil {
			return errors.Wrap(err, "Cannot add srlconfig manager")
		}

		// assign gnmi address
		var gnmiAddress string
		if grpcQueryAddress != "" {
			gnmiAddress = grpcQueryAddress
		} else {
			gnmiAddress = getGnmiServerAddress(podname)
		}
		zlog.Info("gnmi address", "address", gnmiAddress)

		// initialize controllers
		_, _, err = srl.Setup(mgr, &shared.NddControllerOptions{
			Logger:      logging.NewLogrLogger(zlog.WithName("srl")),
			Poll:        pollInterval,
			Namespace:   namespace,
			GnmiAddress: gnmiAddress,
		})
		if err != nil {
			return errors.Wrap(err, "Cannot add ndd controllers to manager")
		}

		if err = (&srlv1alpha1.SrlConfig{}).SetupWebhookWithManager(mgr); err != nil {
			return errors.Wrap(err, "unable to create webhook for srl3device")
		}

		// +kubebuilder:scaffold:builder

		if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
			return errors.Wrap(err, "unable to set up health check")
		}
		if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
			return errors.Wrap(err, "unable to set up ready check")
		}

		zlog.Info("starting manager")
		if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			return errors.Wrap(err, "problem running manager")
		}

		// TODO setup event channel for config changes

		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().StringVarP(&metricsAddr, "metrics-bind-address", "m", ":8080", "The address the metric endpoint binds to.")
	startCmd.Flags().StringVarP(&probeAddr, "health-probe-bind-address", "p", ":8081", "The address the probe endpoint binds to.")
	startCmd.Flags().BoolVarP(&enableLeaderElection, "leader-elect", "l", false, "Enable leader election for controller manager. "+
		"Enabling this will ensure there is only one active controller manager.")
	startCmd.Flags().IntVarP(&concurrency, "concurrency", "", 1, "Number of items to process simultaneously")
	startCmd.Flags().DurationVarP(&pollInterval, "poll-interval", "", 10*time.Minute, "Poll interval controls how often an individual resource should be checked for drift.")
	startCmd.Flags().StringVarP(&namespace, "namespace", "n", os.Getenv("POD_NAMESPACE"), "Namespace used to unpack and run packages.")
	startCmd.Flags().StringVarP(&podname, "podname", "", os.Getenv("POD_NAME"), "Name from the pod")
	startCmd.Flags().StringVarP(&grpcServerAddress, "grpc-server-address", "s", "", "The address of the grpc server binds to.")
	startCmd.Flags().StringVarP(&grpcQueryAddress, "grpc-query-address", "", "", "Validation query address.")
	startCmd.Flags().BoolVarP(&autoPilot, "autopilot", "a", true,
		"Apply delta/diff changes to the config automatically when set to true, if set to false the provider will report the delta and the operator should intervene what to do with the delta/diffs")
	startCmd.Flags().StringVarP(&serviceDiscovery, "service-discovery", "", "consul", "the service discovery kind used in this deployment")
	startCmd.Flags().StringVarP(&serviceDiscoveryNamespace, "service-discovery-namespace", "", "consul", "the namespace for service discovery")
	startCmd.Flags().StringVarP(&controllerConfigName, "controller-config-name", "", "", "The name of the controller configuration")
}

func nddCtlrOptions(c int) controller.Options {
	return controller.Options{
		MaxConcurrentReconciles: c,
		RateLimiter:             ratelimiter.NewDefaultProviderRateLimiter(ratelimiter.DefaultProviderRPS),
	}
}

func getGnmiServerAddress(podname string) string {
	//revision := strings.Split(podname, "-")[len(strings.Split(podname, "-"))-3]
	var newName string
	for i, s := range strings.Split(podname, "-") {
		if i == 0 {
			newName = s
		} else if i <= (len(strings.Split(podname, "-")) - 3) {
			newName += "-" + s
		}
	}
	return pkgmetav1.PrefixGnmiService + "-" + newName + "." + pkgmetav1.NamespaceLocalK8sDNS + strconv.Itoa((pkgmetav1.GnmiServerPort))
}

func getClient(scheme *runtime.Scheme) (client.Client, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}

	return client.New(cfg, client.Options{Scheme: scheme})
}