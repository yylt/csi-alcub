module github.com/yylt/csi-alcub

go 1.15

require (
	github.com/container-storage-interface/spec v1.3.0
	github.com/imroc/req v0.3.0
	github.com/kubernetes-sigs/sig-storage-lib-external-provisioner v4.1.0+incompatible
	github.com/pborman/uuid v1.2.0
	google.golang.org/grpc v1.27.1
	k8s.io/api v0.20.0-beta.2
	k8s.io/apimachinery v0.20.0-beta.2
	k8s.io/client-go v0.20.0-beta.2
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.4.0
	k8s.io/kubernetes v1.19.4
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	sigs.k8s.io/controller-runtime v0.6.4
)

replace k8s.io/api => k8s.io/api v0.20.0-beta.2

replace k8s.io/apimachinery => k8s.io/apimachinery v0.20.0-beta.2

replace k8s.io/client-go => k8s.io/client-go v0.20.0-beta.2

replace k8s.io/cloud-provider => k8s.io/cloud-provider v0.20.0-beta.2

replace k8s.io/kubectl => k8s.io/kubectl v0.20.0-beta.2

replace k8s.io/apiserver => k8s.io/apiserver v0.20.0-beta.2

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.20.0-beta.2

replace k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.20.0-beta.2

replace k8s.io/kube-proxy => k8s.io/kube-proxy v0.20.0-beta.2

replace k8s.io/cri-api => k8s.io/cri-api v0.20.0-beta.2

replace k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.20.0-beta.2

replace k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.20.0-beta.2

replace k8s.io/component-base => k8s.io/component-base v0.20.0-beta.2

replace k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.20.0-beta.2

replace k8s.io/metrics => k8s.io/metrics v0.20.0-beta.2

replace k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.20.0-beta.2

replace k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.20.0-beta.2

replace k8s.io/kubelet => k8s.io/kubelet v0.20.0-beta.2

replace k8s.io/cli-runtime => k8s.io/cli-runtime v0.20.0-beta.2

replace k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.20.0-beta.2

replace k8s.io/code-generator => k8s.io/code-generator v0.20.0-beta.2

replace github.com/gophercloud/gophercloud => github.com/gophercloud/gophercloud v0.11.0
