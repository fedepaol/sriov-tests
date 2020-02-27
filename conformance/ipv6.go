package conformance

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift/sriov-tests/pkg/util/cluster"
	"github.com/openshift/sriov-tests/pkg/util/execute"
	"github.com/openshift/sriov-tests/pkg/util/namespaces"
	"github.com/openshift/sriov-tests/pkg/util/network"
	"github.com/openshift/sriov-tests/pkg/util/pod"
	netattdefv1 "github.com/openshift/sriov-network-operator/pkg/apis/k8s/v1"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

)

const sriovInterfaceName = "net1"
const numVfs = 5
const resourceName = "sriovnic"
const ipv6NetworkName = "ipv6network"

var _ = Describe("ipv6", func() {
	var sriovInfos *cluster.EnabledNodes
	var testNode string

	execute.BeforeAll(func() {
		err := namespaces.Create(namespaces.Test, clients)
		Expect(err).ToNot(HaveOccurred())

		sriovInfos, err = cluster.DiscoverSriov(clients, operatorNamespace)
		Expect(err).ToNot(HaveOccurred())
		testNode = sriovInfos.Nodes[0]
	})

	BeforeEach(func() {
		err := namespaces.Clean(operatorNamespace, namespaces.Test, clients)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() bool {
			res, err := cluster.SriovStable(operatorNamespace, clients)
			Expect(err).ToNot(HaveOccurred())
			return res
		}, 10*time.Minute, 1*time.Second).Should(Equal(true))
	})

	Describe("ipv6", func() {
		Context("IPv6 configured secondary interfaces on pods", func() {
			It("should be able to ping each other", func() {
				sriovDevice, err := sriovInfos.FindOneSriovDevice(testNode)
				Expect(err).ToNot(HaveOccurred())
				createSriovPolicy(sriovDevice.Name, testNode)

				ipam := `{"type": "host-local","ranges": [[{"subnet": "3ffe:ffff:0:01ff::/64"}]],"dataDir": "/run/my-orchestrator/container-ipam-state"}`
				err = network.CreateSriovNetwork(clients, ipv6NetworkName, namespaces.Test, operatorNamespace, resourceName, ipam)
				Expect(err).ToNot(HaveOccurred())
				Eventually(func() error {
					netAttDef := &netattdefv1.NetworkAttachmentDefinition{}
					return clients.Get(context.Background(), runtimeclient.ObjectKey{Name: ipv6NetworkName, Namespace: namespaces.Test}, netAttDef)
					}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())

				pod := createTestPod([]string{"/bin/bash", "-c", "--"},
					[]string{"while true; do sleep 300000; done;"}, testNode, ipv6NetworkName)
				ips, err := network.GetSriovNicIPs(pod, sriovInterfaceName)
				Expect(err).ToNot(HaveOccurred())
				Expect(ips).NotTo(BeNil(), "No sriov network interface found.")
				Expect(len(ips)).Should(Equal(1))
				for _, ip := range ips {
					pingPod(ip, testNode, ipv6NetworkName)
				}
			})
		})
	})
})

func createSriovPolicy(sriovDevice string, testNode string) {
	err := network.CreateSriovPolicy(clients, "test-policy-", operatorNamespace, sriovDevice, testNode, numVfs, resourceName)
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() bool {
		stable, err := cluster.SriovStable(operatorNamespace, clients)
		Expect(err).ToNot(HaveOccurred())
		return stable
	}, 10*time.Minute, 1*time.Second).Should(Equal(true))

	Eventually(func() int64 {
		testedNode, err := clients.Nodes().Get(testNode, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		resNum, _ := testedNode.Status.Capacity["openshift.io/" + resourceName]
		capacity, _ := resNum.AsInt64()
		return capacity
	}, 3*time.Minute, time.Second).Should(Equal(int64(numVfs)))	
}

func createTestPod(command []string, args []string, node string, sriovNetworkAttachment string) *k8sv1.Pod {
	podDefinition := pod.RedefineWithNodeSelector(
		pod.DefineWithNetworks([]string {ipv6NetworkName}),
		node,
	)
	createdPod, err := clients.Pods(namespaces.Test).Create(podDefinition)

	Eventually(func() k8sv1.PodPhase {
		runningPod, err := clients.Pods(namespaces.Test).Get(createdPod.Name, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		return runningPod.Status.Phase
	}, 3*time.Minute, 1*time.Second).Should(Equal(k8sv1.PodRunning))
	pod, err := clients.Pods(namespaces.Test).Get(createdPod.Name, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())
	return pod
}

func pingPod(ip string, nodeSelector string, sriovNetworkAttachment string) {
	podDefinition := pod.RedefineWithNodeSelector(
		pod.RedefineWithRestartPolicy(
			pod.RedefineWithCommand(
				pod.DefineWithNetworks([]string {ipv6NetworkName}),
				[]string{"sh", "-c", "ping -6 -c 3 " + ip}, []string{},
			),
			k8sv1.RestartPolicyNever,
		),
		nodeSelector,
	)
	createdPod, err := clients.Pods(namespaces.Test).Create(podDefinition)
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() k8sv1.PodPhase {
		runningPod, err := clients.Pods(namespaces.Test).Get(createdPod.Name, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		return runningPod.Status.Phase
	}, 3*time.Minute, 1*time.Second).Should(Equal(k8sv1.PodSucceeded))
}
