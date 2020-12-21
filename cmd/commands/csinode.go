package commands

import (
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/yylt/csi-alcub/pkg/manager"
	"github.com/yylt/csi-alcub/pkg/noderpc"
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

func NewNodeCmd() *cobra.Command {
	var cmd = &cobra.Command{
		Use:        "node",
		Aliases:    nil,
		SuggestFor: nil,
		Short:      "csi node implement",
		RunE: func(cmd *cobra.Command, args []string) error {
			kubeconfg := config.GetConfigOrDie()
			client := kubernetes.NewForConfigOrDie(kubeconfg)

			mgr, err := ctrl.NewManager(kubeconfg, ctrl.Options{
				Scheme:             scheme,
				MetricsBindAddress: "0",
			})
			if err != nil {
				klog.Error(err, "unable to set up overall controller manager")
				os.Exit(1)
			}
			alcubcon := manager.NewAlcubCon(mgr)
			var (
				storeDynConf store.DynConf
			)
			storeDynConf.Nodename = nodename

			s := store.NewClient(&storeConf, &storeDynConf)
			rbd := rbd2.NewRbd(client, time.Second*5)

			csiNode := noderpc.NewNode(s, alcubcon, rbd, nodename, storageIfName)

			csiIdentify, err := server.NewIdenty(drivername, server.ConstraCapability())
			if err != nil {
				return err
			}
			runner := utils.NewRunner(false, endpoint, csiIdentify, nil, csiNode)
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
	flagset := cmd.PersistentFlags()

	ApplyStore(flagset)
	ApplyNode(flagset)
	ApplyCsiInfo(flagset)
	ApplyStorageIfName(flagset)

	return cmd
}
