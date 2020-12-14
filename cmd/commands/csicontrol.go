package commands

import (
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/yylt/csi-alcub/pkg/controlrpc"
	"github.com/yylt/csi-alcub/pkg/manager"
	rbd2 "github.com/yylt/csi-alcub/pkg/rbd"
	"github.com/yylt/csi-alcub/pkg/server"
	"github.com/yylt/csi-alcub/pkg/store"
	"github.com/yylt/csi-alcub/utils"
	"k8s.io/client-go/kubernetes"
	klog "k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

func NewControllerCmd() *cobra.Command {
	var cmd = &cobra.Command{
		Use:        "controller",
		Aliases:    nil,
		SuggestFor: nil,
		Short:      "csi controller implement",
		RunE: func(cmd *cobra.Command, args []string) error {
			kubeconfg := config.GetConfigOrDie()
			client := kubernetes.NewForConfigOrDie(kubeconfg)

			mgr, err := ctrl.NewManager(kubeconfg, ctrl.Options{
				Scheme: scheme,
			})
			if err != nil {
				klog.Error(err, "unable to set up overall controller manager")
				os.Exit(1)
			}
			alcubcon := manager.NewAlcubCon(mgr)

			s := store.NewClient(&storeConf, nil)

			nodecon, err := controlrpc.NewNode(mgr, alcubKeyPrefix)
			if err != nil {
				return err
			}

			rbd := rbd2.NewRbd(client, time.Second*5)

			csiController := controlrpc.NewController(nodename, s, alcubcon, nodecon, rbd)

			csiIdentify, err := server.NewIdenty(nodename, server.ControllerCapability())
			if err != nil {
				return err
			}
			runner := utils.NewCobraRunner(true, endpoint, csiIdentify, csiController, nil)
			err = mgr.Add(runner)
			if err != nil {
				return err
			}
			klog.Info("starting manager")
			if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
				klog.Error(err, "unable to run manager")
				return err
			}
			return nil
		},
	}
	flagset := cmd.LocalFlags()

	ApplyStore(flagset)
	ApplyAlcubConf(flagset)
	ApplyNodeName(flagset)
	ApplyLeaderConf(flagset)
	ApplyEndpoint(flagset)

	return cmd
}
