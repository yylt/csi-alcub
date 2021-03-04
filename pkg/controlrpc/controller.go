package controlrpc

import (
	"fmt"
	"strings"

	alcubv1beta1 "github.com/yylt/csi-alcub/pkg/api/v1beta1"
	"github.com/yylt/csi-alcub/pkg/manager"
	rbd2 "github.com/yylt/csi-alcub/pkg/rbd"
	"github.com/yylt/csi-alcub/pkg/store"
	"github.com/yylt/csi-alcub/utils"

	"github.com/container-storage-interface/spec/lib/go/csi"
	klog "k8s.io/klog/v2"
)

var _ csi.ControllerServer = &Controller{}

var (
	scParam = "scname"
)

type Controller struct {
	store store.Alcuber

	alcubControl *manager.AlcubCon
	//Node resource store
	rbd  *rbd2.Rbd
	node *Node
	caps []*csi.ControllerServiceCapability

	alcubDynConf store.DynConf
	nodeID       string
}

func NewController(nodeid string, store store.Alcuber, alcubControl *manager.AlcubCon, rbd *rbd2.Rbd) *Controller {
	return &Controller{
		rbd:          rbd,
		nodeID:       nodeid,
		store:        store,
		alcubControl: alcubControl,
		caps: getControllerServiceCapabilities(
			[]csi.ControllerServiceCapability_RPC_Type{
				csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
				csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
			}),
	}
}

func (c *Controller) SetupNode(nodemanager *Node) {
	if nodemanager == nil {
		panic("node manager is nil")
	}
	c.node = nodemanager
}

// start node, some actions
// 1. add blacklist
// 2. notify alcub server: node is not ready
// called by node reconcile
func (c *Controller) StopNode(nodename string, addblack bool) error {
	var (
		err error
	)
	node := c.alcubControl.GetNodeInfo(nodename)
	if node == nil {
		klog.Errorf("no found storage ip on node %s", nodename)
		return fmt.Errorf("not found")
	}

	// add blacklist
	if addblack {
		//TODO hostha had add blacklist, so we should not operat
		//err = rbd2.AddBlackList(node.StoreIp, fmt.Sprintf("csi-alcub-%s", c.nodeID))
		err = nil
		if err != nil {
			klog.Errorf("add blacklist on ipaddr %s fail: %v", node.StoreIp.String(), err)
			return err
		}
	}
	return c.notidyAlcub(nodename, node, true)
}

// start node, some actions
//1. remove black list on nodename
//2. flush data
func (c *Controller) StartNode(nodename string, rmblack bool) error {

	node := c.alcubControl.GetNodeInfo(nodename)
	if node == nil {
		klog.Errorf("no found storage ip on node %s", nodename)
		return fmt.Errorf("not found")
	}
	if rmblack {
		if node.StoreIp == nil {
			klog.Errorf("not found storage ip on node %s", nodename)
		}
		//err = rbd2.RmBlackList(node.StoreIp, fmt.Sprintf("csi-alcub-%s", c.nodeID))
	}
	return c.notidyAlcub(nodename, node, false)
}

func (c *Controller) deleteVolume(alcub *alcubv1beta1.CsiAlcub) error {
	err := c.rbd.DeleteImage(alcub.Spec.RbdSc, alcub.Spec.Image)
	if err != nil {
		klog.Errorf("delete image failed:%v", err)
		return err
	}
	return c.alcubControl.Delete(alcub.Name)
}

func (c *Controller) notidyAlcub(nodename string, zone *manager.Nodeinfo, fail bool) error {
	var (
		buferr  = utils.GetBuf()
		success bool
	)
	defer utils.PutBuf(buferr)

	c.alcubDynConf.Nodename = nodename

	actionfn := func(AlucbUrl string) error {
		c.alcubDynConf.AlucbUrl = []byte(AlucbUrl)
		if fail {
			return c.store.FailNode(&c.alcubDynConf, nodename)
		} else {
			if strings.Index(AlucbUrl, nodename) < 0 {
				klog.Infof("skip alcubUrl:%v, because dev stop must be in host:%v", AlucbUrl, nodename)
				return nil
			}
			return c.alcubControl.ForEach(func(a *alcubv1beta1.CsiAlcub) {
				if a.Spec.Pool == "" || a.Spec.Image == "" {
					klog.Infof("skip %v, pool or image not found", a.Name)
					return
				}
				err := c.store.DevStop(&c.alcubDynConf, a.Spec.Pool, a.Spec.Image)
				if err != nil {
					klog.Errorf("stop pool(%v) image(%v) failed: %v", a.Spec.Pool, a.Spec.Image, err)
					return
				}
				klog.V(2).Infof("stop pool(%v) image(%v) success", a.Spec.Pool, a.Spec.Image)
			})
		}
	}
	for _, v := range zone.Zones {
		if v == "" {
			continue
		}
		err := actionfn(v)
		if err != nil {
			buferr.WriteString(err.Error())
			continue
		}
		success = true
		break
	}
	if buferr.Len() != 0 {
		klog.Errorf("notify store alcub fail: %v", buferr.Bytes())
	}
	if fail {
		klog.Infof("notify store alcub fail-node success")
	} else {
		klog.Infof("notify store alcub dev-stop success")
	}

	if !success {
		return fmt.Errorf("notify alcub server failed!")
	}
	return nil
}

func (c *Controller) createVolume(params map[string]string, name, uuid string, bytesize int64) (*alcubv1beta1.CsiAlcubSpec, error) {

	if params == nil {
		return nil, fmt.Errorf("params is nil")
	}
	v, ok := params[scParam]
	if !ok {
		return nil, fmt.Errorf("not found %s in params", scParam)
	}
	volume, err := c.rbd.CreateImage(v, name, bytesize)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			//TODO should delete image forever if delete failed
			c.rbd.DeleteImage(v, name)
		}
	}()
	spec := &alcubv1beta1.CsiAlcubSpec{
		Pool:     volume.Pool,
		Image:    volume.Image,
		Capacity: bytesize,
		Uuid:     uuid,
		RbdSc:    v,
	}
	err = c.alcubControl.Create(name, spec)
	return spec, err
}
