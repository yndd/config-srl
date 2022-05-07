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
	"strings"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/pkg/errors"
	"github.com/pkg/profile"
	"github.com/spf13/cobra"
	"github.com/yndd/ndd-runtime/pkg/model"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/yndd/ndd-config-srl/internal/register"
	"github.com/yndd/ndd-config-srl/internal/target/srl"
	"github.com/yndd/ndd-config-srl/pkg/ygotsrl"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-runtime/pkg/ratelimiter"
	"github.com/yndd/ndd-target-runtime/pkg/resource"
	"github.com/yndd/ndd-target-runtime/pkg/target"
	"github.com/yndd/ndd-target-runtime/pkg/targetcontroller"
	"github.com/yndd/ndd-target-runtime/pkg/ygotnddtarget"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	//+kubebuilder:scaffold:imports
)

var (
	metricsAddr          string
	probeAddr            string
	enableLeaderElection bool
	concurrency          int
	pollInterval         time.Duration
	namespace            string
	podname              string
	grpcServerAddress    string
	grpcQueryAddress     string
	autoPilot            bool
	controllerName       string
	deploymentKind       string // integrated or distributed
	consulNamespace      string
)

type DeploymentKind string

const (
	DeploymentKindIntegrated  DeploymentKind = "integrated"
	DeploymentKindDistributed DeploymentKind = "distributed"
)

// startCmd represents the start command for the network device driver
var startCmd = &cobra.Command{
	Use:          "start",
	Short:        "start the srl ndd config proxy",
	Long:         "start the srl ndd config proxy",
	Aliases:      []string{"start"},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		zlog := zap.New(zap.UseDevMode(debug), zap.JSONEncoder())
		if debug {
			// Only use a logr.Logger when debug is on
			ctrl.SetLogger(zlog)
		}

		if profiler {
			defer profile.Start().Stop()
			go func() {
				http.ListenAndServe(":8000", nil)
			}()
		}

		// get k8s client
		client, err := getClient(scheme)
		if err != nil {
			return err
		}

		// register the worker as a aservice to consul
		if deploymentKind == string(DeploymentKindDistributed) {
			reg, err := register.New(cmd.Context(),
				register.WithClient(resource.ClientApplicator{
					Client:     client,
					Applicator: resource.NewAPIPatchingApplicator(client),
				}),
				register.WithRegisterKind(strings.Join([]string{controllerName, "worker"}, "-")),
				register.WithConsulNamespace(consulNamespace),
				register.WithLogger(logging.NewLogrLogger(zlog.WithName("consul register"))),
			)
			if err != nil {
				return errors.Wrap(err, "Cannot start consul register")
			}
			reg.RegisterService(cmd.Context())
		}

		// initialize the target registry and register the vendor type
		tr := target.NewTargetRegistry()
		tr.RegisterInitializer(ygotnddtarget.NddTarget_VendorType_nokia_srl, func() target.Target {
			return srl.New()
		})
		// intialize the devicedriver
		d := targetcontroller.New(
			cmd.Context(),
			targetcontroller.WithClient(resource.ClientApplicator{
				Client:     client,
				Applicator: resource.NewAPIPatchingApplicator(client),
			}),
			targetcontroller.WithLogger(logging.NewLogrLogger(zlog.WithName("target driver"))),
			//targetcontroller.WithEventCh(eventChs),
			targetcontroller.WithTargetsRegistry(tr),
			targetcontroller.WithTargetModel(&model.Model{
				StructRootType:  reflect.TypeOf((*ygotsrl.Device)(nil)),
				SchemaTreeRoot:  ygotsrl.SchemaTree["Device"],
				JsonUnmarshaler: ygotsrl.Unmarshal,
				EnumData:        ygotsrl.ΛEnum,
			}),
		)
		if err := d.Start(); err != nil {
			return errors.Wrap(err, "Cannot start device driver")
		}

		// +kubebuilder:scaffold:builder

		// TODO Health check
		for {
		}

		//return nil
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
	startCmd.Flags().StringVarP(&controllerName, "controller-name", "", "", "controller-name identifies the name of the controller")
	startCmd.Flags().StringVarP(&deploymentKind, "deployment-kind", "", "distributed", "DeploymentKind identifies whether this is an integarted and distributed deployment")
	startCmd.Flags().StringVarP(&consulNamespace, "consul-namespace", "", "consul", "Namespace in which consul is deployed")
}

func nddCtlrOptions(c int) controller.Options {
	return controller.Options{
		MaxConcurrentReconciles: c,
		RateLimiter:             ratelimiter.NewDefaultProviderRateLimiter(ratelimiter.DefaultProviderRPS),
	}
}

func getClient(scheme *runtime.Scheme) (client.Client, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}

	return client.New(cfg, client.Options{Scheme: scheme})

}
