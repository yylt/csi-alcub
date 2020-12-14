package controlrpc

import (
	"context"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/pborman/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	klog "k8s.io/klog/v2"
)

func (c *Controller) ControllerGetCapabilities(context.Context, *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: c.caps,
	}, nil
}

// Called by external-provisor, and only once
// - prepare: check capacity,mount/block,
// - check name exist?
// - create volume and uuid
// - create cr which used by node rpc
func (c *Controller) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	// Check arguments
	if len(req.GetName()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Name missing in request")
	}
	caps := req.GetVolumeCapabilities()
	if caps == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capabilities missing in request")
	}

	// Keep a record of the requested access types.
	var (
		accessTypeMount bool
		topologies      []*csi.Topology
	)

	for _, ca := range caps {
		if ca.GetMount() != nil {
			accessTypeMount = true
		}
	}
	// A real driver would also need to check that the other
	// fields in VolumeCapabilities are sane. The check above is
	// just enough to pass the "[Testpattern: Dynamic PV (block
	// volmode)] volumeMode should fail in binding dynamic
	// provisioned PV to PVC" storage E2E test.

	if !accessTypeMount {
		return nil, status.Error(codes.InvalidArgument, "cannot support non mount access type")
	}
	if req.GetVolumeContentSource() != nil {
		return nil, status.Error(codes.InvalidArgument, "not support create volume from source")
	}
	// Check for maximum available capacity
	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	//TODO check max storage capacity

	alcub := c.alcubControl.GetByName(req.GetName())
	if alcub != nil {
		if alcub.Spec.Capacity < capacity {
			return nil, status.Errorf(codes.AlreadyExists, "Volume with the same name: %s but with different size already exist", req.GetName())
		}
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      alcub.Spec.Uuid,
				CapacityBytes: int64(alcub.Spec.Capacity),
				VolumeContext: req.GetParameters(),
				ContentSource: req.GetVolumeContentSource(),
			},
		}, nil
	}

	volumeID := uuid.NewUUID().String()
	//TODO add volume id into volumeContext and check by node
	_, err := c.createVolume(req.GetParameters(), req.GetName(), volumeID, capacity)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create volume %v, %v", volumeID, err)
	}

	//TODO both add to topology?
	if accessReq := req.GetAccessibilityRequirements(); accessReq != nil {
		if accessReq.Requisite != nil {
			topologies = accessReq.Requisite
		} else if accessReq.Preferred != nil {
			topologies = accessReq.Preferred
		}
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:           volumeID,
			CapacityBytes:      req.GetCapacityRange().GetRequiredBytes(),
			VolumeContext:      req.GetParameters(),
			ContentSource:      req.GetVolumeContentSource(),
			AccessibleTopology: topologies,
		},
	}, nil
}

// check cr exist
// cr status is ready to delete
// delete image and cr now
func (c *Controller) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	var (
		volid = req.VolumeId
	)
	alcub := c.alcubControl.GetByUuid(volid)
	if alcub == nil {
		klog.V(4).Infof("volume %v had deleted!", volid)
		return &csi.DeleteVolumeResponse{}, nil
	}
	err := c.deleteVolume(alcub)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete volume %v: %v", volid, err)
	}
	klog.V(4).Infof("volume %v successfully deleted", volid)
	return &csi.DeleteVolumeResponse{}, nil
}

func (c *Controller) ValidateVolumeCapabilities(context.Context, *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (c *Controller) ControllerPublishVolume(context.Context, *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
func (c *Controller) ControllerUnpublishVolume(context.Context, *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (c *Controller) ListVolumes(context.Context, *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
func (c *Controller) GetCapacity(context.Context, *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
func (c *Controller) CreateSnapshot(context.Context, *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
func (c *Controller) DeleteSnapshot(context.Context, *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
func (c *Controller) ListSnapshots(context.Context, *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
func (c *Controller) ControllerExpandVolume(context.Context, *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
func (c *Controller) ControllerGetVolume(context.Context, *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func getControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) []*csi.ControllerServiceCapability {
	var csc []*csi.ControllerServiceCapability

	for _, cs := range cl {
		klog.Infof("Enabling controller service capability: %v", cs.String())
		csc = append(csc, &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cs,
				},
			},
		})
	}

	return csc
}
