package conformance

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	sriovv1 "github.com/openshift/sriov-network-operator/pkg/apis/sriovnetwork/v1"
	"github.com/openshift/sriov-tests/pkg/util/cluster"
	"github.com/openshift/sriov-tests/pkg/util/execute"
	"github.com/openshift/sriov-tests/pkg/util/namespaces"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("operator", func() {
	// var sriovInfos *sriovEnabledNodes
	execute.BeforeAll(func() {
		err := namespaces.Create(namespaces.Test, clients)
		Expect(err).ToNot(HaveOccurred())

		err = namespaces.Clean(namespaces.Test, clients)
		Expect(err).ToNot(HaveOccurred())

		// This also validates that sriov is properly configured, ignoring the configuration ATM
		_, err = cluster.DiscoverSriov(clients, operatorNamespace)
		Expect(err).ToNot(HaveOccurred())
	})

	var _ = Describe("Configuration", func() {

		Context("SR-IOV network config daemon can be set by nodeselector", func() {
			It("Should schedule the config daemon on selected nodes", func() {

				By("Checking that a daemon is scheduled on each worker node")
				Eventually(func() bool {
					return daemonsScheduledOnNodes("node-role.kubernetes.io/worker=")
				}, 3*time.Minute, 1*time.Second).Should(Equal(true))

				By("Labelling one worker node with the label needed for the daemon")
				allNodes, err := clients.Nodes().List(metav1.ListOptions{
					LabelSelector: "node-role.kubernetes.io/worker",
				})
				Expect(len(allNodes.Items)).To(BeNumerically(">", 0), "There must be at least one worker")
				candidate := allNodes.Items[0]
				candidate.Labels["sriovenabled"] = "true"
				_, err = clients.Nodes().Update(&candidate)
				Expect(err).ToNot(HaveOccurred())

				By("Setting the node selector for each daemon")
				cfg := sriovv1.SriovOperatorConfig{}
				err = clients.Get(context.TODO(), runtimeclient.ObjectKey{
					Name:      "default",
					Namespace: operatorNamespace,
				}, &cfg)
				Expect(err).ToNot(HaveOccurred())
				cfg.Spec.ConfigDaemonNodeSelector = map[string]string{
					"sriovenabled": "true",
				}
				err = clients.Update(context.TODO(), &cfg)
				Expect(err).ToNot(HaveOccurred())

				By("Checking that a daemon is scheduled only on selected node")
				Eventually(func() bool {
					return !daemonsScheduledOnNodes("sriovenabled!=true") &&
						daemonsScheduledOnNodes("sriovenabled=true")
				}, 1*time.Minute, 1*time.Second).Should(Equal(true))

				By("Restoring the node selector for daemons")
				err = clients.Get(context.TODO(), runtimeclient.ObjectKey{
					Name:      "default",
					Namespace: operatorNamespace,
				}, &cfg)
				Expect(err).ToNot(HaveOccurred())
				cfg.Spec.ConfigDaemonNodeSelector = map[string]string{}
				err = clients.Update(context.TODO(), &cfg)
				Expect(err).ToNot(HaveOccurred())

				By("Checking that a daemon is scheduled on each worker node")
				Eventually(func() bool {
					return daemonsScheduledOnNodes("node-role.kubernetes.io/worker")
				}, 1*time.Minute, 1*time.Second).Should(Equal(true))

			})
		})
	})
})

func daemonsScheduledOnNodes(selector string) bool {
	nn, err := clients.Nodes().List(metav1.ListOptions{
		LabelSelector: selector,
	})
	nodes := nn.Items

	daemons, err := clients.Pods(operatorNamespace).List(metav1.ListOptions{LabelSelector: "app=sriov-network-config-daemon"})
	Expect(err).ToNot(HaveOccurred())
	for _, d := range daemons.Items {
		foundNode := false
		for i, n := range nodes {
			if d.Spec.NodeName == n.Name {
				foundNode = true
				// Removing the element from the list as we want to make sure
				// the daemons are running on different nodes
				nodes = append(nodes[:i], nodes[i+1:]...)
				break
			}
		}
		if !foundNode {
			return false
		}
	}
	return true

}
