package controlrpc

import (
	"bytes"
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	klog "k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
//NodeAlcubLabelKey = "csi-alcub"
//NodeAlcubLabelVal = "enable"
)

type updateNodeFn func(node *corev1.Node)

var (
	UnreachableTaintTemplate = &corev1.Taint{
		Key:    corev1.TaintNodeUnreachable,
		Effect: corev1.TaintEffectNoExecute,
	}
	//hosthaKv   = map[string]string{"hostha-maintain": "true"}
	csiBlackKv = map[string]string{"csi-alcub-maintain": "true"}

	tempNodeKey = "%N"
)

// label control: watch node labels which added {alcublabelpre}-{nodename}
//                and add labels defined by self
// notready control: watch not ready node and decide to add some actions
type Node struct {
	ctx context.Context

	client  client.Client
	manager *Controller

	// the label updated to node,
	// TODO(yy) only support one pair
	csilabel map[string]string

	halabel map[string]string

	// label key for filter, the key used as:
	filterKey []byte

	filtervalue []byte
}

func NewNode(mgr ctrl.Manager, manager *Controller, filterKey, filtervalue []byte, halabel, csilabel map[string]string) (*Node, error) {
	if csilabel == nil {
		return nil, fmt.Errorf("label must not be nil!")
	}
	n := &Node{
		ctx:       context.Background(),
		client:    mgr.GetClient(),
		filterKey: filterKey,

		filtervalue: filtervalue,

		halabel:  halabel,
		manager:  manager,
		csilabel: csilabel,
	}
	return n, n.probe(mgr)
}

func (n *Node) probe(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Complete(n)
}

// check node alcub label exist and update/delete nodes
func (n *Node) labelController(labels map[string]string, nodename string) updateNodeFn {
	if labels == nil {
		return nil
	}
	var (
		alcubExist     bool
		nodeLabelExist bool
	)
	tmpmap := map[string]string{tempNodeKey: nodename}
	key := string(tempReplace(n.filterKey, tmpmap))
	val := string(tempReplace(n.filtervalue, tmpmap))

	klog.V(2).Infof("label controller filter label: %s=%s", key, val)
	for k, v := range labels {
		if key == k && val == v {
			alcubExist = true
		}

		if _, ok := n.csilabel[k]; ok {
			nodeLabelExist = true
		}
	}

	// delete csi label
	if !alcubExist {
		klog.V(2).Infof("node %s is not alcub type!", nodename)
		if !nodeLabelExist {
			return nil
		}
		klog.Infof("csi labelkey exist, but node do not have alcub label")
		return func(node *corev1.Node) {
			for k, _ := range n.csilabel {
				delete(node.Labels, k)
			}
		}
	}

	// had add csi label
	if nodeLabelExist {
		return nil
	}
	return func(node *corev1.Node) {
		for k, v := range n.csilabel {
			node.Labels[k] = v
		}
	}
}

func (n *Node) notreadyController(node *corev1.Node) updateNodeFn {
	if len(node.Spec.Taints) == 0 {
		return nil
	}
	var (
		nodeUnreach bool
	)
	for _, taint := range node.Spec.Taints {
		//TODO(y) check Unreachable Taint maybe too late
		if taint.MatchTaint(UnreachableTaintTemplate) {
			nodeUnreach = true
		}
	}

	// check or update label
	if nodeUnreach && !inMaps(node.Labels, csiBlackKv) {
		err := n.manager.StopNode(node.Name, !inMaps(node.Labels, n.halabel))
		if err != nil {
			klog.Errorf("controller stop node failed: %v", err)
			return nil
		}
		return func(node *corev1.Node) {
			for k, v := range csiBlackKv {
				node.Labels[k] = v
			}
		}
	}
	if !nodeUnreach && inMaps(node.Labels, csiBlackKv) {
		err := n.manager.StartNode(node.Name, !inMaps(node.Labels, n.halabel))
		if err != nil {
			klog.Errorf("controller start node failed: %v", err)
			return nil
		}
		return func(node *corev1.Node) {
			for k, _ := range csiBlackKv {
				delete(node.Labels, k)
			}
		}
	}
	return nil
}

func (n *Node) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	var (
		node      corev1.Node
		err       error
		updateFns []updateNodeFn
	)
	defer func() {
		n.updateNode(req, updateFns)
	}()
	err = n.client.Get(n.ctx, req.NamespacedName, &node)
	if err != nil {
		if apierrs.IsNotFound(err) {
			// Delete event
			klog.Info("object had deleted", "object", req.String())
			return ctrl.Result{}, nil
		}
		klog.Error(err, "get object failed", "object", req.String())
	}
	if node.DeletionTimestamp != nil {
		klog.Info("object is deleting", "object", req.String())
		return ctrl.Result{}, nil
	}
	fn := n.labelController(node.Labels, node.Name)
	if fn != nil {
		updateFns = append(updateFns, fn)
	}
	fn = n.notreadyController(&node)
	if fn != nil {
		updateFns = append(updateFns, fn)
	}
	//TODO the node which beccome ready should remove blacklist
	return ctrl.Result{}, err
}

func (n *Node) updateNode(req reconcile.Request, fns []updateNodeFn) {
	retry.RetryOnConflict(retry.DefaultRetry, func() error {
		original := &corev1.Node{}

		if err := n.client.Get(n.ctx, req.NamespacedName, original); err != nil {
			klog.Error(err, "get object failed", "object", req.String())
			return err
		}
		for _, fn := range fns {
			fn(original)
		}
		err := n.client.Update(n.ctx, original)
		if err != nil {
			klog.Errorf("update object(%s) failed:%v", req.String(), err)
			return err
		}
		return nil
	})
}

func inMaps(src map[string]string, kv map[string]string) bool {
	if kv == nil || src == nil {
		return false
	}
	for k, v := range kv {
		if v0, ok := src[k]; ok {
			if v0 == v {
				continue
			}
		}
		return false
	}
	return true
}

func tempReplace(src []byte, replaces map[string]string) []byte {
	var tmp = src
	for k, v := range replaces {
		tmp = bytes.ReplaceAll(tmp, []byte(k), []byte(v))
	}
	if tmp == nil {
		return []byte{}
	}
	return tmp
}
