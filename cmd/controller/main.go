package main

import (
	"flag"
	mpcmetrics "github.com/konflux-ci/multi-platform-controller/pkg/metrics"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"

	zap2 "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/klog/v2"

	// needed for hack/update-codegen.sh
	_ "k8s.io/code-generator"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/konflux-ci/multi-platform-controller/pkg/controller"
	k8scontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	//+kubebuilder:scaffold:imports
	"github.com/go-logr/logr"
)

var (
	mainLog logr.Logger
)

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var abAPIExportName string
	var secureMetrics bool
	var concurrentReconciles int
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&abAPIExportName, "api-export-name", "jvm-build-service", "The name of the jvm-build-service APIExport.")

	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.IntVar(&concurrentReconciles, "concurrent-reconciles", 10, "The concurrency level for reconciling resources.")

	opts := zap.Options{
		TimeEncoder: zapcore.RFC3339TimeEncoder,
		ZapOpts:     []zap2.Option{zap2.WithCaller(true)},
	}
	opts.BindFlags(flag.CommandLine)
	klog.InitFlags(flag.CommandLine)
	flag.Parse()

	logger := zap.New(zap.UseFlagOptions(&opts))

	ctrl.SetLogger(logger)
	mainLog = ctrl.Log.WithName("main")
	ctx := ctrl.SetupSignalHandler()
	restConfig := ctrl.GetConfigOrDie()
	klog.SetLogger(mainLog)

	var mgr ctrl.Manager
	var err error
	managerOptions := ctrl.Options{
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "5483be8f.redhat.com",
		Metrics: metricsserver.Options{
			BindAddress:    metricsAddr,
			SecureServing:  secureMetrics,
			FilterProvider: filters.WithAuthenticationAndAuthorization,
		},
	}

	controllerOptions := k8scontroller.Options{
		MaxConcurrentReconciles: concurrentReconciles,
	}

	mgr, err = controller.NewManager(restConfig, managerOptions, controllerOptions)
	if err != nil {
		mainLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = mpcmetrics.RegisterCommonMetrics(ctx, metrics.Registry); err != nil {
		mainLog.Error(err, "failed to register common metrics")
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		mainLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		mainLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	mainLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		mainLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
