package conformance

import (
	"context"
	"fmt"
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
	"github.com/openshift/sriov-tests/pkg/util/network"
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
		}, 10*time.Minute, 1*time.Second).Should(Equal(true))
	})

	var _ = Describe("Configuration", func() {

		Context("SR-IOV network config daemon can be set by nodeselector", func() {
			// 26186
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
			// 27633
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

				Eventually(func() bool {
					res, err := cluster.SriovStable(operatorNamespace, clients)
					Expect(err).ToNot(HaveOccurred())
					return res
				}, 10*time.Minute, 1*time.Second).Should(BeTrue())

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

				Eventually(func() bool {
					res, err := cluster.SriovStable(operatorNamespace, clients)
					Expect(err).ToNot(HaveOccurred())
					return res
				}, 10*time.Minute, 1*time.Second).Should(BeTrue())

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
				}, 2*time.Minute, time.Second).Should(Equal(map[string]int64{
					"openshift.io/testresource":  int64(3),
					"openshift.io/testresource1": int64(2),
				}))
			})

			// 27630
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
			numVfs := 5

			validationFunction := func(networks []string, containsFunc func(line string) bool) {
				// Validate all the virtual functions are in the host namespace
				_, err := findPodVFInHost(intf.Name, numVfs, debugPod)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("failed to find the vf number that was moved into the pod"))

				podObj := pod.DefineWithNetworks(networks)
				err = clients.Create(context.Background(), podObj)
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
				vfID, err := findPodVFInHost(intf.Name, numVfs, debugPod)
				Expect(err).ToNot(HaveOccurred())
				stdout, stderr, err = pod.ExecCommand(clients, debugPod, "ip", "link", "show")
				Expect(err).ToNot(HaveOccurred())
				Expect(stderr).To(Equal(""))

				found := false
				for _, line := range strings.Split(stdout, "\n") {
					if strings.Contains(line, fmt.Sprintf("vf %d ", vfID)) && containsFunc(line) {
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
						NumVfs:       numVfs,
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
						"NumVfs": Equal(numVfs),
					})))

				Eventually(func() bool {
					res, err := cluster.SriovStable(operatorNamespace, clients)
					Expect(err).ToNot(HaveOccurred())
					return res
				}, 10*time.Minute, 1*time.Second).Should(BeTrue())

				debugPod = pod.DefineWithHostNetwork(node)
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

			// 25961
			It("Should configure the the link state variable", func() {
				sriovNetwork := &sriovv1.SriovNetwork{
					ObjectMeta: metav1.ObjectMeta{Name: "statenetwork", Namespace: operatorNamespace},
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

				By("configuring link-state as enabled")
				enabledLinkNetwork := sriovNetwork.DeepCopy()
				enabledLinkNetwork.Spec.LinkState = "enable"
				linkStateChkStatusValidation := "link-state enable"
				err := clients.Create(context.Background(), enabledLinkNetwork)
				Expect(err).ToNot(HaveOccurred())

				validateNetworkFields(enabledLinkNetwork, linkStateChkStatusValidation)

				By("removing sriov network")
				err = clients.Delete(context.Background(), enabledLinkNetwork)
				Expect(err).ToNot(HaveOccurred())

				By("configuring link-state as disable")
				disabledLinkNetwork := sriovNetwork.DeepCopy()
				disabledLinkNetwork.Spec.LinkState = "disable"
				linkStateChkStatusValidation = "link-state disable"
				err = clients.Create(context.Background(), disabledLinkNetwork)
				Expect(err).ToNot(HaveOccurred())

				validateNetworkFields(disabledLinkNetwork, linkStateChkStatusValidation)

				By("removing sriov network")
				err = clients.Delete(context.Background(), disabledLinkNetwork)
				Expect(err).ToNot(HaveOccurred())

				By("configuring link-state as auto")
				autoLinkNetwork := sriovNetwork.DeepCopy()
				autoLinkNetwork.Spec.LinkState = "auto"
				linkStateChkStatusValidation = "link-state auto"
				err = clients.Create(context.Background(), autoLinkNetwork)
				Expect(err).ToNot(HaveOccurred())

				validateNetworkFields(autoLinkNetwork, linkStateChkStatusValidation)
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
				}, 10*time.Minute, 1*time.Second).Should(Equal(true))

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
		Context("MTU", func() {
			BeforeEach(func() {
				node := sriovInfos.Nodes[0]
				intf, err := sriovInfos.FindOneSriovDevice(node)
				Expect(err).ToNot(HaveOccurred())

				mtuPolicy := &sriovv1.SriovNetworkNodePolicy{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "mtupolicy",
						Namespace:    operatorNamespace,
					},

					Spec: sriovv1.SriovNetworkNodePolicySpec{
						NodeSelector: map[string]string{
							"kubernetes.io/hostname": node,
						},
						Mtu:          9000,
						NumVfs:       5,
						ResourceName: "mturesource",
						Priority:     99,
						NicSelector: sriovv1.SriovNetworkNicSelector{
							PfNames: []string{intf.Name},
						},
						DeviceType: "netdevice",
					},
				}

				err = clients.Create(context.Background(), mtuPolicy)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() bool {
					stable, err := cluster.SriovStable(operatorNamespace, clients)
					Expect(err).ToNot(HaveOccurred())
					return stable
				}, 2*time.Minute, 1*time.Second).Should(Equal(false))

				Eventually(func() bool {
					stable, err := cluster.SriovStable(operatorNamespace, clients)
					Expect(err).ToNot(HaveOccurred())
					return stable
				}, 10*time.Minute, 1*time.Second).Should(Equal(true))

				err = network.CreateSriovNetwork(clients,
					"mtuvolnetwork",
					namespaces.Test,
					operatorNamespace,
					"mturesource",
					`{"type":"host-local","subnet":"10.10.10.0/24","rangeStart":"10.10.10.171","rangeEnd":"10.10.10.181","routes":[{"dst":"0.0.0.0/0"}],"gateway":"10.10.10.1"}`)

				Expect(err).ToNot(HaveOccurred())

				Eventually(func() error {
					netAttDef := &netattdefv1.NetworkAttachmentDefinition{}
					return clients.Get(context.Background(), runtimeclient.ObjectKey{Name: "mtuvolnetwork", Namespace: namespaces.Test}, netAttDef)
				}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())

			})

			// 27662
			It("Should support jumbo frames", func() {
				podDefinition := pod.DefineWithNetworks([]string{"mtuvolnetwork"})
				firstPod, err := clients.Pods(namespaces.Test).Create(podDefinition)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() corev1.PodPhase {
					firstPod, _ = clients.Pods(namespaces.Test).Get(firstPod.Name, metav1.GetOptions{})
					return firstPod.Status.Phase
				}, 3*time.Minute, time.Second).Should(Equal(corev1.PodRunning))

				stdout, stderr, err := pod.ExecCommand(clients, firstPod, "ip", "link", "show", "net1")
				Expect(err).ToNot(HaveOccurred(), "Failed to show net1", stderr)
				Expect(stdout).To(ContainSubstring("mtu 9000"))
				firstPodIPs, err := network.GetSriovNicIPs(firstPod, "net1")
				Expect(err).ToNot(HaveOccurred())
				Expect(len(firstPodIPs)).To(Equal(1))

				podDefinition = pod.DefineWithNetworks([]string{"mtuvolnetwork"})
				secondPod, err := clients.Pods(namespaces.Test).Create(podDefinition)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() corev1.PodPhase {
					secondPod, _ = clients.Pods(namespaces.Test).Get(secondPod.Name, metav1.GetOptions{})
					return secondPod.Status.Phase
				}, 3*time.Minute, time.Second).Should(Equal(corev1.PodRunning))

				stdout, stderr, err = pod.ExecCommand(clients, secondPod,
					"ping", firstPodIPs[0], "-s", "8972", "-M", "do", "-c", "2")
				Expect(err).ToNot(HaveOccurred(), "Failed to ping first pod", stderr)
				Expect(stdout).To(ContainSubstring("2 packets transmitted, 2 received, 0% packet loss"))
			})
		})

	})
})

// findPodVFInHost goes over the virtual functions related to the physical function that was provided on the host
// and return the virtual function id if founds or error if not.
// return error also if more than one virtual functions is missing in the host network namespace
func findPodVFInHost(pfName string, numVfs int, podObj *corev1.Pod) (int, error) {
	found := false
	vfID := 0
	for idx := 0; idx < numVfs; idx++ {
		stdout, _, err := pod.ExecCommand(clients, podObj, "ip", "link", "show", fmt.Sprintf("%sv%d", pfName, idx))
		if err != nil && strings.Contains(stdout, "does not exist") {
			// Validate that only one virtual function was moved
			if found {
				return vfID, fmt.Errorf("found more that one virtual function was moved from the host network namespace")
			}

			found = true
			vfID = idx
		}
	}

	if found {
		return vfID, nil
	}

	return vfID, fmt.Errorf("failed to find the vf number that was moved into the pod")
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
