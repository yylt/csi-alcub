package controlrpc

import (
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	alcubv1beta1 "github.com/yylt/csi-alcub/pkg/api/v1beta1"
	"github.com/yylt/csi-alcub/pkg/manager"
	rbd2 "github.com/yylt/csi-alcub/pkg/rbd"
	"github.com/yylt/csi-alcub/pkg/store"
	"k8s.io/klog"
)

var _ csi.ControllerServer = &Controller{}

var (
	scParam = "scname"
)

type Controller struct {
	store store.Alcuber
	nodeControl *Node
	alcubControl *manager.AlcubCon
	//Node resource store
	rbd *rbd2.Rbd

	caps   []*csi.ControllerServiceCapability

	nodeID string

}

func NewController(nodeid string, alcubControl *manager.AlcubCon, nodeControl *Node,rbd *rbd2.Rbd) *Controller {
	return &Controller{
		rbd: rbd,
		nodeID: nodeid,
		alcubControl: alcubControl,
		nodeControl: nodeControl,
		caps:   getControllerServiceCapabilities(
			[]csi.ControllerServiceCapability_RPC_Type{
			csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		}),
	}
}

// 1. add blacklist
// 2. notify alcub server: node is not ready
// called by node reconcile
func (c *Controller) StopNode(nodename string) error {
	ipaddr := c.alcubControl.GetStorageIp(nodename)
	if ipaddr== nil {
		klog.Errorf("no found storage ip on node %s",nodename)
		return fmt.Errorf("not found")
	}
	dynconf,err := store.GetDynConf(ipaddr,nodename)
	if err!= nil {
		klog.Errorf("get confiure on node %s fail: %v",nodename,err)
		return err
	}
	rbd2.AddBlackList()
	c.store.FailNode(dynconf,nodename)
}

func (c *Controller) deleteVolume(alcub *alcubv1beta1.CsiAlcub) error {
	return c.alcubControl.Delete(alcub.Name)
}

func (c *Controller) createVolume(params map[string]string, name, uuid string, bytesize int64) (*alcubv1beta1.CsiAlcubSpec, error) {

	if params==nil{
		return nil, fmt.Errorf("params is nil")
	}
	v,ok:=params[scParam]
	if !ok {
		return nil, fmt.Errorf("not found %s in params",scParam)
	}
	volume, err := c.rbd.CreateImage(v,name,bytesize)
	if err!= nil {
		return nil, err
	}
	defer func() {
		if err!= nil {
			//TODO should delete image forever if delete failed
			c.rbd.DeleteImage(v,uuid)
		}
	}()
	spec:= &alcubv1beta1.CsiAlcubSpec{
		Pool: volume.Pool,
		Image: volume.Image,
		Capacity: bytesize,
		Uuid: uuid,
	}
	err = c.alcubControl.Create(name,spec)
	return spec,err
}
