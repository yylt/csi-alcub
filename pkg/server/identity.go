package server

import (
	"context"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type IdentityServer struct {
	caps          []*csi.PluginCapability
	name, version string
}

var (
	version = "1.0.0"
)

func ConstraCapability() csi.PluginCapability_Service_Type {
	return csi.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS
}

func ControllerCapability() csi.PluginCapability_Service_Type {
	return csi.PluginCapability_Service_CONTROLLER_SERVICE
}

var _ csi.IdentityServer = &IdentityServer{}

func NewIdenty(drivername string, caps ...csi.PluginCapability_Service_Type) (*IdentityServer, error) {
	if len(caps) == 0 {
		return nil, fmt.Errorf("Capability must have one at least")
	}
	var identcaps = make([]*csi.PluginCapability, len(caps))
	for i, v := range caps {
		identcaps[i] = &csi.PluginCapability{
			Type: &csi.PluginCapability_Service_{
				Service: &csi.PluginCapability_Service{
					Type: v,
				},
			},
		}
	}
	return &IdentityServer{
		caps:    identcaps,
		name:    drivername,
		version: version,
	}, nil
}

func (c *IdentityServer) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	if c.name == "" {
		return nil, status.Error(codes.Unavailable, "Driver name not configured")
	}

	if c.version == "" {
		return nil, status.Error(codes.Unavailable, "Driver is missing version")
	}

	return &csi.GetPluginInfoResponse{
		Name:          c.name,
		VendorVersion: c.version,
	}, nil
}

func (c *IdentityServer) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: c.caps,
	}, nil
}

func (c *IdentityServer) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	return &csi.ProbeResponse{}, nil
}
