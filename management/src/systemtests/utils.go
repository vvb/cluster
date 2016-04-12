// +build systemtest

package systemtests

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/contiv/cluster/management/src/boltdb"
	tutils "github.com/contiv/systemtests-utils"
	"github.com/contiv/vagrantssh"
	. "gopkg.in/check.v1"
)

// XXX: go-check doesn't pass the test's state to the test set/teardown functions.
// So we have no way to know if a test failed and take some approrpate action.
// This hack let's me do that for now.
func (s *CliTestSuite) Assert(c *C, obtained interface{}, checker Checker, args ...interface{}) {
	if c.Check(obtained, checker, args...) == false {
		s.failed = true
		c.FailNow()
	}
}

func (s *CliTestSuite) assertMatch(c *C, exptd, rcvd string) {
	// XXX: the `Matches` checker doesn't match the expression in a multi-line
	// output so resorting to a regex check here.
	if match, err := regexp.MatchString(exptd, rcvd); err != nil || !match {
		s.Assert(c, false, Equals, true, Commentf("output: %s", rcvd))
	}
}

func (s *CliTestSuite) assertMultiMatch(c *C, exptd, rcvd string, eMatchCount int) {
	r := regexp.MustCompile(fmt.Sprintf("(?m)%s", exptd))
	s.Assert(c, len(r.FindAllString(rcvd, -1)), Equals, eMatchCount, Commentf("output: %s", rcvd))
}

func (s *CliTestSuite) startSerf(c *C, nut vagrantssh.TestbedNode) {
	out, err := tutils.ServiceStartAndWaitForUp(nut, "serf", 30)
	s.Assert(c, err, IsNil, Commentf("output: %s", out))
	c.Logf("serf is running. %s", out)
}

func (s *CliTestSuite) stopSerf(c *C, nut vagrantssh.TestbedNode) {
	out, err := tutils.ServiceStop(nut, "serf")
	s.Assert(c, err, IsNil, Commentf("output: %s", out))
	c.Logf("serf is stopped. %s", out)
}

func (s *CliTestSuite) restartClusterm(c *C, nut vagrantssh.TestbedNode, timeout int) {
	out, err := tutils.ServiceRestartAndWaitForUp(nut, "clusterm", timeout)
	s.Assert(c, err, IsNil, Commentf("output: %s", out))
	c.Logf("clusterm is running. %s", out)
}

func (s *CliTestSuite) nukeNodeInCollins(c *C, nodeName string) {
	// Ignore errors here as asset might not exist.
	out, err := s.tbn1.RunCommandWithOutput(fmt.Sprintf(`curl --basic -u blake:admin:first -d status="Decommissioned" -d reason="test" -X POST http://localhost:9000/api/asset/%s`, nodeName))
	c.Logf("collins asset decommission result: %v. Output: %s", err, out)
	out, err = s.tbn1.RunCommandWithOutput(fmt.Sprintf(`curl --basic -u blake:admin:first -d reason=test -X DELETE http://localhost:9000/api/asset/%s`, nodeName))
	c.Logf("collins asset deletion result: %v. Output: %s", err, out)
}

func (s *CliTestSuite) nukeNodesInBoltdb(c *C) {
	dbfile := boltdb.DefaultConfig().DBFile
	out, err := s.tbn1.RunCommandWithOutput(fmt.Sprintf("sudo rm -f %s", dbfile))
	c.Logf("boltdb asset deletion result: %v. Output: %s", err, out)
}

func (s *CliTestSuite) nukeNodesInInventory(c *C) {
	// XXX: we cleanup up assets from collins instead of restarting it to save test time.
	for _, name := range validNodeNames {
		s.nukeNodeInCollins(c, name)
	}
	s.nukeNodesInBoltdb(c)
}

func (s *CliTestSuite) checkProvisionStatus(c *C, tbn1 vagrantssh.TestbedNode, nodeName, exptdStatus string) {
	exptdStr := fmt.Sprintf(`.*"status".*"%s".*`, exptdStatus)
	out, err := tutils.WaitForDone(func() (string, bool) {
		cmdStr := fmt.Sprintf("clusterctl node get %s", nodeName)
		out, err := tbn1.RunCommandWithOutput(cmdStr)
		if err != nil {
			return out, false
			//replace newline with empty string for regex to match properly
		} else if match, err := regexp.MatchString(exptdStr,
			strings.Replace(out, "\n", "", -1)); err == nil && match {
			return out, true
		}
		return out, false
	}, 1*time.Second, 30*time.Second, fmt.Sprintf("node is still not in %q status", exptdStatus))
	s.Assert(c, err, IsNil, Commentf("output: %s", out))
}

func (s *CliTestSuite) waitForStatToSucceed(c *C, nut vagrantssh.TestbedNode, file string) {
	out, err := tutils.WaitForDone(func() (string, bool) {
		cmdStr := fmt.Sprintf("stat -t %s", file)
		out, err := nut.RunCommandWithOutput(cmdStr)
		if err != nil {
			return out, false
		}
		return out, true
	}, 1*time.Second, 10*time.Second, fmt.Sprintf("file %q still doesn't seems to exist", file))
	s.Assert(c, err, IsNil, Commentf("output: %s", out))
}

func (s *CliTestSuite) waitForStatToFail(c *C, nut vagrantssh.TestbedNode, file string) {
	out, err := tutils.WaitForDone(func() (string, bool) {
		cmdStr := fmt.Sprintf("stat -t %s", file)
		out, err := nut.RunCommandWithOutput(cmdStr)
		if err == nil {
			return out, false
		}
		return out, true
	}, 1*time.Second, 10*time.Second, fmt.Sprintf("file %q still seems to exist", file))
	s.Assert(c, err, IsNil, Commentf("output: %s", out))
}

func (s *CliTestSuite) waitForSerfMembership(c *C, nut vagrantssh.TestbedNode, nodeName, state string) {
	out, err := tutils.WaitForDone(func() (string, bool) {
		out, err := nut.RunCommandWithOutput(`serf members`)
		if err != nil {
			return out, false
		}
		stateStr := fmt.Sprintf(`%s.*%s.*`, nodeName, state)
		if match, err := regexp.MatchString(stateStr, out); err != nil || !match {
			return out, false
		}
		return out, true
	}, 1*time.Second, time.Duration(10)*time.Second,
		fmt.Sprintf("%s's serf membership is not in %s state", nodeName, state))
	s.Assert(c, err, IsNil, Commentf("output: %s", out))
}

func (s *CliTestSuite) commissionNode(c *C, nodeName string, nut vagrantssh.TestbedNode) {
	// provision the node
	cmdStr := fmt.Sprintf("clusterctl node commission %s", nodeName)
	out, err := s.tbn1.RunCommandWithOutput(cmdStr)
	s.Assert(c, err, IsNil, Commentf("output: %s", out))
	s.checkProvisionStatus(c, s.tbn1, nodeName, "Allocated")

	// verify that site.yml got executed on the node and created the dummy file
	s.waitForStatToSucceed(c, nut, dummyAnsibleFile)
}

func (s *CliTestSuite) decommissionNode(c *C, nodeName string, nut vagrantssh.TestbedNode) {
	// decommission the node
	cmdStr := fmt.Sprintf("clusterctl node decommission %s", nodeName)
	out, err := s.tbn1.RunCommandWithOutput(cmdStr)
	s.Assert(c, err, IsNil, Commentf("output: %s", out))
	s.checkProvisionStatus(c, s.tbn1, nodeName, "Decommissioned")

	// verify that cleanup.yml got executed on the node and deleted the dummy file
	s.waitForStatToFail(c, nut, dummyAnsibleFile)
}

func (s *CliTestSuite) getNodeInfoFailureNonExistentNode(c *C, nodeName string) {
	cmdStr := fmt.Sprintf(`clusterctl node get %s`, nodeName)
	out, err := s.tbn1.RunCommandWithOutput(cmdStr)
	s.Assert(c, err, NotNil, Commentf("output: %s", out))
	exptdOut := fmt.Sprintf(`.*node with name.*%s.*doesn't exists.*`, nodeName)
	s.assertMatch(c, exptdOut, out)
}

func (s *CliTestSuite) getNodeInfoSuccess(c *C, nodeName string) {
	cmdStr := fmt.Sprintf(`clusterctl node get %s`, nodeName)
	out, err := s.tbn1.RunCommandWithOutput(cmdStr)
	s.Assert(c, err, IsNil, Commentf("output: %s", out))
	exptdOut := `.*"monitoring-state":.*`
	s.assertMultiMatch(c, exptdOut, out, 1)
	exptdOut = `.*"inventory-state":.*`
	s.assertMultiMatch(c, exptdOut, out, 1)
	exptdOut = `.*"configuration-state".*`
	s.assertMultiMatch(c, exptdOut, out, 1)
}
