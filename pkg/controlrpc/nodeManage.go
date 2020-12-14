package controlrpc

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	klog "k8s.io/klog/v2"
	"strings"
	"sync"

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	NodeAlcubLabelKey = "csi-alcub"
	NodeAlcubLabelVal = "enable"
)

type updateNodeFn func(node *corev1.Node)

var (
	UnreachableTaintTemplate = &corev1.Taint{
		Key:    corev1.TaintNodeUnreachable,
		Effect: corev1.TaintEffectNoExecute,
	}
)

// label control: watch node labels which added {alcublabelpre}-{nodename}
//                and add labels defined by self
// notready control: watch not ready node and decide to add some actions
type Node struct {
	ctx context.Context

	client  client.Client
	manager *Controller

	mu    sync.RWMutex
	nodes map[string]struct{}

	stopmu   sync.RWMutex
	nodestop map[string]struct{}

	// label key for filter
	lableKeyPrefix string
}

func NewNode(mgr ctrl.Manager, alcubLabelkeyPrefix string) (*Node, error) {
	n := &Node{
		ctx:            context.Background(),
		client:         mgr.GetClient(),
		lableKeyPrefix: alcubLabelkeyPrefix,
		nodes:          make(map[string]struct{}),
		nodestop:       make(map[string]struct{}),
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
		alcubExist     string
		nodeLabelExist bool
	)

	for k, _ := range labels {
		if strings.HasPrefix(k, n.lableKeyPrefix) {
			alcubExist = k
		}
	}
	if _, ok := labels[NodeAlcubLabelKey]; ok {
		nodeLabelExist = true
	}

	// delete csi label
	if alcubExist == "" {
		klog.V(2).Infof("node labelkey not include keyprefix %s", n.lableKeyPrefix)
		if !nodeLabelExist {
			return nil
		}
		klog.Infof("csi labelkey exist, but node do not have alcub label")
		return n.removeNode(nodename)
	}

	// had add csi label
	if nodeLabelExist {
		return nil
	}
	return n.addNode(nodename)
}

func (n *Node) notreadyController(node *corev1.Node) error {
	if len(node.Spec.Taints) == 0 {
		return nil
	}
	var (
		nodeUnreach bool
	)
	for _, taint := range node.Spec.Taints {
		if taint.MatchTaint(UnreachableTaintTemplate) {
			nodeUnreach = true
		}
	}

	if !nodeUnreach {
		return nil
	}
	if _, ok := n.nodestop[node.Name]; ok {
		return nil
	}
	err := n.manager.StopNode(node.Name)
	if err == nil {
		n.nodestop[node.Name] = struct{}{}
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
	err = n.notreadyController(&node)
	if err != nil {
		klog.Errorf("notready controller failed nodename:%v, %v", req.Name, err)
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

func (n *Node) removeNode(nodename string) updateNodeFn {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.nodes, nodename)
	return func(node *corev1.Node) {
		delete(node.Labels, NodeAlcubLabelKey)
	}
}

func (n *Node) addNode(nodename string) updateNodeFn {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.nodes[nodename] = struct{}{}
	return func(node *corev1.Node) {
		node.Labels[NodeAlcubLabelKey] = NodeAlcubLabelVal
	}
}
