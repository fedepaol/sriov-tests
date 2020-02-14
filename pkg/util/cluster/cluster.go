package cluster

import (
	"fmt"

	sriovv1 "github.com/openshift/sriov-network-operator/pkg/apis/sriovnetwork/v1"
	testclient "github.com/openshift/sriov-tests/pkg/util/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnabledNodes provides info on sriov enabled nodes of the cluster.
type EnabledNodes struct {
	Nodes  []string
	States map[string]sriovv1.SriovNetworkNodeState
}

// DiscoverSriov retrieves Sriov related information of a given cluster.
func DiscoverSriov(clients *testclient.ClientSet, operatorNamespace string) (*EnabledNodes, error) {
	nodeStates, err := clients.SriovNetworkNodeStates(operatorNamespace).List(metav1.ListOptions{})
	res := &EnabledNodes{}
	res.States = make(map[string]sriovv1.SriovNetworkNodeState)
	res.Nodes = make([]string, 0)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve note states %v", err)
	}

	for _, state := range nodeStates.Items {
		if state.Status.SyncStatus != "Succeeded" {
			return nil, fmt.Errorf("Sync status still in progress")
		}
		node := state.Name
		for _, itf := range state.Status.Interfaces {
			if itf.TotalVfs > 0 {
				res.Nodes = append(res.Nodes, node)
				res.States[node] = state
				break
			}
		}
	}

	if len(res.Nodes) == 0 {
		return nil, fmt.Errorf("No sriov enabled node found")
	}
	return res, nil
}

// FindOneSriovDevice retrieves a valid sriov device for the given node.
func (n *EnabledNodes) FindOneSriovDevice(node string) (*sriovv1.InterfaceExt, error) {
	s, ok := n.States[node]
	if !ok {
		return nil, fmt.Errorf("Node %s not found", node)
	}
	for _, itf := range s.Status.Interfaces {
		if itf.TotalVfs > 0 {
			return &itf, nil
		}
	}

	return nil, fmt.Errorf("Unable to find sriov devices in node %s", node)
}
