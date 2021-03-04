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

type updateNodeFn func(node *corev1.Node)

var (
	UnreachableTaintTemplate = &corev1.Taint{
		Key:    corev1.TaintNodeUnreachable,
		Effect: corev1.TaintEffectNoSchedule,
	}
	//hosthaKv   = map[string]string{"hostha-maintain": "true"}
	csiBlackKv = map[string]string{"csi-alcub.io/maintain": "true"}

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

	for k, v := range labels {
		if key == k && val == v {
			alcubExist = true
		}

		if _, ok := n.csilabel[k]; ok {
			nodeLabelExist = true
		}
	}
	klog.V(5).Infof("node(%s) alcub-manager label exist:%s, csi node label exist:%s", nodename, alcubExist, nodeLabelExist)
	// delete csi label
	if !alcubExist {
		if !nodeLabelExist {
			return nil
		}
		klog.V(2).Infof("remove csi node label on node %s", nodename)
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
	klog.V(2).Infof("add csi node label on node %s", nodename)
	return func(node *corev1.Node) {
		for k, v := range n.csilabel {
			node.Labels[k] = v
		}
	}
}

// node notready or maintain should do someting
// 1. add/remove blacklist
// 2. call some api
// DEPRECATED, it's useless now!
func (n *Node) notreadyController(node *corev1.Node) updateNodeFn {
	var (
		notready   bool
		hamaintain bool
		csiblack   bool
	)
	if node.Spec.Unschedulable {
		notready = true
	}
	if inMaps(node.Labels, n.halabel) {
		hamaintain = true
	}
	if inMaps(node.Annotations, csiBlackKv) {
		csiblack = true
	}
	// reference: doc/csi故障处理
	// node is in probleam state, shold stop_node
	if notready && hamaintain {
		if csiblack {
			klog.V(2).Infof("csi black label still exist, skip call stop_node")
			return nil
		} else {
			err := n.manager.StopNode(node.Name, !inMaps(node.Labels, n.halabel))
			if err != nil {
				klog.Errorf("call stop_node failed: %v", err)
				return nil
			}
			klog.V(2).Infof("success call stop_node")
			return func(node *corev1.Node) {
				for k, v := range csiBlackKv {
					node.Annotations[k] = v
				}
			}
		}
	}
	// should check node recover from csi black status
	if csiblack {
		var isrecover = true
		if notready || hamaintain {
			isrecover = false
		}
		for _, taint := range node.Spec.Taints {
			if taint.MatchTaint(UnreachableTaintTemplate) {
				isrecover = false
			}
		}
		if isrecover {
			err := n.manager.StartNode(node.Name, !inMaps(node.Labels, n.halabel))
			if err != nil {
				klog.Errorf("controller start node failed: %v", err)
				return nil
			}
			klog.Infof("success controller start node")
			return func(node *corev1.Node) {
				for k, _ := range csiBlackKv {
					delete(node.Annotations, k)
				}
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
	return ctrl.Result{}, err
}

func (n *Node) NotReadyNodes() []string {
	var (
		nodes    corev1.NodeList
		retnodes []string
	)
	err := n.client.List(n.ctx, &nodes)
	if err != nil {
		klog.Errorf("list node failed:%v", err)
		return retnodes
	}
	for _, node := range nodes.Items {
		for _, taint := range node.Spec.Taints {
			if taint.MatchTaint(UnreachableTaintTemplate) {
				retnodes = append(retnodes, node.Name)
			}
		}
	}
	return retnodes
}

func (n *Node) LabledNodes() []string {
	var (
		nodes    corev1.NodeList
		retnodes []string
	)
	err := n.client.List(n.ctx, &nodes)
	if err != nil {
		klog.Errorf("list node failed:%v", err)
		return retnodes
	}
	for _, node := range nodes.Items {
		if inMaps(node.Labels, n.csilabel) {
			retnodes = append(retnodes, node.Name)
		}
	}
	return retnodes
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
	if kv == nil {
		return true
	}
	if src == nil {
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
