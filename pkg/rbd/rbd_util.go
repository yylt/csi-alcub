package rbd

import (
	"context"
	"fmt"
	"net"

	"os/exec"
	"strings"
	"time"

	klog "k8s.io/klog/v2"

	"sigs.k8s.io/sig-storage-lib-external-provisioner/v6/util"
)

const (
	imageWatcherStr = "watcher="

	secretKeyName   = "key" // key name used in secret
	rbdImageFormat1 = "1"
	rbdImageFormat2 = "2"
)

var (
	defaultRbdUtil = NewRbdUtil(time.Second * 5)
)

type RBDUtil time.Duration

func NewRbdUtil(du time.Duration) RBDUtil {
	return RBDUtil(du)
}

// See https://github.com/kubernetes/kubernetes/pull/57512.
func (u RBDUtil) kernelRBDMonitorsOpt(mons []string) string {
	return strings.Join(mons, ",")
}

// CreateImage creates a new ceph image with provision and volume options.
func (u RBDUtil) CreateImage(pOpts *rbdProvisionOptions, image string, bytessize int64) (*Volume, error) {
	var output []byte
	var err error

	volSizeBytes := bytessize
	// convert to MB that rbd defaults on
	sz := int(util.RoundUpSize(volSizeBytes, util.MiB))
	if sz <= 0 {
		return nil, fmt.Errorf("invalid volume size '%d' requested for RBD provisioner, it must greater than zero", volSizeBytes)
	}
	volSz := fmt.Sprintf("%d", sz)
	// rbd create
	mon := u.kernelRBDMonitorsOpt(pOpts.monitors)
	if pOpts.imageFormat == rbdImageFormat2 {
		klog.V(4).Infof("rbd: create %s size %s format %s (features: %s) using mon %s, pool %s id %s key %s", image, volSz, pOpts.imageFormat, pOpts.imageFeatures, mon, pOpts.pool, pOpts.adminID, pOpts.adminSecret)
	} else {
		klog.V(4).Infof("rbd: create %s size %s format %s using mon %s, pool %s id %s key %s", image, volSz, pOpts.imageFormat, mon, pOpts.pool, pOpts.adminID, pOpts.adminSecret)
	}
	args := []string{"create", image, "--size", volSz, "--pool", pOpts.pool, "--id", pOpts.adminID, "-m", mon, "--key=" + pOpts.adminSecret, "--image-format", pOpts.imageFormat}
	if pOpts.dataPool != "" {
		args = append(args, "--data-pool", pOpts.dataPool)
	}
	if pOpts.imageFormat == rbdImageFormat2 {
		// if no image features is provided, it results in empty string
		// which disable all RBD image format 2 features as we expected
		features := strings.Join(pOpts.imageFeatures, ",")
		args = append(args, "--image-feature", features)
	}
	output, err = u.execCommand("rbd", args)
	if err != nil {
		klog.Warningf("failed to create rbd image, output %v", string(output))
		return nil, fmt.Errorf("failed to create rbd image: %v, command output: %s", err, string(output))
	}

	return &Volume{
		Pool:  pOpts.pool,
		Image: image,
	}, nil
}

// rbdStatus checks if there is watcher on the image.
// It returns true if there is a watcher onthe image, otherwise returns false.
func (u RBDUtil) rbdStatus(image string, pOpts *rbdProvisionOptions) (bool, error) {
	var err error
	var output string
	var cmd []byte

	mon := u.kernelRBDMonitorsOpt(pOpts.monitors)
	// cmd "rbd status" list the rbd client watch with the following output:
	//
	// # there is a watcher (exit=0)
	// Watchers:
	//   watcher=10.16.153.105:0/710245699 client.14163 cookie=1
	//
	// # there is no watcher (exit=0)
	// Watchers: none
	//
	// Otherwise, exit is non-zero, for example:
	//
	// # image does not exist (exit=2)
	// rbd: error opening image kubernetes-dynamic-pvc-<UUID>: (2) No such file or directory
	//
	klog.V(4).Infof("rbd: status %s using mon %s, pool %s id %s key %s", image, mon, pOpts.pool, pOpts.adminID, pOpts.adminSecret)
	args := []string{"status", image, "--pool", pOpts.pool, "-m", mon, "--id", pOpts.adminID, "--key=" + pOpts.adminSecret}
	cmd, err = u.execCommand("rbd", args)
	output = string(cmd)

	// If command never succeed, returns its last error.
	if err != nil {
		return false, err
	}

	if strings.Contains(output, imageWatcherStr) {
		klog.V(4).Infof("rbd: watchers on %s: %s", image, output)
		return true, nil
	}
	klog.Warningf("rbd: no watchers on %s", image)
	return false, nil
}

// DeleteImage deletes a ceph image with provision and volume options.
func (u RBDUtil) DeleteImage(pOpts *rbdProvisionOptions, image string) error {
	var output []byte
	found, err := u.rbdStatus(image, pOpts)
	if err != nil {
		return err
	}
	if found {
		klog.Info("rbd is still being used ", image)
		return fmt.Errorf("rbd %s is still being used", image)
	}
	// rbd rm
	mon := u.kernelRBDMonitorsOpt(pOpts.monitors)
	klog.V(4).Infof("rbd: rm %s using mon %s, pool %s id %s key %s", image, mon, pOpts.pool, pOpts.adminID, pOpts.adminSecret)
	args := []string{"rm", image, "--pool", pOpts.pool, "--id", pOpts.adminID, "-m", mon, "--key=" + pOpts.adminSecret}
	output, err = u.execCommand("rbd", args)
	if err == nil {
		return nil
	}
	klog.Errorf("failed to delete rbd image: %v, command output: %s", err, string(output))
	return err
}

func (u RBDUtil) FetchUrl(pool, attr string) ([]byte, error) {
	if pool == "" || attr == "" {
		return nil, fmt.Errorf("pool or attr not define")
	}
	args := []string{"-p", pool, "getxattr", attr, "URL"}
	return u.execCommand("rados", args)
}

func (u RBDUtil) BlackList(entityAddr string, id string, add bool) error {
	if entityAddr == "" || id == "" {
		return fmt.Errorf("pool or attr not define")
	}
	var (
		op string
	)
	if add {
		op = "add"
	} else {
		op = "rm"
	}
	args := []string{"--id", id, "osd", "blacklist", op, entityAddr}
	output, err := u.execCommand("ceph", args)
	if err == nil {
		return nil
	}
	klog.Errorf("failed to add blacklist: %v, command output: %s", err, string(output))
	return err
}

func (u RBDUtil) execCommand(command string, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(u))
	defer cancel()

	// Create the command with our context
	cmd := exec.CommandContext(ctx, command, args...)
	klog.V(4).Infof("Executing command: %v %v", command, args)
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("rbd: Command timed out")
	}

	// If there's no context error, we know the command completed (or errored).
	if err != nil {
		return nil, fmt.Errorf("rbd: Command exited with non-zero code: %v", err)
	}

	return out, err
}

//command: rados -p {pool} getxattr {attr} URL
func FetchUrl(pool, attr string) ([]byte, error) {
	return defaultRbdUtil.FetchUrl(pool, attr)
}

//commmand: ceph --id {id} osd blacklis add {ip}:0/0
func AddBlackList(storageip net.IP, id string) error {
	entityAddr := fmt.Sprintf("%s:0/0", storageip.String())
	return defaultRbdUtil.BlackList(entityAddr, id, true)
}

func RmBlackList(storageip net.IP, id string) error {
	entityAddr := fmt.Sprintf("%s:0/0", storageip.String())
	return defaultRbdUtil.BlackList(entityAddr, id, false)
}
