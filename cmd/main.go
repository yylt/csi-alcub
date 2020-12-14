package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/yylt/csi-alcub/cmd/commands"
	klog "k8s.io/klog/v2"
)

func init() {
	rand.Seed(time.Now().Unix())
}

func rootCmd() *cobra.Command {
	rootc := &cobra.Command{
		Use:   "hyper",
		Short: "csi alcub hyper command which include node and controller rpc",
	}

	rootflag := rootc.PersistentFlags()
	klog.InitFlags(nil)
	flag.Parse()
	rootflag.AddGoFlagSet(flag.CommandLine)

	rootc.AddCommand(
		commands.NewControllerCmd(),
		commands.NewNodeCmd(),
	)

	return rootc
}

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
