package main

import (
	"flag"
	"os"

	alcubv1beta1 "github.com/yylt/csi-alcub/pkg/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	klog "k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)


var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = alcubv1beta1.AddToScheme(scheme)
}

func main()  {
	var klogv2fs flag.FlagSet

	klog.InitFlags(&klogv2fs)

	flag.Parse()

	mgr, err := ctrl.NewManager(config.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		klog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	// TODO

	klog.Info("starting manager")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		klog.Error(err, "unable to run manager")
		os.Exit(1)
	}
}