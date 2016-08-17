package gluster

import (
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"strings"
	. "testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/contiv/volplugin/storage"

	. "gopkg.in/check.v1"
)

const mountpath = "/mnt/gluster"

var distBricks = map[string]string{
	"mon0": "/brick",
	"mon1": "/brick",
	"mon2": "/brick",
}

var bricks = map[string]string{
	"mon0": "/brick",
}

var volumeSpecs = map[string]storage.Volume{
	"basic": {
		Name:   "test/pistachio",
		Params: storage.DriverParams{"bricks": bricks},
	},
	"dist": {
		Name:   "test/distPistachio",
		Params: storage.DriverParams{"bricks": distBricks},
	},
}

type glusterSuite struct{}

var _ = Suite(&glusterSuite{})

func TestGluster(t *T) { TestingT(t) }

func (s *glusterSuite) SetUpTest(c *C) {
	if os.Getenv("DEBUG") != "" {
		logrus.SetLevel(logrus.DebugLevel)
	}
}

func (s *glusterSuite) SetUpSuite(c *C) {
	// Delete all snapshots
	args := []string{"snapshot", "delete", "all", "--mode=script"}
	c.Assert(exec.Command("gluster", args...).Run(), IsNil)
}

func (s *glusterSuite) TearDownSuite(c *C) {
}

// Write a file and verify you can read it
func (s *glusterSuite) mountDirRWTest(c *C, mountDir string) {
	file, err := os.Create(mountDir + "/test.txt")
	c.Assert(err, IsNil)
	_, err = file.WriteString("Test string\n")
	c.Assert(err, IsNil)

	file.Close()

	file, err = os.Open(mountDir + "/test.txt")
	c.Assert(err, IsNil)

	rb := make([]byte, 11)
	_, err = io.ReadAtLeast(file, rb, 11)
	c.Assert(err, IsNil)

	file.Close()

	var rbs = strings.TrimSpace(string(rb))
	c.Assert(rbs, Equals, strings.TrimSpace("Test string"))
}

func (s *glusterSuite) TestMountUnmountVolume(c *C) {
	crudDrv, err := NewCRUDDriver()
	c.Assert(err, IsNil)
	mountDrv, err := NewMountDriver(mountpath)
	c.Assert(err, IsNil)

	for _, volSpec := range volumeSpecs {
		driverOpts := storage.DriverOptions{
			Volume:  volSpec,
			Timeout: 5 * time.Second,
		}

		// Incase of failure, we want to clear-up things
		defer mountDrv.Unmount(driverOpts)
		defer crudDrv.Destroy(driverOpts)

		iterations := 3
		for i := 0; i < iterations; i++ {
			c.Assert(crudDrv.Create(driverOpts), IsNil)

			ms, err := mountDrv.Mount(driverOpts)
			c.Assert(err, IsNil)
			c.Assert(ms.Volume, DeepEquals, driverOpts.Volume)

			mp, err := mountDrv.MountPath(driverOpts)
			c.Assert(err, IsNil)
			s.mountDirRWTest(c, mp)

			c.Assert(mountDrv.Unmount(driverOpts), IsNil)
			c.Assert(crudDrv.Destroy(driverOpts), IsNil)
		}
	}
}

// Tests the validity of the different volume names
func (s *glusterSuite) TestExternalInternalNames(c *C) {
	driver := &Driver{}
	out, err := driver.internalName("tenant1/test")
	c.Assert(err, IsNil)
	c.Assert(out, Equals, "tenant1_test")

	out, err = driver.internalName("tenant1.test/test")
	c.Assert(err, NotNil)
	c.Assert(out, Equals, "")

	out, err = driver.internalName("tenant1/test.two")
	c.Assert(err, NotNil)
	c.Assert(out, Equals, "")

	out, err = driver.internalName("tenant1/test/two")
	c.Assert(err, NotNil)
	c.Assert(out, Equals, "")

	out, err = driver.internalName("tenant1/test")
	c.Assert(driver.externalName(out), Equals, "tenant1/test")
	c.Assert(err, IsNil)
}

func (s *glusterSuite) TestMounted(c *C) {
	crudDrv, err := NewCRUDDriver()
	c.Assert(err, IsNil)
	mountDrv, err := NewMountDriver(mountpath)
	c.Assert(err, IsNil)

	for _, volSpec := range volumeSpecs {
		driverOpts := storage.DriverOptions{
			Volume:  volSpec,
			Timeout: 2 * time.Minute,
		}

		defer mountDrv.Unmount(driverOpts)
		defer crudDrv.Destroy(driverOpts)

		c.Assert(crudDrv.Create(driverOpts), IsNil)
		c.Assert(crudDrv.Format(driverOpts), IsNil) // No-op

		_, err = mountDrv.Mount(driverOpts)
		c.Assert(err, IsNil)
		mounts, err := mountDrv.Mounted(2 * time.Minute)
		c.Assert(err, IsNil)

		gVolName := strings.Replace(driverOpts.Volume.Name, "/", "_", -1)

		foundMntSrc := false
		for server := range driverOpts.Volume.Params["bricks"].(map[string]string) {
			mountSource := server + ":" + gVolName
			if mountSource == (*mounts[0]).Volume.Params["mount"].(string) {
				foundMntSrc = true
				break
			}
		}

		c.Assert(foundMntSrc, Equals, true)

		// Verify the results
		c.Assert((*mounts[0]).Path, Equals, path.Join(mountpath, driverOpts.Volume.Name)) // MountPoint
		c.Assert((*mounts[0]).Volume.Name, Equals, driverOpts.Volume.Name)                // Volume Name

		c.Assert(mountDrv.Unmount(driverOpts), IsNil)
		c.Assert(crudDrv.Destroy(driverOpts), IsNil)
	}
}

func (s *glusterSuite) TestRepeatedMountUnmount(c *C) {
	mountDrv, err := NewMountDriver(mountpath)
	c.Assert(err, IsNil)
	crudDrv, err := NewCRUDDriver()
	c.Assert(err, IsNil)

	for _, volSpec := range volumeSpecs {
		driverOpts := storage.DriverOptions{
			Volume:  volSpec,
			Timeout: 5 * time.Second,
		}

		defer mountDrv.Unmount(driverOpts)
		defer crudDrv.Destroy(driverOpts)

		c.Assert(crudDrv.Create(driverOpts), IsNil)
		c.Assert(crudDrv.Format(driverOpts), IsNil) // No-op

		iterations := 10
		for i := 0; i < iterations; i++ {
			_, err := mountDrv.Mount(driverOpts)
			c.Assert(err, IsNil)

			mp, err := mountDrv.MountPath(driverOpts)
			c.Assert(err, IsNil)
			s.mountDirRWTest(c, mp)

			c.Assert(mountDrv.Unmount(driverOpts), IsNil)
		}

		c.Assert(crudDrv.Destroy(driverOpts), IsNil)
	}
}

func (s *glusterSuite) TestGlusterVolumeTypes(c *C) {
	crudDrv, err := NewCRUDDriver()
	c.Assert(err, IsNil)

	driverOpts := storage.DriverOptions{
		Volume:  volumeSpecs["basic"], // has only one brick; so it cannot specify replica>1 or stripes >1
		Timeout: 5 * time.Second,
	}

	// Gluster create options: stripe, replica, transport
	driverOpts.Volume.Params["replica"] = 2 // number of bricks is not a multiple of replica count
	err = crudDrv.Create(driverOpts)
	c.Assert(err, ErrorMatches, ".*number of bricks is not a multiple of replica count.*")

	driverOpts.Volume.Params["replica"] = 0 // this will be ignored as part of the vlidation
	driverOpts.Volume.Params["stripe"] = 2  // number of bricks is not a multiple of stripe count
	err = crudDrv.Create(driverOpts)
	c.Assert(err, ErrorMatches, ".*number of bricks is not a multiple of stripe count.*")

	driverOpts.Volume.Params["replica"] = 2 //number of bricks given doesn't match required count
	driverOpts.Volume.Params["stripe"] = 2
	err = crudDrv.Create(driverOpts)
	c.Assert(err, ErrorMatches, ".*number of bricks given doesn't match required count.*")

	// Resetting values
	driverOpts.Volume.Params["replica"] = 0
	driverOpts.Volume.Params["stripe"] = 0

	// Distributed volume > 1 bricks : 3
	driverOpts.Volume = volumeSpecs["dist"]

	// Replicated Volume
	driverOpts.Volume.Params["replica"] = 3 // Volume replicated across bricks in the volume
	c.Assert(crudDrv.Create(driverOpts), IsNil)
	c.Assert(crudDrv.Destroy(driverOpts), IsNil)

	// Striped Volume
	driverOpts.Volume.Params["replica"] = 0
	driverOpts.Volume.Params["stripe"] = 3 // Volume striped across bricks in the volume
	c.Assert(crudDrv.Create(driverOpts), IsNil)
	c.Assert(crudDrv.Destroy(driverOpts), IsNil)

	// Striped-Replcated volume : requires 9 bricks
	driverOpts.Volume.Params["replica"] = 3
	driverOpts.Volume.Params["stripe"] = 3
	err = crudDrv.Create(driverOpts)
	c.Assert(err, ErrorMatches, ".*number of bricks given doesn't match required count.*")

	// Reset values
	driverOpts.Volume.Params["replica"] = 0
	driverOpts.Volume.Params["stripe"] = 0

	// Dispersed Volume : stripes the encoded data of files, with some redundancy addedd, across multiple bricks in the volume

	// XXX Below can be done only with a minimum of 4 nodes
	// Distributed Replicated : replicates data across two or more nodes in a cluster
	// Distributed Striped : stripes data across two or more nodes in a cluster
	// Distributed Striped Replicated Volumes : distributes striped data across replicated bricks in the cluster
}

func (s *glusterSuite) TestSnapshots(c *C) {
	snapDrv, err := NewSnapshotDriver()
	c.Assert(err, IsNil)
	crudDrv, err := NewCRUDDriver()
	c.Assert(err, IsNil)

	for _, volSpec := range volumeSpecs {
		driverOpts := storage.DriverOptions{
			Volume:  volSpec,
			Timeout: 5 * time.Second,
		}

		defer snapDrv.RemoveSnapshot("hello", driverOpts)
		defer crudDrv.Destroy(driverOpts)

		c.Assert(crudDrv.Create(driverOpts), IsNil)
		c.Assert(snapDrv.CreateSnapshot("hello", driverOpts), IsNil)
		c.Assert(snapDrv.CreateSnapshot("hello", driverOpts), NotNil)

		list, err := snapDrv.ListSnapshots(driverOpts)
		c.Assert(err, IsNil)
		c.Assert(len(list), Equals, 1)
		c.Assert(list, DeepEquals, []string{"hello"})

		c.Assert(snapDrv.RemoveSnapshot("hello", driverOpts), IsNil)
		c.Assert(snapDrv.RemoveSnapshot("hello", driverOpts), NotNil)

		list, err = snapDrv.ListSnapshots(driverOpts)
		c.Assert(err, IsNil)
		c.Assert(len(list), Equals, 0)
		c.Assert(crudDrv.Destroy(driverOpts), IsNil)
	}
}

func (s *glusterSuite) TestSnapshotsClone(c *C) {
	snapDrv, err := NewSnapshotDriver()
	c.Assert(err, IsNil)
	crudDrv, err := NewCRUDDriver()
	c.Assert(err, IsNil)

	for _, volSpec := range volumeSpecs {
		driverOpts := storage.DriverOptions{
			Volume:  volSpec,
			Timeout: 5 * time.Second,
		}

		defer snapDrv.RemoveSnapshot("test", driverOpts)
		defer snapDrv.RemoveSnapshot("testsnap", driverOpts)
		defer crudDrv.Destroy(driverOpts)

		volName := driverOpts.Volume.Name
		newVolName := "test" + "/" + getRandomStr(5)

		c.Assert(crudDrv.Create(driverOpts), IsNil)
		c.Assert(snapDrv.CreateSnapshot("test", driverOpts), IsNil)
		c.Assert(snapDrv.CreateSnapshot("testsnap", driverOpts), IsNil)
		c.Assert(snapDrv.CopySnapshot(driverOpts, "testsnap", newVolName), IsNil)
		c.Assert(snapDrv.CopySnapshot(driverOpts, "test", newVolName), NotNil)
		c.Assert(snapDrv.CopySnapshot(driverOpts, "foo", newVolName), NotNil)
		c.Assert(snapDrv.CopySnapshot(driverOpts, "testsnap", newVolName), NotNil)

		driverOpts.Volume.Name = newVolName
		volExists, err := crudDrv.Exists(driverOpts)
		c.Assert(err, IsNil)
		c.Assert(volExists, Equals, true)

		volumes, err := crudDrv.List(storage.ListOptions{Params: driverOpts.Volume.Params}) // original + cloned vol
		c.Assert(err, IsNil)
		c.Assert(len(volumes) >= 2, Equals, true)

		c.Assert(snapDrv.RemoveSnapshot("test", driverOpts), IsNil)
		c.Assert(snapDrv.RemoveSnapshot("testsnap", driverOpts), IsNil)

		c.Assert(crudDrv.Destroy(driverOpts), IsNil) // delete the cloned vol.
		driverOpts.Volume.Name = volName
		c.Assert(crudDrv.Destroy(driverOpts), IsNil) // delete original vol.
	}
}

// Test repeated clone of the same volume
func (s *glusterSuite) TestRepeatedClone(c *C) {
	snapDrv, err := NewSnapshotDriver()
	c.Assert(err, IsNil)
	crudDrv, err := NewCRUDDriver()
	c.Assert(err, IsNil)
	for _, volSpec := range volumeSpecs {
		driverOpts := storage.DriverOptions{
			Volume:  volSpec,
			Timeout: 5 * time.Second,
		}

		defer crudDrv.Destroy(driverOpts)
		defer snapDrv.RemoveSnapshot("test", driverOpts) // all snapshots of this volume will be deleted

		c.Assert(crudDrv.Create(driverOpts), IsNil)
		c.Assert(snapDrv.CreateSnapshot("test", driverOpts), IsNil)

		origVolName := driverOpts.Volume.Name

		iterations := 3
		newVolumes := []string{}
		for i := 0; i < iterations; i++ {
			newVolName := "test" + "/" + getRandomStr(5)
			c.Assert(snapDrv.CopySnapshot(driverOpts, "test", newVolName), IsNil)

			newVolumes = append(newVolumes, newVolName)

			driverOpts.Volume.Name = newVolName
			volExists, err := crudDrv.Exists(driverOpts)
			c.Assert(err, IsNil)
			c.Assert(volExists, Equals, true)
		}

		volumes, err := crudDrv.List(storage.ListOptions{Params: driverOpts.Volume.Params})
		c.Assert(err, IsNil)
		c.Assert(len(volumes) >= iterations+1, Equals, true)

		// Delete all cloned vols
		for _, vol := range newVolumes {
			driverOpts.Volume.Name = vol
			c.Assert(crudDrv.Destroy(driverOpts), IsNil)
		}

		driverOpts.Volume.Name = origVolName
		c.Assert(snapDrv.RemoveSnapshot(driverOpts.Volume.Name, driverOpts), IsNil)
		c.Assert(crudDrv.Destroy(driverOpts), IsNil) // delete original vol.
	}
}

func getRandomStr(strlen int) string {
	charSet := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	randStr := make([]byte, 0, strlen)
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < strlen; i++ {
		randStr = append(randStr, charSet[rand.Int()%len(charSet)])
	}
	return string(randStr)
}
