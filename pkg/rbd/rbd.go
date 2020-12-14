package rbd

import (
	"context"
	"fmt"

	"net"
	"time"

	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	klog "k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/volume"
	"sigs.k8s.io/sig-storage-lib-external-provisioner/v6/util"
)

var (
	supportedFeatures = sets.NewString("layering")
)

type rbdProvisionOptions struct {
	// Ceph monitors.
	monitors []string
	// Ceph RBD pool. Default is "rbd".
	pool string
	// Optional data pool for erasure pool support, default is ""
	dataPool string
	// Ceph client ID that is capable of creating images in the pool. Default is "admin".
	adminID string
	// Secret of admin client ID.
	adminSecret string
	// Ceph client ID that is used to map the RBD image. Default is the same as admin client ID.
	userID string
	// The name of Ceph Secret for userID to map RBD image. This parameter is required.
	userSecretName string
	// The namespace of Ceph Secret for userID to map RBD image. This parameter is optional.
	userSecretNamespace string
	// fsType that is supported by kubernetes. Default: "ext4".
	fsType string
	// Ceph RBD image format, "1" or "2". Default is "2".
	imageFormat string
	// This parameter is optional and should only be used if you set
	// imageFormat to "2". Currently supported features are layering only.
	// Default is "", and no features are turned on.
	imageFeatures []string
}

type Volume struct {
	Pool  string
	Image string
}

type Rbd struct {
	ctx    context.Context
	client kubernetes.Interface

	rbdutil RBDUtil

	dnsip string
}

// create/delete image function
// NOTE: Only support storageclass v1
func NewRbd(client kubernetes.Interface, createTimeout time.Duration) *Rbd {
	rbd := &Rbd{
		client:  client,
		rbdutil: NewRbdUtil(createTimeout),
	}
	return rbd
}

func (r *Rbd) parseParameters(parameters map[string]string) (*rbdProvisionOptions, error) {
	// options with default values
	opts := &rbdProvisionOptions{
		pool:        "rbd",
		dataPool:    "",
		adminID:     "admin",
		imageFormat: rbdImageFormat2,
	}

	var (
		err                  error
		adminSecretName      = ""
		adminSecretNamespace = "default"
	)

	for k, v := range parameters {
		switch strings.ToLower(k) {
		case "monitors":
			// Try to find DNS info in local cluster DNS so that the kubernetes
			// host DNS config doesn't have to know about cluster DNS
			if r.dnsip == "" {
				r.dnsip = util.FindDNSIP(context.Background(), r.client)
			}
			klog.V(4).Infof("dnsip: %q\n", r.dnsip)
			arr := strings.Split(v, ",")
			for _, m := range arr {
				mhost, mport := util.SplitHostPort(m)
				if r.dnsip != "" && net.ParseIP(mhost) == nil {
					var lookup []string
					if lookup, err = util.LookupHost(mhost, r.dnsip); err == nil {
						for _, a := range lookup {
							klog.V(1).Infof("adding %+v from mon lookup\n", a)
							opts.monitors = append(opts.monitors, util.JoinHostPort(a, mport))
						}
					} else {
						opts.monitors = append(opts.monitors, util.JoinHostPort(mhost, mport))
					}
				} else {
					opts.monitors = append(opts.monitors, util.JoinHostPort(mhost, mport))
				}
			}
			klog.V(4).Infof("final monitors list: %v\n", opts.monitors)
			if len(opts.monitors) < 1 {
				return nil, fmt.Errorf("missing Ceph monitors")
			}
		case "adminid":
			if v == "" {
				return nil, fmt.Errorf("missing Ceph adminid")
			}
			opts.adminID = v
		case "adminsecretname":
			adminSecretName = v
		case "adminsecretnamespace":
			adminSecretNamespace = v
		case "userid":
			opts.userID = v
		case "pool":
			if v == "" {
				return nil, fmt.Errorf("missing Ceph pool")
			}
			opts.pool = v
		case "datapool":
			opts.dataPool = v
		case "usersecretname":
			if v == "" {
				return nil, fmt.Errorf("missing user secret name")
			}
			opts.userSecretName = v
		case "usersecretnamespace":
			opts.userSecretNamespace = v
		case "imageformat":
			if v != rbdImageFormat1 && v != rbdImageFormat2 {
				return nil, fmt.Errorf("invalid ceph imageformat %s, expecting %s or %s", v, rbdImageFormat1, rbdImageFormat2)
			}
			opts.imageFormat = v
		case "imagefeatures":
			arr := strings.Split(v, ",")
			for _, f := range arr {
				if !supportedFeatures.Has(f) {
					return nil, fmt.Errorf("invalid feature %q, supported features are: %v", f, supportedFeatures)
				}
				opts.imageFeatures = append(opts.imageFeatures, f)
			}
		case volume.VolumeParameterFSType:
			opts.fsType = v
		default:
			return nil, fmt.Errorf("invalid option %q", k)
		}
	}

	// find adminSecret
	var secret string
	if adminSecretName == "" {
		return nil, fmt.Errorf("missing Ceph admin secret name")
	}
	if secret, err = r.parsePVSecret(adminSecretNamespace, adminSecretName); err != nil {
		return nil, fmt.Errorf("failed to get admin secret from [%q/%q]: %v", adminSecretNamespace, adminSecretName, err)
	}
	opts.adminSecret = secret

	// set user ID to admin ID if empty
	if opts.userID == "" {
		opts.userID = opts.adminID
	}

	return opts, nil
}

// parsePVSecret retrives secret value for a given namespace and name.
func (r *Rbd) parsePVSecret(namespace, secretName string) (string, error) {
	if r.client == nil {
		return "", fmt.Errorf("Cannot get kube client")
	}
	ctx := context.Background()
	secrets, err := r.client.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	// TODO: Should we check secret.Type, like `k8s.io/kubernetes/pkg/volume/util.GetSecretForPV` function?
	secret := ""
	for k, v := range secrets.Data {
		if k == secretKeyName {
			return string(v), nil
		}
		secret = string(v)
	}

	// If not found, the last secret in the map wins as done before
	return secret, nil
}

func (r *Rbd) CreateImage(scname string, image string, bytesize int64) (*Volume, error) {
	ctx := context.Background()
	sc, err := r.client.StorageV1().StorageClasses().Get(ctx, scname, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	rbdoption, err := r.parseParameters(sc.Parameters)
	if err != nil {
		return nil, err
	}
	return r.rbdutil.CreateImage(rbdoption, image, bytesize)
}

func (r *Rbd) DeleteImage(scname string, image string) error {
	ctx := context.Background()
	sc, err := r.client.StorageV1().StorageClasses().Get(ctx, scname, metav1.GetOptions{})
	if err != nil {
		return err
	}
	rbdoption, err := r.parseParameters(sc.Parameters)
	if err != nil {
		return err
	}
	return r.rbdutil.DeleteImage(rbdoption, image)
}
