package noderpc

import (
	"fmt"
	"net"

	alcubv1beta1 "github.com/yylt/csi-alcub/pkg/api/v1beta1"
	"github.com/yylt/csi-alcub/pkg/manager"
	rbd2 "github.com/yylt/csi-alcub/pkg/rbd"
	"github.com/yylt/csi-alcub/pkg/store"
	"github.com/yylt/csi-alcub/utils"

	"github.com/container-storage-interface/spec/lib/go/csi"
	klog "k8s.io/klog/v2"
)

var _ csi.NodeServer = &Node{}

const (
	TopologyKeyNode = "topology.alcub.csi/node"
	defaultPerm     = 0750
)

type delfn func()
type okfn func() error

type Node struct {
	store        store.Alcuber
	alcubControl *manager.AlcubCon
	//Node resource store
	rbd *rbd2.Rbd

	maxVolumesPerNode int64

	nodeID string

	nodename string

	storeip string
}

func NewNode(store store.Alcuber, alcubControl *manager.AlcubCon, rbd *rbd2.Rbd, nodename, storeifname string) *Node {
	node := &Node{
		store:             store,
		alcubControl:      alcubControl,
		rbd:               rbd,
		maxVolumesPerNode: 0, //TODO now alcub is unlimit
		nodeID:            nodename,
		nodename:          nodename,
		storeip:           getStoraIfIp(storeifname),
	}
	if node.storeip == "" {
		panic("not found storage ip")
	}
	return node
}

func (c *Node) detachDevice(alcub *alcubv1beta1.CsiAlcub) error {
	err := c.store.DoDisConn(nil, alcub.Spec.Pool, alcub.Spec.Image)
	if err != nil {
		klog.Errorf("detach device failed: %v", err)
	}
	klog.V(2).Infof("deatach device success pool: %v, image:%v", alcub.Spec.Pool, alcub.Spec.Image)
	return err
}

func (c *Node) attachDevice(alcub *alcubv1beta1.CsiAlcub) (string, error) {
	devpath, err := c.store.DoConn(nil, alcub.Spec.Pool, alcub.Spec.Image)
	if err != nil {
		klog.Errorf("attach device failed: %v", err)
		return "", err
	}
	if devpath == "" {
		klog.Errorf("attach path is null, spec: %v", alcub.Spec)
		return "", fmt.Errorf("device path is null")
	}
	return devpath, err
}

// retrun
// string: device
// delfn: delete function which called when next action failed
// okfn: success function which called when next action success
func (c *Node) preMountValid(alcub *alcubv1beta1.CsiAlcub) (string, delfn, okfn, error) {
	// node is null: the volume not mounted
	var (
		dev      string
		err      error
		nodes    []string
		okAttach bool
	)
	// node is not null: the volume had mounted by other,
	//  other node can not handler volume, because not ready , etc...
	klog.V(2).Infof("in preMount, the status is %v, the nodename: %v", alcub.Status, c.nodename)
	if alcub.Status.Node == "" {
		okAttach = true
	}
	if alcub.Status.Node == c.nodename {
		okAttach = true
	}
	if okAttach {
		//TODO alcub server is idempotent?
		nodes, err = c.store.GetNode(nil, c.nodename)
		if err != nil {
			return "", nil, nil, err
		}
		dev, err = c.attachDevice(alcub)
		if err != nil {
			return dev, nil, nil, err
		}
	}
	faielfunc := func() {
		//TODO ensure device is removed success
		c.detachDevice(alcub)
	}
	successfunc := func() error {
		if alcub.Status.Node != "" {
			if alcub.Status.Prenode == "" {
				alcub.Status.Prenode = alcub.Status.Node
			}
		}
		alcub.Status.Node = c.nodename
		alcub.Status.AllNodes = nodes
		alcub.Status.VolumeInfo = alcubv1beta1.VolumeInfo{
			Devpath:   dev,
			StorageIp: c.storeip,
		}
		return c.alcubControl.Update(alcub.Name, nil, &alcub.Status)
	}
	if !okAttach {
		//TODO not ready node and the volume be migrated to other node
		return "", nil, nil, fmt.Errorf("not impl")
	}
	if dev == "" {
		return "", nil, nil, fmt.Errorf("attach volulme successs, but the device path is null")
	}
	return dev, faielfunc, successfunc, nil
}

func (c *Node) preUnmountValid(alcub *alcubv1beta1.CsiAlcub) (delfn, okfn, error) {
	var (
		err error
	)
	klog.V(2).Infof("in preUnmount, the status is %v, the nodename: %v", alcub.Status, c.nodename)
	if alcub.Status.Node != c.nodename {
		//TODO the volume had migerated when node is notready
		return nil, nil, fmt.Errorf("not impl")
	}
	successfunc := func() error {
		err = c.detachDevice(alcub)
		if err != nil {
			//TODO detachDevice func should be idempotent.
			return err
		}
		alcub.Status = alcubv1beta1.CsiAlcubStatus{}
		return c.alcubControl.Update(alcub.Name, nil, &alcub.Status)
	}
	return nil, successfunc, nil
}

// check some zone in spec
// now this is not need because json option is non omitempty
func validAlcub(alcub *alcubv1beta1.CsiAlcub) error {
	return nil
}

func getStoraIfIp(storeifname string) string {
	var ipaddr string
	err := utils.LookupAddresses(func(name string, ip net.IP, ipmask net.IPMask) bool {
		klog.V(4).Infof("Found net interface:%v, ip: %v", name, ip.String())
		if name == storeifname {
			ipaddr = ip.String()
			return false
		}
		return true
	})
	if err != nil {
		klog.Errorf("fetech storage ip addr failed: %v", err)
		return ""
	}
	if ipaddr == "" {
		klog.Errorf("not found storage ip addr by name: %v", storeifname)
		return ""
	}
	return ipaddr
}
