package manager

import (
	"context"
	"fmt"
	"net"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/klog"
	"sync"

	alcubv1beta1 "github.com/yylt/csi-alcub/pkg/api/v1beta1"
	mtypes "github.com/yylt/csi-alcub/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctrl "sigs.k8s.io/controller-runtime"

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	defaultNs  = ""
	finalizers = []string{"controller/csi-alcub"}
)

type Nodeinfo struct {
	StoreIp net.IP
	Nodes   []string
}

func (ni *Nodeinfo) DeepCopy() *Nodeinfo {
	tmpn := &Nodeinfo{}
	copy(tmpn.StoreIp, ni.StoreIp)
	copy(tmpn.Nodes, ni.Nodes)
	return tmpn
}

type AlcubCon struct {
	client client.Client
	reader cache.Cache
	ctx    context.Context

	mu sync.RWMutex

	uuidname map[string]string

	nodemu sync.RWMutex
	nodes  map[string]*Nodeinfo
}

func NewAlcubCon(mgr ctrl.Manager) *AlcubCon {
	alcub := &AlcubCon{
		client:   mgr.GetClient(),
		reader:   mgr.GetCache(),
		ctx:      context.Background(),
		mu:       sync.RWMutex{},
		uuidname: make(map[string]string),
		nodes:    make(map[string]*Nodeinfo),
	}
	err := alcub.probe(mgr)
	if err != nil {
		panic(err)
	}
	return alcub
}

func (al *AlcubCon) probe(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&alcubv1beta1.CsiAlcub{}).
		Complete(al)
}

// delete actually , and will block if delete is forbidden
// cache some information which add by noderpc
//   1. storageip
func (al *AlcubCon) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	var (
		alcub alcubv1beta1.CsiAlcub
		err   error
	)
	err = al.client.Get(al.ctx, req.NamespacedName, &alcub)
	if err != nil {
		if apierrs.IsNotFound(err) {
			// Delete event
			klog.Info("object had deleted", "object", req.String())
			return ctrl.Result{}, nil
		}
		klog.Error(err, "get object failed", "object", req.String())
		return reconcile.Result{}, err
	}
	if alcub.DeletionTimestamp != nil {
		klog.Info("object is deleting", "object", req.String())
		if al.validBeDelete(&alcub) == nil {
			alcub.Finalizers = nil
			retry.RetryOnConflict(retry.DefaultRetry, func() error {
				err := al.client.Update(al.ctx, &alcub)
				if err != nil {
					klog.Errorf("update object(%s) failed:%v", req.String(), err)
					return err
				}
				return nil
			})
		}
		return ctrl.Result{}, nil
	}
	err = al.reverseUuid(alcub.Spec.Uuid, alcub.Name)
	if err != nil {
		klog.Infof("Reconcile failed , reverse uuid failed:%v", err)
	}
	al.reverseNode(&alcub.Status)
	return ctrl.Result{}, nil
}

func (al *AlcubCon) Create(name string, spec *alcubv1beta1.CsiAlcubSpec) error {
	var (
		err    error
		reterr error
	)
	if spec == nil {
		return fmt.Errorf("spec is nil")
	}
	err = al.reverseUuid(spec.Uuid, name)
	if err != nil {
		return err
	}
	defer func() {
		if reterr != nil {
			al.releaseUuid(spec.Uuid)
		}
	}()

	newobj := alcubv1beta1.CsiAlcub{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Finalizers: finalizers,
		},
		Spec:   alcubv1beta1.CsiAlcubSpec{},
		Status: alcubv1beta1.CsiAlcubStatus{},
	}
	reterr = al.client.Create(al.ctx, &newobj)
	if reterr != nil {
		if apierrs.IsAlreadyExists(reterr) {
			return mtypes.NewAlreadyExistError(fmt.Sprintf("%s is alerady exist!", name))
		}
		return reterr
	}
	return nil
}

func (al *AlcubCon) Delete(name string) error {
	var (
		nsname = types.NamespacedName{
			Namespace: defaultNs,
			Name:      name,
		}
		obj = &alcubv1beta1.CsiAlcub{}
	)

	err := al.client.Get(al.ctx, nsname, obj)
	if err != nil {
		return err
	}
	err = al.validBeDelete(obj)
	if err != nil {
		return err
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return al.client.Delete(al.ctx, obj)
	})
}

func (al *AlcubCon) GetByUuid(uuid string) *alcubv1beta1.CsiAlcub {
	if uuid == "" {
		klog.Errorf("Uuid must not be null")
		return nil
	}
	name := al.getNameByuuid(uuid)
	if name == "" {
		klog.Errorf("Not found alcub by uuid %s", uuid)
		return nil
	}
	return al.GetByName(name)
}

func (al *AlcubCon) GetByName(name string) *alcubv1beta1.CsiAlcub {
	var (
		nsname = types.NamespacedName{
			Namespace: defaultNs,
			Name:      name,
		}
		obj = &alcubv1beta1.CsiAlcub{}
	)
	err := al.client.Get(al.ctx, nsname, obj)
	if err != nil {
		klog.Errorf("Get alcub by name failed:%v", err)
		return nil
	}
	return obj
}

func (al *AlcubCon) Update(name string, spec *alcubv1beta1.CsiAlcubSpec, stat *alcubv1beta1.CsiAlcubStatus) error {
	var (
		nsname = types.NamespacedName{
			Namespace: defaultNs,
			Name:      name,
		}
		obj = &alcubv1beta1.CsiAlcub{}
	)

	err := al.client.Get(al.ctx, nsname, obj)
	if err != nil {
		return err
	}
	if spec != nil {
		spec.DeepCopyInto(&obj.Spec)
	}
	if stat != nil {
		stat.DeepCopyInto(&obj.Status)
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return al.client.Update(al.ctx, obj)
	})
}

func (al *AlcubCon) releaseUuid(uuid string) {
	al.mu.Lock()
	defer al.mu.Unlock()
	delete(al.uuidname, uuid)
}

func (al *AlcubCon) getNameByuuid(uuid string) string {
	al.mu.RLock()
	defer al.mu.RUnlock()
	v, ok := al.uuidname[uuid]
	if ok {
		return v
	}
	return ""
}

func (al *AlcubCon) GetNodeInfo(nodename string) *Nodeinfo {
	al.nodemu.RLock()
	defer al.nodemu.RUnlock()
	v, ok := al.nodes[nodename]
	if ok {
		return v.DeepCopy()
	}
	return nil
}

func (al *AlcubCon) reverseNode(stat *alcubv1beta1.CsiAlcubStatus) {
	al.nodemu.Lock()
	defer al.nodemu.Unlock()

	if stat == nil {
		return
	}
	if stat.Node == "" || stat.VolumeInfo.StorageIp == "" {
		return
	}
	ip := net.ParseIP(stat.VolumeInfo.StorageIp)
	if ip == nil {
		klog.Errorf("parse ip %s failed", stat.VolumeInfo.StorageIp)
		return
	}

	oldv, ok := al.nodes[stat.Node]
	if !ok {
		oldv = &Nodeinfo{
			StoreIp: nil,
			Nodes:   nil,
		}
	}
	//TODO: check ip only
	if oldv.StoreIp.Equal(ip) {
		return
	}
	oldv.StoreIp = ip
	oldv.Nodes = stat.AllNodes
	al.nodes[stat.Node] = oldv
}

func (al *AlcubCon) reverseUuid(uuid, name string) error {
	al.mu.Lock()
	defer al.mu.Unlock()
	v, ok := al.uuidname[uuid]
	if ok {
		if v == name {
			return nil
		}
		return fmt.Errorf("Alerady exist: uuid %s, and value is %s", uuid, v)
	}
	al.uuidname[uuid] = name
	return nil
}

func (al *AlcubCon) validBeDelete(alcub *alcubv1beta1.CsiAlcub) error {
	if alcub.Status.Node != "" {
		return fmt.Errorf("status node is not nil")
	}
	if alcub.Status.Prenode != "" {
		return fmt.Errorf("status preNode is not nil")
	}
	if alcub.Status.VolumeInfo.Devpath != "" {
		return fmt.Errorf("status volumeinfo is not null")
	}
	return nil
}
