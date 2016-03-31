package manager

import (
	"bufio"
	"fmt"
	"io"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/contiv/cluster/management/src/configuration"
	"github.com/contiv/cluster/management/src/monitor"
	"github.com/contiv/errored"
)

// errInvalidJSON is the error returned when an invalid json value is specified for
// the ansible extra variables configuration
var errInvalidJSON = func(name string, err error) error {
	return errored.Errorf("%q should be a valid json. Error: %s", name, err)
}

// event associates an event to corresponding processing logic
type event interface {
	String() string
	process() error
}

type nodeDiscovered struct {
	mgr  *Manager
	node monitor.SubsysNode
}

func newNodeDiscovered(mgr *Manager, node monitor.SubsysNode) *nodeDiscovered {
	return &nodeDiscovered{
		mgr:  mgr,
		node: node,
	}
}

func (e *nodeDiscovered) String() string {
	return fmt.Sprintf("nodeDiscovered: %+v", e.node)
}

func (e *nodeDiscovered) process() error {
	//XXX: need to form the name that adheres to collins tag requirements
	name := e.node.GetLabel() + "-" + e.node.GetSerial()

	enode, err := e.mgr.findNode(name)
	if err.Error() == nodeNotExistsError(name).Error() {
		e.mgr.nodes[name] = &node{
			// XXX: node's role/group shall come from manager's role assignment logic or
			// from user configuration
			Cfg: configuration.NewAnsibleHost(name, e.node.GetMgmtAddress(),
				ansibleMasterGroupName, map[string]string{
					ansibleNodeNameHostVar: name,
					ansibleNodeAddrHostVar: e.node.GetMgmtAddress(),
				}),
		}
		enode = e.mgr.nodes[name]
	} else if err != nil {
		return err
	}

	// update node's monitoring info to the one received in the event
	enode.Mon = e.node
	enode.Inv = e.mgr.inventory.GetAsset(name)
	if enode.Inv == nil {
		if err := e.mgr.inventory.AddAsset(name); err != nil {
			// XXX. Log this to collins
			log.Errorf("adding asset %q to discovered in inventory failed. Error: %s", name, err)
			return err
		}
		enode.Inv = e.mgr.inventory.GetAsset(name)
	} else if err := e.mgr.inventory.SetAssetDiscovered(name); err != nil {
		// XXX. Log this to collins
		log.Errorf("setting asset %q to discovered in inventory failed. Error: %s", name, err)
		return err
	}
	return nil
}

type nodeDisappeared struct {
	mgr  *Manager
	node monitor.SubsysNode
}

func newNodeDisappeared(mgr *Manager, node monitor.SubsysNode) *nodeDisappeared {
	return &nodeDisappeared{
		mgr:  mgr,
		node: node,
	}
}

func (e *nodeDisappeared) String() string {
	return fmt.Sprintf("nodeDisappeared: %+v", e.node)
}

func (e *nodeDisappeared) process() error {
	//XXX: need to form the name that adheres to collins tag requirements
	name := e.node.GetLabel() + "-" + e.node.GetSerial()

	node, err := e.mgr.findNode(name)
	if err != nil {
		return err
	}

	// update node's monitoring info to the one received in the event.
	node.Mon = e.node

	if err := e.mgr.inventory.SetAssetDisappeared(name); err != nil {
		// XXX. Log this to collins
		return err
	}
	return nil
}

type nodeCommissioned struct {
	mgr       *Manager
	nodeName  string
	extraVars string
}

func newNodeCommissioned(mgr *Manager, nodeName, extraVars string) *nodeCommissioned {
	return &nodeCommissioned{
		mgr:       mgr,
		nodeName:  nodeName,
		extraVars: extraVars,
	}
}

func (e *nodeCommissioned) String() string {
	return fmt.Sprintf("nodeCommissioned: %s", e.nodeName)
}

func (e *nodeCommissioned) process() error {
	isDiscovered, err := e.mgr.isDiscoveredNode(e.nodeName)
	if err != nil {
		return err
	}
	if !isDiscovered {
		return errored.Errorf("node %q has disappeared from monitoring subsystem, it can't be commissioned. Please check node's network reachability", e.nodeName)
	}

	if err := e.mgr.inventory.SetAssetProvisioning(e.nodeName); err != nil {
		// XXX. Log this to collins
		return err
	}

	// trigger node configuration event
	e.mgr.reqQ <- newNodeConfigure(e.mgr, e.nodeName, e.extraVars)
	return nil
}

type nodeDecommissioned struct {
	mgr       *Manager
	nodeName  string
	extraVars string
}

func newNodeDecommissioned(mgr *Manager, nodeName, extraVars string) *nodeDecommissioned {
	return &nodeDecommissioned{
		mgr:       mgr,
		nodeName:  nodeName,
		extraVars: extraVars,
	}
}

func (e *nodeDecommissioned) String() string {
	return fmt.Sprintf("nodeDecommissioned: %s", e.nodeName)
}

func (e *nodeDecommissioned) process() error {
	isMasterNode, err := e.mgr.isMasterNode(e.nodeName)
	if err != nil {
		return err
	}

	// before setting the node cancelled and triggering the cleanup make sure
	// that the master node is decommissioned only if there are no more worker nodes.
	// XXX: revisit this check once we are able to support multiple master nodes.
	if isMasterNode {
		for name := range e.mgr.nodes {
			if name == e.nodeName {
				// skip this node
				continue
			}

			isDiscoveredAndAllocated, err := e.mgr.isDiscoveredAndAllocatedNode(name)
			if err != nil || !isDiscoveredAndAllocated {
				if err != nil {
					log.Debugf("a node check failed for %q. Error: %s", name, err)
				}
				// skip hosts that are not yet provisioned or not in discovered state
				continue
			}

			isWorkerNode, err := e.mgr.isWorkerNode(name)
			if err != nil {
				// skip this node
				log.Debugf("a node check failed for %q. Error: %s", name, err)
				continue
			}

			if isWorkerNode {
				return errored.Errorf("%q is a master node and it can only be decommissioned after all worker nodes have been decommissioned", e.nodeName)
			}
		}
	}

	if err := e.mgr.inventory.SetAssetCancelled(e.nodeName); err != nil {
		// XXX. Log this to collins
		return err
	}
	// trigger node clenup event
	e.mgr.reqQ <- newNodeCleanup(e.mgr, e.nodeName, e.extraVars)
	return nil
}

type nodeInMaintenance struct {
	mgr       *Manager
	nodeName  string
	extraVars string
}

func newNodeInMaintenance(mgr *Manager, nodeName, extraVars string) *nodeInMaintenance {
	return &nodeInMaintenance{
		mgr:       mgr,
		nodeName:  nodeName,
		extraVars: extraVars,
	}
}

func (e *nodeInMaintenance) String() string {
	return fmt.Sprintf("nodeInMaintenance: %s", e.nodeName)
}

func (e *nodeInMaintenance) process() error {
	if err := e.mgr.inventory.SetAssetInMaintenance(e.nodeName); err != nil {
		// XXX. Log this to collins
		return err
	}
	// trigger node upgrade event
	e.mgr.reqQ <- newNodeUpgrade(e.mgr, e.nodeName, e.extraVars)
	return nil
}

type nodeConfigure struct {
	mgr       *Manager
	nodeName  string
	extraVars string
}

func newNodeConfigure(mgr *Manager, nodeName, extraVars string) *nodeConfigure {
	return &nodeConfigure{
		mgr:       mgr,
		nodeName:  nodeName,
		extraVars: extraVars,
	}
}

func (e *nodeConfigure) String() string {
	return fmt.Sprintf("nodeConfigure: %s", e.nodeName)
}

// helper function to log the stream of bytes from a reader while waiting on
// the error channel. It returns on first error received on the channel
func logOutputAndReturnStatus(r io.Reader, errCh chan error) error {
	// this can happen if an error occurred before the ansible could be run,
	// just return that error
	if r == nil {
		return <-errCh
	}

	s := bufio.NewScanner(r)
	ticker := time.Tick(50 * time.Millisecond)
	for {
		select {
		case err := <-errCh:
			for s.Scan() {
				log.Infof("%s", s.Bytes())
			}
			return err
		case <-ticker:
			// scan any available output while waiting
			if s.Scan() {
				log.Infof("%s", s.Bytes())
			}
		}
	}
}

func (e *nodeConfigure) process() error {
	node, err := e.mgr.findNode(e.nodeName)
	if err != nil {
		return err
	}

	if node.Cfg == nil {
		return nodeConfigNotExistsError(e.nodeName)
	}

	hostInfo := node.Cfg.(*configuration.AnsibleHost)
	nodeGroup := ansibleMasterGroupName
	masterAddr := ""
	masterName := ""
	// update the online master address if this is second node that is being commissioned.
	// Also set the group for second or later nodes to be worker, as right now services like
	// swarm and netmaster can only have one master node and also we don't yet have a vip
	// service.
	// XXX: revisit this when the above changes
	for name, node := range e.mgr.nodes {
		if name == e.nodeName {
			// skip this node
			continue
		}

		isDiscoveredAndAllocated, err := e.mgr.isDiscoveredAndAllocatedNode(name)
		if err != nil || !isDiscoveredAndAllocated {
			if err != nil {
				log.Debugf("a node check failed for %q. Error: %s", name, err)
			}
			// skip hosts that are not yet provisioned or not in discovered state
			continue
		}

		isMasterNode, err := e.mgr.isMasterNode(name)
		if err != nil || !isMasterNode {
			if err != nil {
				log.Debugf("a node check failed for %q. Error: %s", name, err)
			}
			//skip the hosts that are not in master group
			continue
		}

		// found our node
		masterAddr = node.Mon.GetMgmtAddress()
		masterName = node.Cfg.GetTag()
		nodeGroup = ansibleWorkerGroupName
		break
	}
	hostInfo.SetGroup(nodeGroup)
	hostInfo.SetVar(ansibleEtcdMasterAddrHostVar, masterAddr)
	hostInfo.SetVar(ansibleEtcdMasterNameHostVar, masterName)
	outReader, _, errCh := e.mgr.configuration.Configure(
		configuration.SubsysHosts([]*configuration.AnsibleHost{hostInfo}), e.extraVars)
	if err := logOutputAndReturnStatus(outReader, errCh); err != nil {
		log.Errorf("configuration failed. Error: %s", err)
		// set asset state back to unallocated
		if err1 := e.mgr.inventory.SetAssetUnallocated(e.nodeName); err1 != nil {
			// XXX. Log this to collins
			return err1
		}
		return err
	}
	// set asset state to commissioned
	if err := e.mgr.inventory.SetAssetCommissioned(e.nodeName); err != nil {
		// XXX. Log this to collins
		return err
	}
	return nil
}

type nodeCleanup struct {
	mgr       *Manager
	nodeName  string
	extraVars string
}

func newNodeCleanup(mgr *Manager, nodeName, extraVars string) *nodeCleanup {
	return &nodeCleanup{
		mgr:       mgr,
		nodeName:  nodeName,
		extraVars: extraVars,
	}
}

func (e *nodeCleanup) String() string {
	return fmt.Sprintf("nodeCleanup: %s", e.nodeName)
}

func (e *nodeCleanup) process() error {
	node, err := e.mgr.findNode(e.nodeName)
	if err != nil {
		return err
	}

	if node.Cfg == nil {
		return nodeConfigNotExistsError(e.nodeName)
	}

	outReader, _, errCh := e.mgr.configuration.Cleanup(
		configuration.SubsysHosts([]*configuration.AnsibleHost{
			e.mgr.nodes[e.nodeName].Cfg.(*configuration.AnsibleHost),
		}), e.extraVars)
	if err := logOutputAndReturnStatus(outReader, errCh); err != nil {
		log.Errorf("cleanup failed. Error: %s", err)
	}
	// set asset state to decommissioned
	if err := e.mgr.inventory.SetAssetDecommissioned(e.nodeName); err != nil {
		// XXX. Log this to collins
		return err
	}
	return nil
}

type nodeUpgrade struct {
	mgr       *Manager
	nodeName  string
	extraVars string
}

func newNodeUpgrade(mgr *Manager, nodeName, extraVars string) *nodeUpgrade {
	return &nodeUpgrade{
		mgr:       mgr,
		nodeName:  nodeName,
		extraVars: extraVars,
	}
}

func (e *nodeUpgrade) String() string {
	return fmt.Sprintf("nodeUpgrade: %s", e.nodeName)
}

func (e *nodeUpgrade) process() error {
	node, err := e.mgr.findNode(e.nodeName)
	if err != nil {
		return err
	}

	if node.Cfg == nil {
		return nodeConfigNotExistsError(e.nodeName)
	}

	outReader, _, errCh := e.mgr.configuration.Upgrade(
		configuration.SubsysHosts([]*configuration.AnsibleHost{
			e.mgr.nodes[e.nodeName].Cfg.(*configuration.AnsibleHost),
		}), e.extraVars)
	if err := logOutputAndReturnStatus(outReader, errCh); err != nil {
		log.Errorf("upgrade failed. Error: %s", err)
		// set asset state to provision-failed
		if err1 := e.mgr.inventory.SetAssetUnallocated(e.nodeName); err1 != nil {
			// XXX. Log this to collins
			return err1
		}
		return err
	}
	// set asset state to commissioned
	if err := e.mgr.inventory.SetAssetCommissioned(e.nodeName); err != nil {
		// XXX. Log this to collins
		return err
	}
	return nil
}

type nodeDiscover struct {
	mgr       *Manager
	nodeAddr  string
	extraVars string
}

func newNodeDiscover(mgr *Manager, nodeAddr, extraVars string) *nodeDiscover {
	return &nodeDiscover{
		mgr:       mgr,
		nodeAddr:  nodeAddr,
		extraVars: extraVars,
	}
}

func (e *nodeDiscover) String() string {
	return fmt.Sprintf("nodeDiscover: %s", e.nodeAddr)
}

func (e *nodeDiscover) process() error {
	node, err := e.mgr.findNodeByMgmtAddr(e.nodeAddr)
	if err == nil {
		return errored.Errorf("a node %q already exists with the management address %q",
			node.Inv.GetTag(), e.nodeAddr)
	}

	// create a temporary ansible host config to provision the host in discover host-group
	hostCfg := configuration.NewAnsibleHost("node1", e.nodeAddr,
		ansibleDiscoverGroupName, map[string]string{
			ansibleNodeNameHostVar: "node1",
			ansibleNodeAddrHostVar: e.nodeAddr,
		})

	outReader, _, errCh := e.mgr.configuration.Configure(
		configuration.SubsysHosts([]*configuration.AnsibleHost{hostCfg}), e.extraVars)
	if err := logOutputAndReturnStatus(outReader, errCh); err != nil {
		log.Errorf("discover failed. Error: %s", err)
		return err
	}
	return nil
}

type setGlobals struct {
	mgr       *Manager
	extraVars string
}

func newSetGlobals(mgr *Manager, extraVars string) *setGlobals {
	return &setGlobals{
		mgr:       mgr,
		extraVars: extraVars,
	}
}

func (e *setGlobals) String() string {
	return fmt.Sprintf("setGlobals")
}

func (e *setGlobals) process() error {
	if err := e.mgr.configuration.SetGlobals(e.extraVars); err != nil {
		return err
	}
	return nil
}

// waitableEvent provides a way to wait for event's processing to complete
// and return the event's processing status.
// This can be useful for generating responses to a UI event.
// Note that an event processing may itself generate more events and it is upto
// the processing logic of the event to handle waits internally.
type waitableEvent struct {
	inEvent  event
	statusCh chan error
}

func newWaitableEvent(e event) *waitableEvent {
	return &waitableEvent{
		inEvent:  e,
		statusCh: make(chan error),
	}
}

func (e *waitableEvent) String() string {
	return fmt.Sprintf("waitableEvent: %s", e.inEvent)
}

func (e *waitableEvent) process() error {
	// run the contained event's processing
	err := e.inEvent.process()
	// signal it's status
	e.statusCh <- err
	//return the status to event loop
	return err
}

func (e *waitableEvent) waitForCompletion() error {
	select {
	case err := <-e.statusCh:
		return err
	}
}

func (m *Manager) eventLoop() {
	for {
		me := <-m.reqQ
		log.Debugf("dequeued manager event: %+v", me)
		if err := me.process(); err != nil {
			// log and continue
			log.Errorf("error handling event %q. Error: %s", me, err)
		}
	}
}
