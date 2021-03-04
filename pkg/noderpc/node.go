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
	klog.V(2).Infof("dev connect success, devpath: %v", devpath)
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
	klog.V(2).Infof("in preMount, %s the volumeInfo is %v", alcub.Name, alcub.Status.VolumeInfo)
	if alcub.Spec.Image == "" || alcub.Spec.Pool == "" {
		klog.Errorf("csialcub(%s) image or pool is null", alcub.Name)
		return "", nil, nil, fmt.Errorf("image or pool is null")
	}

	//check image is ready to use
	if c.store.GetImageStatus(nil, alcub.Spec.Pool, alcub.Spec.Image) == false {
		klog.Errorf("image(%v) pool(%v) is not ready", alcub.Spec.Pool, alcub.Spec.Image)
		return "", nil, nil, fmt.Errorf("image(%s) status is not ready, wait clear", alcub.Spec.Image)
	}

	if alcub.Status.Node == "" {
		okAttach = true
	}
	if alcub.Status.Node == c.nodename {
		okAttach = true
	}
	nodes, err = c.store.GetNode(nil, c.nodename)
	if err != nil {
		return "", nil, nil, err
	}

	if !okAttach {
		klog.V(2).Infof("expect node:%s, but status.node is %v", c.nodename, alcub.Status.Node)
	}

	dev, err = c.attachDevice(alcub)
	if err != nil {
		return dev, nil, nil, err
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

	return dev, faielfunc, successfunc, nil
}

func (c *Node) preUnmountValid(alcub *alcubv1beta1.CsiAlcub) (delfn, okfn, error) {
	var (
		err error
	)
	klog.V(2).Infof("in preUnmount, %s the volumeInfo is %v", alcub.Name, alcub.Status.VolumeInfo)
	if alcub.Status.Node != c.nodename {
		// Not here
		return nil, nil, fmt.Errorf("node excepte:%v, but here is %v", alcub.Status.Node, c.nodename)
	}
	successfunc := func() error {
		err = c.detachDevice(alcub)
		if err != nil {
			//TODO detachDevice func should be idempotent.
			return err
		}
		alcub.Status.Node = ""
		return c.alcubControl.Update(alcub.Name, nil, &alcub.Status)
	}
	return nil, successfunc, nil
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
