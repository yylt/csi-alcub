package commands

import (
	flag "github.com/spf13/pflag"
	alcubv1beta1 "github.com/yylt/csi-alcub/pkg/api/v1beta1"
	"github.com/yylt/csi-alcub/pkg/store"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var (
	scheme         = runtime.NewScheme()
	storeConf      = store.AlcubConf{}
	nodename       string
	leaderID       string
	alcubKeyPrefix string
	leaderElector  bool
	endpoint       string
	storageIfName  string
)

func ApplyStore(flagset *flag.FlagSet) {
	flagset.StringVar(&storeConf.ApiUrl, "alcub-api-url", "", "alcub api url")
	flagset.StringVar(&storeConf.User, "alcub-user", "", "alucb username")
	flagset.StringVar(&storeConf.Password, "alcub-password", "", "alcub password")
	flagset.StringVar(&storeConf.AlucbPool, "alcub-pool-name", "", "alcub pool name")
}

func ApplyLeaderConf(flagset *flag.FlagSet) {
	flagset.StringVar(&leaderID, "leader-id", "", "leader electore id")
	flagset.BoolVar(&leaderElector, "leader-elect", false, "leader enable")
}

func ApplyAlcubConf(flagset *flag.FlagSet) {
	flagset.StringVar(&alcubKeyPrefix, "alcub-key-prefix", "csi-alcub", "leader electore id")
}

func ApplyNodeName(flagset *flag.FlagSet) {
	flagset.StringVar(&nodename, "node-name", "", "node name")
}

func ApplyEndpoint(flagset *flag.FlagSet) {
	flagset.StringVar(&endpoint, "endpoint", "", "unix socket file path, should start unix://")
}

func ApplyStorageIfName(flagset *flag.FlagSet) {
	flagset.StringVar(&storageIfName, "storage-if-name", "", "storage net interface name")
}

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = alcubv1beta1.AddToScheme(scheme)

}
