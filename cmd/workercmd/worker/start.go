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

package worker

import (
	"os"
	"reflect"
	"strconv"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/pkg/errors"
	"github.com/pkg/profile"
	"github.com/spf13/cobra"
	"github.com/yndd/ndd-runtime/pkg/model"
	"github.com/yndd/registrator/registrator"

	itarget "github.com/yndd/ndd-config-srl/internal/controllers/target"
	"github.com/yndd/ndd-config-srl/internal/target/srl"
	"github.com/yndd/ndd-config-srl/pkg/ygotsrl"
	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-runtime/pkg/ratelimiter"
	"github.com/yndd/ndd-target-runtime/pkg/shared"
	"github.com/yndd/ndd-target-runtime/pkg/target"
	"github.com/yndd/ndd-target-runtime/pkg/targetcontroller"
	"github.com/yndd/ndd-target-runtime/pkg/ygotnddtarget"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
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
	serviceDiscoveryDcName    string
	serviceDiscovery          string
	serviceDiscoveryNamespace string // todo initialization
)

// startCmd represents the start command for the network device driver
var startCmd = &cobra.Command{
	Use:          "start",
	Short:        "start srl config worker",
	Long:         "start srl config worker",
	Aliases:      []string{"start"},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		zlog := zap.New(zap.UseDevMode(debug), zap.JSONEncoder())
		if debug {
			// Only use a logr.Logger when debug is on
			ctrl.SetLogger(zlog)
		}
		logger := logging.NewLogrLogger(zlog.WithName("worker"))

		if profiler {
			defer profile.Start().Stop()
			go func() {
				http.ListenAndServe(":8000", nil)
			}()
		}

		// create a service discovery registrator
		reg, err := registrator.New(cmd.Context(), ctrl.GetConfigOrDie(), &registrator.Options{
			Logger:                    logger,
			Scheme:                    scheme,
			ServiceDiscoveryDcName:    serviceDiscoveryDcName,
			ServiceDiscovery:          pkgmetav1.ServiceDiscoveryType(serviceDiscovery),
			ServiceDiscoveryNamespace: serviceDiscoveryNamespace,
		})
		if err != nil {
			return errors.Wrap(err, "Cannot create registrator")
		}

		// initialize the target registry and register the vendor type
		tr := target.NewTargetRegistry()
		tr.RegisterInitializer(ygotnddtarget.NddTarget_VendorType_nokia_srl, func() target.Target {
			return srl.New()
		})
		// inittialize the target controller
		tc, err := targetcontroller.New(cmd.Context(), ctrl.GetConfigOrDie(), &targetcontroller.Options{
			Logger:            logger,
			Registrator:       reg,
			GrpcServerAddress: ":" + strconv.Itoa(pkgmetav1.GnmiServerPort),
			TargetRegistry:    tr,
			TargetModel: &model.Model{
				StructRootType:  reflect.TypeOf((*ygotsrl.Device)(nil)),
				SchemaTreeRoot:  ygotsrl.SchemaTree["Device"],
				JsonUnmarshaler: ygotsrl.Unmarshal,
				EnumData:        ygotsrl.ΛEnum,
			},
		})
		if err != nil {
			return errors.Wrap(err, "Cannot create target controller")
		}
		if err := tc.Start(); err != nil {
			return errors.Wrap(err, "Cannot start target controller")
		}

		// +kubebuilder:scaffold:builder

		zlog.Info("create manager")
		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:                 scheme,
			MetricsBindAddress:     metricsAddr,
			Port:                   7443,
			HealthProbeBindAddress: probeAddr,
			LeaderElection:         enableLeaderElection,
			LeaderElectionID:       "c66ce353.ndd.yndd.io",
		})
		if err != nil {
			return errors.Wrap(err, "Cannot create manager")
		}

		// initialize controllers
		if err = itarget.Setup(mgr, &shared.NddControllerOptions{
			Logger:    logger,
			Poll:      pollInterval,
			Namespace: namespace,
			Copts: controller.Options{
				MaxConcurrentReconciles: concurrency,
				RateLimiter:             ratelimiter.NewDefaultProviderRateLimiter(ratelimiter.DefaultProviderRPS),
			},
		}); err != nil {
			return errors.Wrap(err, "Cannot add target to manager")
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

		// TODO create channel for creating/deleting targets

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
	startCmd.Flags().StringVarP(&serviceDiscovery, "service-discovery", "", os.Getenv("SERVICE_DISCOVERY"), "the service discovery kind used in this deployment")
	startCmd.Flags().StringVarP(&serviceDiscoveryNamespace, "service-discovery-namespace", "", os.Getenv("SERVICE_DISCOVERY_NAMESPACE"), "the namespace used for service discovery")
	startCmd.Flags().StringVarP(&serviceDiscoveryDcName, "service-discovery-dc-name", "", os.Getenv("SERVICE_DISCOVERY_DCNAME"), "The dc name used in service discovery")
}
