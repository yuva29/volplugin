package systemtests

import (
	"strings"
	"time"

	"github.com/Sirupsen/logrus"

	. "gopkg.in/check.v1"
)

func (s *systemtestSuite) TestVolsupervisorSnapLockedVolume(c *C) {
	if !cephDriver() && !glusterDriver() {
		c.Skip("Only ceph/gluster supports snapshots")
		return
	}

	_, err := s.uploadIntent("policy1", "snaplockedvol")
	c.Assert(err, IsNil)

	volName := genRandomVolume()
	fqVolName := fqVolume("policy1", volName)
	c.Assert(s.createVolume("mon0", fqVolName, nil), IsNil) // locked volume
	_, err = s.dockerRun("mon0", true, true, fqVolName, "/bin/sleep 10m")
	c.Assert(err, IsNil)

	prevCount := 0
	for count := 0; count < 5; count++ {
		time.Sleep(4 * time.Second) // buffer time

		count := 0
		if cephDriver() {
			out, err := s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo rbd snap ls policy1." + volName)
			c.Assert(err, IsNil)
			count = len(strings.Split(strings.TrimSpace(out), "\n"))
			c.Assert(count >= prevCount, Equals, true, Commentf("%v", out))
		} else if glusterDriver() {
			out, err := s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo gluster snapshot list policy1_" + volName)
			c.Assert(err, IsNil)
			count = len(strings.Split(strings.TrimSpace(out), "\n"))
			c.Assert(count >= prevCount, Equals, true, Commentf("%v", out))
		}

		prevCount = count
	}
}

func (s *systemtestSuite) TestVolsupervisorSnapshotSchedule(c *C) {
	if !cephDriver() && !glusterDriver() {
		c.Skip("Only ceph/gluster supports snapshots")
		return
	}

	_, err := s.uploadIntent("policy1", "fastsnap")
	c.Assert(err, IsNil)

	volName := genRandomVolume()

	c.Assert(s.createVolume("mon0", fqVolume("policy1", volName), map[string]string{"unlocked": "true"}), IsNil)

	time.Sleep(6 * time.Second)

	if cephDriver() {
		out, err := s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo rbd snap ls policy1." + volName)
		c.Assert(err, IsNil)
		c.Assert(len(strings.Split(strings.TrimSpace(out), "\n"))-1 >= 2, Equals, true)
	} else if glusterDriver() {
		out, err := s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo gluster snapshot list policy1_" + volName)
		c.Assert(err, IsNil)
		logrus.Infof("%#v", out)
		logrus.Infof("%v", len(strings.Split(strings.TrimSpace(out), "\n")))
		c.Assert(len(strings.Split(strings.TrimSpace(out), "\n")) >= 2, Equals, true)

	}
	time.Sleep(15 * time.Second)

	if cephDriver() {
		out, err := s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo rbd snap ls policy1." + volName)
		c.Assert(err, IsNil)
		mylen := len(strings.Split(strings.TrimSpace(out), "\n"))
		c.Assert(mylen, Not(Equals), 0)
		// this is 11 because in rare cases, the snapshot pruner will have not run yet when this is counted.
		c.Assert(mylen >= 5 && mylen <= 11, Equals, true, Commentf("len: %d\n%v", mylen, out))
	} else if glusterDriver() {
		out, err := s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo gluster snapshot list policy1_" + volName)
		c.Assert(err, IsNil)
		mylen := len(strings.Split(strings.TrimSpace(out), "\n")) - 1
		c.Assert(mylen, Not(Equals), 0)
		c.Assert(mylen >= 5 && mylen <= 11, Equals, true, Commentf("len: %d\n%v", mylen, out))
	}

}

func (s *systemtestSuite) TestVolsupervisorStopStartSnapshot(c *C) {
	if !cephDriver() && !glusterDriver() {
		c.Skip("Only ceph/gluster supports snapshots")
		return
	}

	_, err := s.uploadIntent("policy1", "fastsnap")
	c.Assert(err, IsNil)

	volName := genRandomVolume()
	fqVolName := fqVolume("policy1", volName)

	c.Assert(s.createVolume("mon0", fqVolName, map[string]string{"unlocked": "true"}), IsNil)

	time.Sleep(6 * time.Second)

	if cephDriver() {
		out, err := s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo rbd snap ls policy1." + volName)
		c.Assert(err, IsNil)
		c.Assert(len(strings.Split(out, "\n")) > 2, Equals, true)
	} else if glusterDriver() {
		out, err := s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo gluster snapshot list policy1_" + volName)
		c.Assert(err, IsNil)
		c.Assert(len(strings.Split(out, "\n")) > 2, Equals, true)
	}

	out, err := s.volcli("volume remove " + fqVolName)
	c.Assert(err, IsNil, Commentf(out))

	if cephDriver() {
		_, err = s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo rbd snap ls policy1." + volName)
		c.Assert(err, NotNil)
	} else if glusterDriver() {
		out, err := s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo gluster snapshot list policy1_" + volName)
		logrus.Infof("%v", out)
		c.Assert(err, IsNil)
	}

	_, err = s.uploadIntent("policy1", "nosnap")
	c.Assert(err, IsNil)

	// XXX we don't use createVolume here because of a bug in docker that doesn't
	// allow it to create the same volume twice
	_, err = s.volcli("volume create " + fqVolName + " --opt unlocked=true")
	c.Assert(err, IsNil)

	time.Sleep(6 * time.Second)

	if cephDriver() {
		out, err := s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo rbd snap ls policy1." + volName)
		c.Assert(err, IsNil)
		c.Assert(len(out), Equals, 0)
	} else if glusterDriver() {
		out, err := s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo gluster snapshot list policy1_" + volName)
		c.Assert(err, IsNil)
		c.Assert(len(out), Equals, 0)
	}

}

func (s *systemtestSuite) TestVolsupervisorRestart(c *C) {
	if !cephDriver() && !glusterDriver() {
		c.Skip("Only ceph/gluster supports snapshots")
		return
	}

	var err error
	_, err = s.uploadIntent("policy1", "fastsnap")
	c.Assert(err, IsNil)

	volName := genRandomVolume()
	fqVolName := fqVolume("policy1", volName)
	c.Assert(s.createVolume("mon0", fqVolName, map[string]string{"unlocked": "true"}), IsNil)

	time.Sleep(30 * time.Second)

	var out string
	if cephDriver() {
		out, err = s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo rbd snap ls policy1." + volName)
		c.Assert(err, IsNil)
		c.Assert(strings.Count(out, "\n") > 1, Equals, true, Commentf("%v", out))
	} else if glusterDriver() {
		out, err = s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo gluster snapshot list policy1_" + volName)
		c.Assert(err, IsNil)
		c.Assert(strings.Count(out, "\n") > 1, Equals, true, Commentf("%v", out))
	}

	c.Assert(stopVolsupervisor(s.vagrant.GetNode("mon0")), IsNil)
	c.Assert(startVolsupervisor(s.vagrant.GetNode("mon0")), IsNil)
	c.Assert(waitForVolsupervisor(s.vagrant.GetNode("mon0")), IsNil)

	time.Sleep(time.Minute)

	if cephDriver() {
		out2, err := s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo rbd snap ls policy1." + volName)
		c.Assert(err, IsNil)
		c.Assert(out, Not(Equals), out2)
	} else if glusterDriver() {
		out2, err := s.vagrant.GetNode("mon0").RunCommandWithOutput("sudo gluster snapshot list policy1_" + volName)
		c.Assert(err, IsNil)
		c.Assert(out, Not(Equals), out2)
	}

}

func (s *systemtestSuite) TestVolsupervisorSignal(c *C) {
	if !cephDriver() && !glusterDriver() {
		c.Skip("Only ceph/gluster supports snapshots")
		return
	}

	_, err := s.uploadIntent("policy1", "nosnap")
	c.Assert(err, IsNil)

	volName := genRandomVolume()
	fqVolName := fqVolume("policy1", volName)

	c.Assert(s.createVolume("mon0", fqVolName, map[string]string{"unlocked": "true"}), IsNil)
	_, err = s.volcli("volume snapshot take " + fqVolName)
	c.Assert(err, IsNil)

	time.Sleep(5 * time.Second)

	out, err := s.volcli("volume snapshot list " + fqVolName)
	c.Assert(err, IsNil)
	c.Assert(len(strings.TrimSpace(out)), Not(Equals), 0, Commentf(out))
}

func (s *systemtestSuite) TestVolsupervisorStartLock(c *C) {
	// this fails because it's already running on mon0 because of the rebootstrap call.
	c.Assert(s.vagrant.GetNode("mon1").RunCommand("sudo volsupervisor"), NotNil)
	defer s.vagrant.GetNode("mon1").RunCommand("docker kill volsupervisor")
}
