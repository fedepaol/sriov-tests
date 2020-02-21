package conformance

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	netattdefv1 "github.com/openshift/sriov-network-operator/pkg/apis/k8s/v1"
	sriovv1 "github.com/openshift/sriov-network-operator/pkg/apis/sriovnetwork/v1"
	"github.com/openshift/sriov-tests/pkg/util/cluster"
	"github.com/openshift/sriov-tests/pkg/util/execute"
	"github.com/openshift/sriov-tests/pkg/util/namespaces"
	"github.com/openshift/sriov-tests/pkg/util/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
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
			It("Should not be possible to have overlapping pf ranges", func() {
				// Skipping this test as blocking the override will
				// be implemented in 4.5, as per bz #1798880
				Skip("Overlapping is still not blocked")
				node := sriovInfos.Nodes[0]
				intf, err := sriovInfos.FindOneSriovDevice(node)
				Expect(err).ToNot(HaveOccurred())

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
							PfNames: []string{intf.Name + "#1-4"},
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
						"VfGroups": ContainElement(sriovv1.VfGroup{ResourceName: "testresource", DeviceType: "netdevice", VfRange: "1-4"}),
					})))

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
							PfNames: []string{intf.Name + "#0-2"},
						},
						DeviceType: "vfio-pci",
					},
				}

				err = clients.Create(context.Background(), secondConfig)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("VF flags", func() {
			debugPod := &corev1.Pod{}
			intf := &sriovv1.InterfaceExt{}

			validationFunction := func(networks []string, containsFunc func(line string) bool) {
				podObj := pod.DefineWithNetworks(networks)
				err := clients.Create(context.Background(), podObj)
				Expect(err).ToNot(HaveOccurred())
				Eventually(func() corev1.PodPhase {
					podObj, err = clients.Pods(namespaces.Test).Get(podObj.Name, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred())
					return podObj.Status.Phase
				}, 3*time.Minute, time.Second).Should(Equal(corev1.PodRunning))

				Expect(err).ToNot(HaveOccurred())
				stdout, stderr, err := pod.ExecCommand(clients, podObj, "ip", "addr", "show", "dev", "net1")
				Expect(err).ToNot(HaveOccurred())
				Expect(stderr).To(Equal(""))
				podMac := getMacFrom(stdout)
				stdout, stderr, err = pod.ExecCommand(clients, debugPod, "ip", "link", "show")
				Expect(err).ToNot(HaveOccurred())
				Expect(stderr).To(Equal(""))

				found := false
				for _, line := range strings.Split(stdout, "\n") {
					if strings.Contains(line, podMac) && containsFunc(line) {
						found = true
						break
					}
				}

				err = clients.Pods(namespaces.Test).Delete(podObj.Name, &metav1.DeleteOptions{
					GracePeriodSeconds: pointer.Int64Ptr(0)})
				Expect(err).ToNot(HaveOccurred())

				Expect(found).To(BeTrue())
			}

			validateNetworkFields := func(sriovNetwork *sriovv1.SriovNetwork, validationString string) {
				netAttDef := &netattdefv1.NetworkAttachmentDefinition{}
				Eventually(func() error {
					return clients.Get(context.Background(), runtimeclient.ObjectKey{Name: sriovNetwork.Name, Namespace: namespaces.Test}, netAttDef)
				}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())

				checkFunc := func(line string) bool {
					if strings.Contains(line, validationString) {
						return true
					}
					return false
				}

				validationFunction([]string{sriovNetwork.Name}, checkFunc)
			}

			BeforeEach(func() {
				var err error
				node := sriovInfos.Nodes[0]
				intf, err = sriovInfos.FindOneSriovDevice(node)
				Expect(err).ToNot(HaveOccurred())

				config := &sriovv1.SriovNetworkNodePolicy{
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
							PfNames: []string{intf.Name},
						},
						DeviceType: "netdevice",
					},
				}

				err = clients.Create(context.Background(), config)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() sriovv1.Interfaces {
					nodeState, err := clients.SriovNetworkNodeStates(operatorNamespace).Get(node, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred())
					return nodeState.Spec.Interfaces
				}, 1*time.Minute, 1*time.Second).Should(ContainElement(MatchFields(
					IgnoreExtras,
					Fields{
						"Name":   Equal(intf.Name),
						"NumVfs": Equal(5),
					})))

				Eventually(func() bool {
					res, err := cluster.SriovStable(operatorNamespace, clients)
					Expect(err).ToNot(HaveOccurred())
					return res
				}, 3*time.Minute, 1*time.Second).Should(BeTrue())

				debugPod = pod.DefineWithHostNetwork()
				err = clients.Create(context.Background(), debugPod)
				Expect(err).ToNot(HaveOccurred())
				Eventually(func() corev1.PodPhase {
					debugPod, err = clients.Pods(namespaces.Test).Get(debugPod.Name, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred())
					return debugPod.Status.Phase
				}, 3*time.Minute, time.Second).Should(Equal(corev1.PodRunning))
			})

			// 25959
			It("Should configure the spoofChk boolean variable", func() {
				sriovNetwork := &sriovv1.SriovNetwork{
					ObjectMeta: metav1.ObjectMeta{Name: "spoofnetwork", Namespace: operatorNamespace},
					Spec: sriovv1.SriovNetworkSpec{
						ResourceName: "testresource",
						IPAM: `{"type":"host-local",
									"subnet":"10.10.10.0/24",
									"rangeStart":"10.10.10.171",
									"rangeEnd":"10.10.10.181",
									"routes":[{"dst":"0.0.0.0/0"}],
									"gateway":"10.10.10.1"}`,
						NetworkNamespace: namespaces.Test,
					}}

				By("configuring spoofChk on")
				copyObj := sriovNetwork.DeepCopy()
				copyObj.Spec.SpoofChk = "on"
				spoofChkStatusValidation := "spoof checking on"
				err := clients.Create(context.Background(), copyObj)
				Expect(err).ToNot(HaveOccurred())

				validateNetworkFields(copyObj, spoofChkStatusValidation)

				By("removing sriov network")
				err = clients.Delete(context.Background(), sriovNetwork)
				Expect(err).ToNot(HaveOccurred())

				By("configuring spoofChk off")
				copyObj = sriovNetwork.DeepCopy()
				copyObj.Spec.SpoofChk = "off"
				spoofChkStatusValidation = "spoof checking off"
				err = clients.Create(context.Background(), copyObj)
				Expect(err).ToNot(HaveOccurred())

				validateNetworkFields(copyObj, spoofChkStatusValidation)
			})

			// 25960
			It("Should configure the trust boolean variable", func() {
				sriovNetwork := &sriovv1.SriovNetwork{
					ObjectMeta: metav1.ObjectMeta{Name: "trustnetwork", Namespace: operatorNamespace},
					Spec: sriovv1.SriovNetworkSpec{
						ResourceName: "testresource",
						IPAM: `{"type":"host-local",
									"subnet":"10.10.10.0/24",
									"rangeStart":"10.10.10.171",
									"rangeEnd":"10.10.10.181",
									"routes":[{"dst":"0.0.0.0/0"}],
									"gateway":"10.10.10.1"}`,
						NetworkNamespace: namespaces.Test,
					}}

				By("configuring trust on")
				copyObj := sriovNetwork.DeepCopy()
				copyObj.Spec.Trust = "on"
				trustChkStatusValidation := "trust on"
				err := clients.Create(context.Background(), copyObj)
				Expect(err).ToNot(HaveOccurred())

				validateNetworkFields(copyObj, trustChkStatusValidation)

				By("removing sriov network")
				err = clients.Delete(context.Background(), sriovNetwork)
				Expect(err).ToNot(HaveOccurred())

				By("configuring trust off")
				copyObj = sriovNetwork.DeepCopy()
				copyObj.Spec.Trust = "off"
				trustChkStatusValidation = "trust off"
				err = clients.Create(context.Background(), copyObj)
				Expect(err).ToNot(HaveOccurred())

				validateNetworkFields(copyObj, trustChkStatusValidation)
			})

			// 25963
			Describe("rate limit", func() {
				It("Should configure the requested rate limit flags under the vf", func() {
					node := sriovInfos.Nodes[0]
					intf, err := sriovInfos.FindOneSriovDevice(node)
					Expect(err).ToNot(HaveOccurred())

					if intf.Driver != "mlx5_core" {
						// There is an issue with the intel cards both driver i40 and ixgbe
						// BZ 1772847
						// BZ 1772815
						// BZ 1236146
						Skip("Skip rate limit test no mellanox driver found")
					}

					var maxTxRate = 100
					var minTxRate = 40
					sriovNetwork := &sriovv1.SriovNetwork{ObjectMeta: metav1.ObjectMeta{Name: "ratenetwork", Namespace: operatorNamespace},
						Spec: sriovv1.SriovNetworkSpec{
							ResourceName: "testresource",
							IPAM: `{"type":"host-local",
									"subnet":"10.10.10.0/24",
									"rangeStart":"10.10.10.171",
									"rangeEnd":"10.10.10.181",
									"routes":[{"dst":"0.0.0.0/0"}],
									"gateway":"10.10.10.1"}`,
							MaxTxRate:        &maxTxRate,
							MinTxRate:        &minTxRate,
							NetworkNamespace: namespaces.Test,
						}}
					err = clients.Create(context.Background(), sriovNetwork)
					Expect(err).ToNot(HaveOccurred())

					netAttDef := &netattdefv1.NetworkAttachmentDefinition{}
					Eventually(func() error {
						return clients.Get(context.Background(), runtimeclient.ObjectKey{Name: "ratenetwork", Namespace: namespaces.Test}, netAttDef)
					}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())

					checkFunc := func(line string) bool {
						if strings.Contains(line, "max_tx_rate 100Mbps") &&
							strings.Contains(line, "min_tx_rate 40Mbps") {
							return true
						}
						return false
					}

					validationFunction([]string{"ratenetwork"}, checkFunc)
				})
			})

			// 25963
			Describe("vlan and Qos vlan", func() {
				It("Should configure the requested vlan and Qos vlan flags under the vf", func() {
					sriovNetwork := &sriovv1.SriovNetwork{ObjectMeta: metav1.ObjectMeta{Name: "quosnetwork", Namespace: operatorNamespace},
						Spec: sriovv1.SriovNetworkSpec{
							ResourceName: "testresource",
							IPAM: `{"type":"host-local",
									"subnet":"10.10.10.0/24",
									"rangeStart":"10.10.10.171",
									"rangeEnd":"10.10.10.181",
									"routes":[{"dst":"0.0.0.0/0"}],
									"gateway":"10.10.10.1"}`,
							Vlan:             1,
							VlanQoS:          2,
							NetworkNamespace: namespaces.Test,
						}}
					err := clients.Create(context.Background(), sriovNetwork)
					Expect(err).ToNot(HaveOccurred())

					netAttDef := &netattdefv1.NetworkAttachmentDefinition{}
					Eventually(func() error {
						return clients.Get(context.Background(), runtimeclient.ObjectKey{Name: "quosnetwork", Namespace: namespaces.Test}, netAttDef)
					}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())

					checkFunc := func(line string) bool {
						if strings.Contains(line, "vlan 1") &&
							strings.Contains(line, "qos 2") {
							return true
						}
						return false
					}

					validationFunction([]string{"quosnetwork"}, checkFunc)
				})
			})
		})
		Context("Resource Injector", func() {
			// 25815
			It("Should inject downward api volume", func() {
				node := sriovInfos.Nodes[0]
				intf, err := sriovInfos.FindOneSriovDevice(node)
				Expect(err).ToNot(HaveOccurred())

				nodePolicy := &sriovv1.SriovNetworkNodePolicy{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "apivolumepolicy",
						Namespace:    operatorNamespace,
					},

					Spec: sriovv1.SriovNetworkNodePolicySpec{
						NodeSelector: map[string]string{
							"kubernetes.io/hostname": node,
						},
						NumVfs:       5,
						ResourceName: "apivolresource",
						Priority:     99,
						NicSelector: sriovv1.SriovNetworkNicSelector{
							PfNames: []string{intf.Name},
						},
						DeviceType: "netdevice",
					},
				}

				err = clients.Create(context.Background(), nodePolicy)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() bool {
					stable, err := cluster.SriovStable(operatorNamespace, clients)
					Expect(err).ToNot(HaveOccurred())
					return stable
				}, 5*time.Minute, 1*time.Second).Should(Equal(true))

				Eventually(func() int64 {
					testedNode, err := clients.Nodes().Get(node, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred())
					resNum, _ := testedNode.Status.Capacity["openshift.io/apivolresource"]
					capacity, _ := resNum.AsInt64()
					return capacity
				}, 3*time.Minute, time.Second).Should(Equal(int64(5)))

				sriovNetwork := &sriovv1.SriovNetwork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "apivolnetwork",
						Namespace: operatorNamespace,
					},
					Spec: sriovv1.SriovNetworkSpec{
						ResourceName:     "apivolresource",
						IPAM:             `{"type":"host-local","subnet":"10.10.10.0/24","rangeStart":"10.10.10.171","rangeEnd":"10.10.10.181","routes":[{"dst":"0.0.0.0/0"}],"gateway":"10.10.10.1"}`,
						NetworkNamespace: namespaces.Test,
					}}
				err = clients.Create(context.Background(), sriovNetwork)
				Expect(err).ToNot(HaveOccurred())

				podDefinition := pod.DefineWithNetworks([]string{sriovNetwork.Name})
				created, err := clients.Pods(namespaces.Test).Create(podDefinition)
				Expect(err).ToNot(HaveOccurred())

				var runningPod *corev1.Pod
				Eventually(func() corev1.PodPhase {
					runningPod, err = clients.Pods(namespaces.Test).Get(created.Name, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred())
					return runningPod.Status.Phase
				}, 3*time.Minute, time.Second).Should(Equal(corev1.PodRunning))

				var downwardVolume *corev1.Volume
				for _, v := range runningPod.Spec.Volumes {
					if v.Name == "podnetinfo" {
						downwardVolume = v.DeepCopy()
						break
					}
				}

				Expect(downwardVolume).ToNot(BeNil(), "Downward volume not found")
				Expect(downwardVolume.DownwardAPI).ToNot(BeNil(), "Downward api not found in volume")
				Expect(downwardVolume.DownwardAPI.Items).To(SatisfyAll(
					ContainElement(corev1.DownwardAPIVolumeFile{
						Path: "labels",
						FieldRef: &corev1.ObjectFieldSelector{
							APIVersion: "v1",
							FieldPath:  "metadata.labels",
						},
					}), ContainElement(corev1.DownwardAPIVolumeFile{
						Path: "annotations",
						FieldRef: &corev1.ObjectFieldSelector{
							APIVersion: "v1",
							FieldPath:  "metadata.annotations",
						},
					})))
			})
		})
	})
})

func getMacFrom(interfaceOutput string) string {
	interfaceOutput = strings.TrimSpace(interfaceOutput)
	lines := strings.Split(interfaceOutput, "\n")
	Expect(len(lines)).To(Equal(6))

	macConfig := strings.TrimSpace(lines[1])
	macConfigSplit := strings.Split(macConfig, " ")
	Expect(len(macConfigSplit)).To(Equal(4))

	return macConfigSplit[1]
}

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
