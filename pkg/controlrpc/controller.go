package controlrpc

import (
	"fmt"
	"strings"

	alcubv1beta1 "github.com/yylt/csi-alcub/pkg/api/v1beta1"
	"github.com/yylt/csi-alcub/pkg/manager"
	rbd2 "github.com/yylt/csi-alcub/pkg/rbd"
	"github.com/yylt/csi-alcub/pkg/store"

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
	rbd *rbd2.Rbd

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

// 1. add blacklist
// 2. notify alcub server: node is not ready
// called by node reconcile
func (c *Controller) StopNode(nodename string, addblack bool) error {
	var (
		err  error
		errs strings.Builder
	)
	node := c.alcubControl.GetNodeInfo(nodename)
	if node == nil {
		klog.Errorf("no found storage ip on node %s", nodename)
		return fmt.Errorf("not found")
	}

	// add blacklist
	if addblack {
		err = rbd2.AddBlackList(node.StoreIp, fmt.Sprintf("csi-alcub-%s", c.nodeID))
		if err != nil {
			klog.Errorf("add blacklist on ipaddr %s fail: %v", node.StoreIp.String(), err)
			return err
		}
	}

	c.alcubDynConf.Nodename = nodename

	failfn := func(AlucbUrl string) error {
		c.alcubDynConf.AlucbUrl = []byte(AlucbUrl)
		return c.store.FailNode(&c.alcubDynConf, nodename)
	}
	for _, v := range node.NodeUrls {
		if v == "" {
			continue
		}
		if strings.Index(v, nodename) > 0 {
			err = failfn(v)
			if err != nil {
				errs.WriteString(err.Error())
				continue
			}
			break
		}
	}
	if errs.Len() != 0 {
		klog.Errorf("notify store fail_node fail: %v", errs.String())
	}
	return nil
}

func (c *Controller) StartNode(nodename string, rmblack bool) error {
	var (
		err error
	)
	node := c.alcubControl.GetNodeInfo(nodename)
	if node == nil {
		klog.Errorf("no found storage ip on node %s", nodename)
		return fmt.Errorf("not found")
	}
	if rmblack {
		// remove blacklist
		err = rbd2.RmBlackList(node.StoreIp, fmt.Sprintf("csi-alcub-%s", c.nodeID))
		if err != nil {
			klog.Errorf("remove blacklist on ipaddr %s fail: %v", node.StoreIp.String(), err)
			return err
		}
	}
	//TODO Store should start ?
	return nil
}

func (c *Controller) deleteVolume(alcub *alcubv1beta1.CsiAlcub) error {
	err := c.rbd.DeleteImage(alcub.Spec.RbdSc, alcub.Spec.Image)
	if err != nil {
		klog.Errorf("delete image failed:%v", err)
		return err
	}
	return c.alcubControl.Delete(alcub.Name)
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
			c.rbd.DeleteImage(v, uuid)
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
