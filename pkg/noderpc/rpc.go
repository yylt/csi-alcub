package noderpc

import (
	"context"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
	utilexec "k8s.io/utils/exec"
	"k8s.io/utils/mount"
	"os"
)

func (c *Node) NodeGetCapabilities(ctx context.Context,req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error){
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
		},
	}, nil
}

func (c *Node) NodeGetInfo(ctx context.Context,req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error){
	topology := &csi.Topology{
		Segments: map[string]string{TopologyKeyNode: c.nodeID},
	}
	return &csi.NodeGetInfoResponse{
		NodeId:             c.nodeID,
		MaxVolumesPerNode:  c.maxVolumesPerNode,
		AccessibleTopology: topology,
	}, nil
}

func (c *Node) NodePublishVolume(ctx context.Context,req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error){
	var (
		notMnt bool
		reterr error
	)
	// Check arguments
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability missing in request")
	}
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if len(req.GetTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	targetPath := req.GetTargetPath()
	volid := req.GetVolumeId()
	if req.GetVolumeCapability().GetMount() != nil {
		return nil, status.Error(codes.InvalidArgument, "only support mount access type")
	}

	alcub := c.alcubControl.GetByUuid(volid)
	if alcub == nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("not found resource by uuid %v",volid))
	}
	//prepare volume
	devpath, failedfn,successfn,err := c.preMountValid(alcub)
	if err!= nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if reterr!=nil{
			if failedfn!=nil{
				failedfn()
			}
			return
		}
		if successfn!= nil {
			successfn()
		}
	}()

	notMnt, reterr = mount.New("").IsLikelyNotMountPoint(targetPath)
	if reterr != nil {
		if os.IsNotExist(reterr) {
			if reterr = os.MkdirAll(targetPath, 0750); reterr != nil {
				return nil, status.Error(codes.Internal, reterr.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, reterr.Error())
		}
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	fsType := req.GetVolumeCapability().GetMount().GetFsType()

	readOnly := req.GetReadonly()
	attrib := req.GetVolumeContext()
	mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()

	klog.V(4).Infof("target %v\nfstype %v\nreadonly %v\nvolumeId %v\nattributes %v\nmountflags %v\n",
		targetPath, fsType, readOnly, volid, attrib, mountFlags)

	options := []string{"bind"}
	if readOnly {
		options = append(options, "ro")
	}
	safemounter:= mount.SafeFormatAndMount{
		Interface: mount.New(""),
		Exec:      utilexec.New(),
	}

	 reterr = safemounter.FormatAndMount(devpath, targetPath, "", options)
	 if reterr != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to mount device: %s at %s: %v", devpath, targetPath, reterr))
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (c *Node) NodeUnpublishVolume(ctx context.Context,req *csi.NodeUnpublishVolumeRequest) (resp *csi.NodeUnpublishVolumeResponse,reterr error){
	// Check arguments
	var (
		err error
	)
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if len(req.GetTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}
	targetPath := req.GetTargetPath()
	volumeID := req.GetVolumeId()

	alcub := c.alcubControl.GetByUuid(volumeID)
	if alcub == nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("not found resource by uuid %v",volumeID))
	}

	failedfn, successfn, err :=c.preUnmountValid(alcub)
	if err!=nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		if err!=nil{
			if failedfn!=nil{
				failedfn()
			}
			reterr = err
			return
		}
		if successfn!= nil {
			successfn()
		}
	}()
	// Unmount only if the target path is really a mount point.
	if notMnt, err := mount.IsNotMountPoint(mount.New(""), targetPath); err != nil {
		if !os.IsNotExist(err) {
			return nil, status.Error(codes.Internal, err.Error())
		}
		err = nil
	} else if !notMnt {
		// Unmounting the image or filesystem.
		err = mount.New("").Unmount(targetPath)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	// Delete the mount point.
	// Does not return error for non-existent path, repeated calls OK for idempotency.
	if err = os.RemoveAll(targetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	klog.V(4).Infof("hostpath: volume %s has been unpublished.", targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

//TODO support readwriteMany
func (c *Node) NodeStageVolume(ctx context.Context,req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	// Check arguments
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if len(req.GetStagingTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capability missing in request")
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (c *Node) NodeUnstageVolume(ctx context.Context,req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error){
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if len(req.GetStagingTargetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}
func (c *Node) NodeGetVolumeStats(ctx context.Context,req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error){
	return nil, status.Error(codes.Unimplemented, "")
}
func (c *Node) NodeExpandVolume(ctx context.Context,req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error){
	return nil, status.Error(codes.Unimplemented, "Not impl")
}