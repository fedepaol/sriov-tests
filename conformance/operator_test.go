package conformance

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	sriovv1 "github.com/openshift/sriov-network-operator/pkg/apis/sriovnetwork/v1"
	"github.com/openshift/sriov-tests/pkg/util/cluster"
	"github.com/openshift/sriov-tests/pkg/util/execute"
	"github.com/openshift/sriov-tests/pkg/util/namespaces"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("operator", func() {
	var sriovInfos *cluster.EnabledNodes
	execute.BeforeAll(func() {
		err := namespaces.Create(namespaces.Test, clients)
		Expect(err).ToNot(HaveOccurred())

		sriovInfos, err = cluster.DiscoverSriov(clients, operatorNamespace)
		Expect(err).ToNot(HaveOccurred())
	})

	BeforeEach(func() {
		err := namespaces.Clean(operatorNamespace, namespaces.Test, clients)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() bool {
			res, err := cluster.SriovStable(operatorNamespace, clients)
			Expect(err).ToNot(HaveOccurred())
			return res
		}, 3*time.Minute, 1*time.Second).Should(Equal(true))
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

		Context("PF Partitioning", func() {
			It("Should be possible to partition the pf's vfs", func() {
				node := sriovInfos.Nodes[0]
				intf, err := sriovInfos.FindOneSriovDevice(node)
				Expect(err).ToNot(HaveOccurred())
				Expect(intf.TotalVfs).To(BeNumerically(">", 7))

				firstConfig := &sriovv1.SriovNetworkNodePolicy{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "testpolicy",
						Namespace:    operatorNamespace,
					},

					Spec: sriovv1.SriovNetworkNodePolicySpec{
						NodeSelector: map[string]string{
							"kubernetes.io/hostname": node,
						},
						NumVfs:       5,
						ResourceName: "testresource",
						Priority:     99,
						NicSelector: sriovv1.SriovNetworkNicSelector{
							PfNames: []string{intf.Name + "#2-4"},
						},
						DeviceType: "netdevice",
					},
				}

				err = clients.Create(context.Background(), firstConfig)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() sriovv1.Interfaces {
					nodeState, err := clients.SriovNetworkNodeStates(operatorNamespace).Get(node, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred())
					return nodeState.Spec.Interfaces
				}, 1*time.Minute, 1*time.Second).Should(ContainElement(MatchFields(
					IgnoreExtras,
					Fields{
						"Name":     Equal(intf.Name),
						"NumVfs":   Equal(5),
						"VfGroups": ContainElement(sriovv1.VfGroup{ResourceName: "testresource", DeviceType: "netdevice", VfRange: "2-4"}),
					})))

				Eventually(func() int64 {
					testedNode, err := clients.Nodes().Get(node, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred())
					resNum, _ := testedNode.Status.Capacity["openshift.io/testresource"]
					capacity, _ := resNum.AsInt64()
					return capacity
				}, 3*time.Minute, time.Second).Should(Equal(int64(3)))

				secondConfig := &sriovv1.SriovNetworkNodePolicy{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "testpolicy",
						Namespace:    operatorNamespace,
					},

					Spec: sriovv1.SriovNetworkNodePolicySpec{
						NodeSelector: map[string]string{
							"kubernetes.io/hostname": node,
						},
						NumVfs:       5,
						ResourceName: "testresource1",
						Priority:     99,
						NicSelector: sriovv1.SriovNetworkNicSelector{
							PfNames: []string{intf.Name + "#0-1"},
						},
						DeviceType: "vfio-pci",
					},
				}

				err = clients.Create(context.Background(), secondConfig)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() sriovv1.Interfaces {
					nodeState, err := clients.SriovNetworkNodeStates(operatorNamespace).Get(node, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred())
					return nodeState.Spec.Interfaces
				}, 3*time.Minute, 1*time.Second).Should(ContainElement(MatchFields(
					IgnoreExtras,
					Fields{
						"Name":   Equal(intf.Name),
						"NumVfs": Equal(5),
						"VfGroups": SatisfyAll(
							ContainElement(
								sriovv1.VfGroup{ResourceName: "testresource", DeviceType: "netdevice", VfRange: "2-4"}),
							ContainElement(
								sriovv1.VfGroup{ResourceName: "testresource1", DeviceType: "vfio-pci", VfRange: "0-1"}),
						),
					},
				)))

				Eventually(func() map[string]int64 {
					testedNode, err := clients.Nodes().Get(node, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred())
					resNum, _ := testedNode.Status.Capacity["openshift.io/testresource"]
					capacity, _ := resNum.AsInt64()
					res := make(map[string]int64)
					res["openshift.io/testresource"] = capacity
					resNum, _ = testedNode.Status.Capacity["openshift.io/testresource1"]
					capacity, _ = resNum.AsInt64()
					res["openshift.io/testresource1"] = capacity
					return res
				}, time.Minute, time.Second).Should(Equal(map[string]int64{
					"openshift.io/testresource":  int64(3),
					"openshift.io/testresource1": int64(2),
				}))
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
