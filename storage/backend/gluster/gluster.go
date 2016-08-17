package gluster

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	"github.com/contiv/errored"
	"github.com/contiv/executor"
	"github.com/contiv/volplugin/errors"
	"github.com/contiv/volplugin/storage"
	"github.com/contiv/volplugin/storage/mountscan"
)

const (
	// BackendName is string for gluster storage backend
	BackendName = "gluster"
)

// Driver implements a gluster storage driver for volplugin
type Driver struct {
	mountpath string
}

var spaceSplitRegex = regexp.MustCompile(`\s+`)

// NewMountDriver is a generator for Driver structs. It is used by the storage
// framework to yield new drivers on every creation.
func NewMountDriver(mountpath string) (storage.MountDriver, error) {
	return &Driver{mountpath: mountpath}, nil
}

// NewCRUDDriver is a generator for Driver structs. It is used by the storage
// framework to yield new drivers on every creation.
func NewCRUDDriver() (storage.CRUDDriver, error) {
	return &Driver{}, nil
}

// NewSnapshotDriver is a generator for Driver structs. It is used by the storage
// framework to yield new drivers on every creation.
func NewSnapshotDriver() (storage.SnapshotDriver, error) {
	return &Driver{}, nil
}

// Name returns the gluster backend string
func (d *Driver) Name() string {
	return BackendName
}

// XXX Utility functions

func (d *Driver) handleOptions(options interface{}) (map[string]string, error) {
	switch dType := options.(type) {
	case nil, string:
		if dType == nil || dType == "" {
			return map[string]string{}, nil
		}
	default:
		return nil, errored.Errorf("Cannot use %s as type string: %q", dType, "driver.options")
	}

	parts := strings.Split(options.(string), ",")
	mapOptions := map[string]string{}
	kvPattern := regexp.MustCompile("^((\\w)=(\\w))$")
	pattern := regexp.MustCompile("^(\\w)$")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if kvPattern.MatchString(part) {
			keyval := strings.Split(part, "=")
			mapOptions[keyval[0]] = keyval[1]
		} else if pattern.MatchString(part) {
			mapOptions[part] = ""
		} else {
			return nil, errored.Errorf("Cannot use %s as mount option, expected key=value", part)
		}
	}

	return mapOptions, nil
}

// this converts a hash of options into a string we pass to the mount syscall.
func (d *Driver) toString(mapOpts map[string]string) string {
	options := []string{}
	for key, val := range mapOpts {
		options = append(options, key+"="+val)
	}
	return strings.Join(options, ",")
}

func (d *Driver) mkOpts(do storage.DriverOptions) (string, error) {
	var options string
	if err := do.Volume.Params.Get("options", &options); err != nil {
		return "", err
	}

	opts, err := d.handleOptions(options)
	if err != nil {
		return "", err
	}
	return d.toString(opts), nil
}

func (d *Driver) stopVolume(volName string, timeout time.Duration) error {
	args := []string{"volume", "stop", volName, "force"}
	execRes, err := run(args, timeout)
	return processExecRes(execRes, err, fmt.Sprintf("Stopping volume %q", d.externalName(volName)))
}

func (d *Driver) setSnapConfig(volName string, activateOnCreate string, timeout time.Duration) error {
	args := []string{"snapshot", "config", volName, "activate-on-create", activateOnCreate}
	execRes, err := run(args, timeout)
	return processExecRes(execRes, err, fmt.Sprintf("Setting snap config for volume %q", d.externalName(volName)))
}

func (d *Driver) startVolume(volName string, timeout time.Duration) error {
	args := []string{"volume", "start", volName}
	execRes, err := run(args, timeout)
	return processExecRes(execRes, err, fmt.Sprintf("Starting volume %q", d.externalName(volName)))
}

func runCommand(args []string, timeout time.Duration) (*executor.ExecResult, error) {
	args = append([]string{"gluster", "--mode=script"}, args...)
	ctx, _ := context.WithTimeout(context.Background(), timeout)
	return executor.NewCapture(exec.Command("/bin/sh", "-c", strings.Join(args, " "))).Run(ctx)
}

func run(args []string, timeout time.Duration) (*executor.ExecResult, error) {
	args = append(args, "--mode=script")
	cmd := exec.Command("gluster", args...)
	ctx, _ := context.WithTimeout(context.Background(), timeout)
	return executor.NewCapture(cmd).Run(ctx)
}

func processExecRes(execRes *executor.ExecResult, err error, errorMsg string) error {
	if execRes != nil && execRes.ExitStatus != 0 {
		return errored.Errorf("%s: %#v", errorMsg, execRes)
	} else if err != nil {
		return errored.Errorf("%s: %#v", errorMsg, err)
	}
	return nil
}

// XXX CURD functions

// Create a volume
func (d *Driver) Create(do storage.DriverOptions) error {
	volName, err := d.internalName(do.Volume.Name)
	if err != nil {
		return err
	}

	var transport string
	err = do.Volume.Params.Get("transport", &transport)
	if err != nil {
		return err
	} else if isEmpty(transport) {
		transport = "tcp"
	}

	args := []string{"volume", "create", volName, "transport", transport}

	rawExecution := false
	var replica int
	err = do.Volume.Params.Get("replica", &replica)
	if err != nil {
		return err
	}

	if replica > 1 {
		rawExecution = true
		args = append(args, "replica", strconv.Itoa(replica))
	}

	var stripes int
	err = do.Volume.Params.Get("stripe", &stripes)
	if err != nil {
		return err
	}

	if stripes > 1 {
		rawExecution = true
		args = append(args, "stripe", strconv.Itoa(stripes))
	}

	var bricks map[string]string
	err = do.Volume.Params.Get("bricks", &bricks) // returns map[string]string
	if err != nil {
		return err
	}

	mBricks, err := marshalBricks(bricks)
	if err != nil {
		return err
	}

	args = append(args, mBricks)

	var execRes *executor.ExecResult
	if rawExecution {
		execRes, err = runCommand(args, do.Timeout)
	} else {
		execRes, err = run(args, do.Timeout)
	}

	if pErr := processExecRes(execRes, err, fmt.Sprintf("Creating volume %q", d.externalName(volName))); pErr != nil {
		return pErr
	}

	return d.startVolume(volName, do.Timeout)
}

// Destroy a volume (Delete)
func (d *Driver) Destroy(do storage.DriverOptions) error {
	volName, err := d.internalName(do.Volume.Name)
	if err != nil {
		return err
	}

	if err := d.stopVolume(volName, do.Timeout); err != nil {
		return err
	}

	args := []string{"volume", "delete", volName}
	execRes, err := run(args, do.Timeout)
	return processExecRes(execRes, err, fmt.Sprintf("Destroying volume %q", d.externalName(volName)))
}

// Format a volume
func (d *Driver) Format(do storage.DriverOptions) error {
	return nil // No-Op
}

// List all volumes
func (d *Driver) List(lo storage.ListOptions) ([]storage.Volume, error) {
	execRes, err := executor.NewCapture(exec.Command("gluster", "--mode=script", "volume", "info", "--xml")).Run(context.Background())
	if pErr := processExecRes(execRes, err, "Listing volumes"); pErr != nil {
		return nil, pErr
	}

	var glusterVolumes Volumes
	if err := xml.Unmarshal([]byte(execRes.Stdout), &glusterVolumes); err != nil {
		return nil, err
	}

	list := []storage.Volume{}
	for _, glusterVol := range glusterVolumes.List {
		list = append(list, storage.Volume{Name: d.externalName(strings.TrimSpace(glusterVol.Name)), Params: storage.DriverParams{"bricks": unmarshalBricks(glusterVol.Bricks)}})
	}

	return list, nil
}

// Exists - checks for the existence of the given volume
func (d *Driver) Exists(do storage.DriverOptions) (bool, error) {
	volName, err := d.internalName(do.Volume.Name)
	if err != nil {
		return false, err
	}

	args := []string{"volume", "info", volName, "--xml"}
	execRes, err := run(args, do.Timeout)
	if pErr := processExecRes(execRes, err, "Volume Exists?"); pErr != nil {
		return false, pErr
	}

	var glusterVolumes Volumes
	if err := xml.Unmarshal([]byte(execRes.Stdout), &glusterVolumes); err != nil {
		return false, err
	}

	for _, gVol := range glusterVolumes.List {
		if gVol.Name == volName {
			return true, nil
		}
	}

	return false, nil
}

// XXX Mounting functions

// Mount a volume
func (d *Driver) Mount(do storage.DriverOptions) (*storage.Mount, error) {
	/* Structure gluster volume as "IP/hostname:volname" */
	volName, err := d.internalName(do.Volume.Name)
	if err != nil {
		return nil, err
	}

	var bricks map[string]string
	if err = do.Volume.Params.Get("bricks", &bricks); err != nil {
		return nil, err
	}

	// server:volume, server- server IP of one of the volume bricks.
	glusterVolume := getVolumeServer(bricks) + ":" + volName // server:exportDir

	/* Volplugin mountpath */
	vpMountPath, err := d.MountPath(do)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(vpMountPath, 0755); err != nil && !os.IsExist(err) {
		return nil, errored.Errorf("Error creating directory %q while preparing GlusterFS mount for %q", vpMountPath, glusterVolume).Combine(err)
	}

	// Mount options
	opts, err := d.mkOpts(do)
	if err != nil {
		return nil, err
	}

	execRes, err := executor.NewCapture(exec.Command("mount", "-t", "glusterfs", glusterVolume, vpMountPath, "-o", opts)).Run(context.Background())
	if pErr := processExecRes(execRes, err, fmt.Sprintf("Mount Failed for volume %q", d.externalName(volName))); pErr != nil {
		return nil, pErr
	}

	return &storage.Mount{
		Device: glusterVolume,
		Path:   vpMountPath,
		Volume: do.Volume,
	}, nil
}

// Unmount a volume
func (d *Driver) Unmount(do storage.DriverOptions) error {
	vpMountPath, err := d.MountPath(do)
	if err != nil {
		return err
	}

	execRes, err := executor.NewCapture(exec.Command("umount", vpMountPath)).Run(context.Background())
	if pErr := processExecRes(execRes, err, fmt.Sprintf("Unmount Failed for volume %q", do.Volume.Name)); pErr != nil {
		return pErr
	}

	return nil
}

// Mounted describes all the volumes currently mapped on to the host.
func (d *Driver) Mounted(timeout time.Duration) ([]*storage.Mount, error) {
	mounts := []*storage.Mount{}
	hostMounts, err := mountscan.GetMounts(&mountscan.GetMountsRequest{DriverName: "glusterfs", FsType: "fuse.glusterfs"})
	if err != nil {
		if newerr, ok := err.(*errored.Error); ok && newerr.Contains(errors.ErrDevNotFound) {
			return mounts, nil
		}
		return nil, err
	}

	for _, hostMount := range hostMounts {
		rel, err := filepath.Rel(d.mountpath, hostMount.MountPoint)
		if err != nil {
			logrus.Errorf("Invalid volume calucated from mountpoint %q with mountpath %q", hostMount.MountPoint, d.mountpath)
			continue
		}

		mounts = append(mounts, &storage.Mount{
			DevMajor: hostMount.DeviceNumber.Major,
			DevMinor: hostMount.DeviceNumber.Minor,
			Path:     hostMount.MountPoint,
			Volume: storage.Volume{
				Name: rel,
				Params: map[string]interface{}{
					"mount": hostMount.MountSource,
				},
			},
		})
	}
	return mounts, nil
}

// MountPath describes the path at which the volume should be mounted. MountSource for docker `mount`
func (d *Driver) MountPath(do storage.DriverOptions) (string, error) {
	return path.Join(d.mountpath, do.Volume.Name), nil
}

// XXX Snapshot functions - Gluster supports snapshot on only thin-provisioned volumes

// CreateSnapshot creates a named snapshot for the volume. Any error will be returned.
func (d *Driver) CreateSnapshot(snapName string, do storage.DriverOptions) error {
	volName, err := d.internalName(do.Volume.Name)
	if err != nil {
		return err
	}

	sChars := map[string]string{".": "-", ":": "-", "+": "-", " ": ""}
	for sChar, replaceWith := range sChars {
		snapName = strings.Replace(snapName, sChar, replaceWith, -1)
	}

	args := []string{"snapshot", "create", snapName, volName, "no-timestamp", "description", fmt.Sprintf("Snapshot for volume %s", do.Volume.Name)}
	execRes, err := run(args, do.Timeout) // snap gets activated during creation
	if pErr := processExecRes(execRes, err, fmt.Sprintf("Creating snapshot %q of volume %q", snapName, do.Volume.Name)); pErr != nil {
		return pErr
	}

	args = []string{"snapshot", "activate", snapName}
	execRes, err = run(args, do.Timeout)
	if pErr := processExecRes(execRes, err, fmt.Sprintf("Activating snapshot %q of volume %q", snapName, do.Volume.Name)); pErr != nil {
		return pErr
	}

	return nil
}

// RemoveSnapshot removes a named snapshot for the volume.
func (d *Driver) RemoveSnapshot(snapName string, do storage.DriverOptions) error {
	args := []string{"snapshot", "delete"} // snaps call be deleted either using vol name or snap name
	if snapName == do.Volume.Name {
		volName, err := d.internalName(snapName)
		if err != nil {
			return err
		}

		args = append(args, "volume", volName)
	} else {
		args = append(args, snapName)
	}

	execRes, err := run(args, do.Timeout)
	if pErr := processExecRes(execRes, err, fmt.Sprintf("Deleting snapshot %q for volume %q", snapName, do.Volume.Name)); pErr != nil {
		return pErr
	}

	return nil
}

// ListSnapshots all the available snapshots of a given volume
func (d *Driver) ListSnapshots(do storage.DriverOptions) ([]string, error) {
	volName, err := d.internalName(do.Volume.Name)
	if err != nil {
		return nil, err
	}

	args := []string{"snapshot", "list", volName}
	execRes, err := run(args, do.Timeout)
	if pErr := processExecRes(execRes, err, fmt.Sprintf("Listing snapshots for volume %q", do.Volume.Name)); pErr != nil {
		return nil, pErr
	}

	snaps := []string{}
	result := strings.Split(execRes.Stdout, "\n")
	for _, line := range result {
		if isEmpty(line) || len(spaceSplitRegex.Split(strings.TrimSpace(line), -1)) > 1 { // No snapshots present
			continue
		}
		snaps = append(snaps, line)
	}

	return snaps, nil
}

// CopySnapshot copies a snapshot into a new volume
func (d *Driver) CopySnapshot(do storage.DriverOptions, snapName, newName string) error {
	targetVolName, err := d.internalName(newName)
	if err != nil {
		return err
	}

	list, err := d.List(storage.ListOptions{Params: do.Volume.Params})
	if err != nil {
		return err
	}

	for _, vol := range list {
		if targetVolName == vol.Name {
			return errored.Errorf("Volume %q already exists", vol.Name)
		}
	}

	// clone a snap to create new `targetVol`
	args := []string{"snapshot", "clone", targetVolName, snapName}
	execRes, err := run(args, do.Timeout)
	if pErr := processExecRes(execRes, err, fmt.Sprintf("Cloning snapshot %q to create volume %q", snapName, newName)); pErr != nil {
		return pErr
	}

	// On successful `targetVol` creation
	if err := d.startVolume(targetVolName, do.Timeout); err != nil {
		return errored.Errorf("%v", err).Combine(d.Destroy(storage.DriverOptions{Volume: storage.Volume{Name: newName}}))
	}

	return nil
}

// Validate validates the driver options to ensure they are compatible with the
// Ceph storage driver.
func (d *Driver) Validate(do *storage.DriverOptions) error {
	// XXX check this first to guard against nil pointers ahead of time.
	if err := do.Validate(); err != nil {
		return err
	}

	/* Validate `bricks` */
	var bricks map[string]string
	if err := do.Volume.Params.Get("bricks", &bricks); err != nil {
		return err
	}

	/* Validate `transport` */
	var transport string
	if err := do.Volume.Params.Get("transport", &transport); err != nil {
		return err
	}

	/* Validate `replica` */
	var replica int
	if err := do.Volume.Params.Get("replica", &replica); err != nil {
		return err
	}

	/* Validate `stripe` */
	var stripes int
	if err := do.Volume.Params.Get("stripe", &stripes); err != nil {
		return err
	}

	return nil
}
