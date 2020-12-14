package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CsiAlcubSpec struct {
	Uuid string `json:"uuid"`
	//capacity
	Capacity int64  `json:"capacity"`
	RbdSc    string `json:"rbdStorageClass"`
	// alcub need pool and image,
	// if not use alcub, pls add more param.
	Pool  string `json:"pool"`
	Image string `json:"image"`
}

type VolumeInfo struct {
	//dev path, such as /dev/rbd1 .etc
	Devpath   string `json:"devpath,omitempty"`
	StorageIp string `json:"storageip,omitempty"`
}

type CsiAlcubStatus struct {
	VolumeInfo VolumeInfo `json:"volumeInfo,omitempty"`

	//fill in the node name which is first attached
	Prenode string `json:"prenode,omitempty"`

	// fill in the node which is now use the volume
	Node     string   `json:"node,omitempty"`
	AllNodes []string `json:"zone,omitempty"`
}

// +kubebuilder:resource:scope=Cluster
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:JSONPath=.status.node,name="Node",type=string
// +kubebuilder:printcolumn:JSONPath=.status.prenode,name="PreNode",type=string
// +kubebuilder:printcolumn:JSONPath=.status.volumeInfo.devpath,name="Dev",type=string
// +kubebuilder:printcolumn:JSONPath=.status.volumeInfo.storageip,name="StorageIp",type=string
type CsiAlcub struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CsiAlcubSpec   `json:"spec,omitempty"`
	Status CsiAlcubStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type CsiAlcubList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CsiAlcub `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CsiAlcub{}, &CsiAlcubList{})
}
