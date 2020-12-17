package commands

import (
	flag "github.com/spf13/pflag"
	alcubv1beta1 "github.com/yylt/csi-alcub/pkg/api/v1beta1"
	"github.com/yylt/csi-alcub/pkg/store"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"strings"
)

var (
	scheme         = runtime.NewScheme()
	storeConf      = store.AlcubConf{}
	nodename       string
	leader         = &leaderInfo{}
	labels         = &labelkv{}
	alcubKeyPrefix string
	drivername     string
	endpoint       string
	storageIfName  string
)

type labelkv struct {
	filterkv   string
	hakv       string
	csilabelkv string
}

type leaderInfo struct {
	Id     string
	enable bool
}

func ApplyStore(flagset *flag.FlagSet) {
	flagset.StringVar(&storeConf.ApiUrl, "alcub-api-url", "", "alcub api url")
	flagset.StringVar(&storeConf.User, "alcub-user", "", "alucb username")
	flagset.StringVar(&storeConf.Password, "alcub-password", "", "alcub password")
	flagset.StringVar(&storeConf.AlucbPool, "alcub-pool-name", "", "alcub pool name")
}

func ApplyLeaderConf(flagset *flag.FlagSet) {
	flagset.StringVar(&leader.Id, "leader-id", "", "leader electore id")
	flagset.BoolVar(&leader.enable, "leader-elect", false, "leader enable")
}

func ApplyLabels(flagset *flag.FlagSet) {
	flagset.StringVar(&labels.filterkv, "filter-label", "", "filter key-value, support template, now %N replaced by nodename,example: csi-alcub=enable ")
	flagset.StringVar(&labels.hakv, "ha-maintain-label", "", "when exist, node will not add csi maintain label!,example: hamaintain=enable ")
	flagset.StringVar(&labels.csilabelkv, "csi-label", "", "when filter key-value exist, csi-label key-value will added on node,example: csi-alcub=enable ")
}

func ApplyNode(flagset *flag.FlagSet) {
	flagset.StringVar(&nodename, "node-name", "", "node name")
}

func ApplyCsiInfo(flagset *flag.FlagSet) {
	flagset.StringVar(&endpoint, "endpoint", "", "unix socket file path, should start unix://")
	flagset.StringVar(&drivername, "driver-name", "alcub.csi.es.io", "node name")
}

func ApplyStorageIfName(flagset *flag.FlagSet) {
	flagset.StringVar(&storageIfName, "storage-if-name", "", "storage net interface name")
}

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = alcubv1beta1.AddToScheme(scheme)

}

func splitLabel(s string) map[string]string {
	ss := strings.Split(s, "=")
	if len(ss) != 2 {
		return nil
	}
	return map[string]string{ss[0]: ss[1]}
}
