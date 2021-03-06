package e2e

import (
	goctx "context"
	// "reflect"
	// "strings"
	// "testing"
	"time"
	"strconv"

	// dptypes "github.com/intel/sriov-network-device-plugin/pkg/types"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	// "github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	// admv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// "k8s.io/apimachinery/pkg/types"
	// "k8s.io/apimachinery/pkg/util/wait"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"

	// "github.com/openshift/sriov-network-operator/pkg/apis"
	// netattdefv1 "github.com/openshift/sriov-network-operator/pkg/apis/k8s/v1"
	sriovnetworkv1 "github.com/openshift/sriov-network-operator/pkg/apis/sriovnetwork/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	. "github.com/openshift/sriov-tests/pkg/util"
)

var _ = Describe("Operator", func() {

	// BeforeEach(func() {
	// 	// get global framework variables
	// 	f := framework.Global
	// 	var err error

	// 	// Turn off Operator Webhook
	// 	// config := &sriovnetworkv1.SriovOperatorConfig{}
	// 	// err = WaitForNamespacedObject(config, f.Client, namespace, "default", RetryInterval, Timeout)
	// 	// Expect(err).NotTo(HaveOccurred())

	// 	// *config.Spec.EnableInjector = false
	// 	// err = f.Client.Update(goctx.TODO(), config)
	// 	// Expect(err).NotTo(HaveOccurred())

	// 	// daemonSet := &appsv1.DaemonSet{}
	// 	// err = WaitForNamespacedObjectDeleted(daemonSet, f.Client, namespace, "network-resources-injector", RetryInterval, Timeout)
	// 	// Expect(err).NotTo(HaveOccurred())

	// 	// mutateCfg := &admv1beta1.MutatingWebhookConfiguration{}
	// 	// err = WaitForNamespacedObjectDeleted(mutateCfg, f.Client, namespace, "network-resources-injector-config", RetryInterval, Timeout)
	// 	// Expect(err).NotTo(HaveOccurred())
	// })

	Context("with single policy", func() {
		policy1 := &sriovnetworkv1.SriovNetworkNodePolicy{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SriovNetworkNodePolicy",
				APIVersion: "sriovnetwork.openshift.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-1",
				Namespace: namespace,
			},
			Spec: sriovnetworkv1.SriovNetworkNodePolicySpec{
				ResourceName: "resource_1",
				NodeSelector: map[string]string{
					"feature.node.kubernetes.io/network-sriov.capable": "true",
				},
				Priority: 99,
				Mtu:      9000,
				NumVfs:   6,
				NicSelector: sriovnetworkv1.SriovNetworkNicSelector{
					Vendor:      "8086",
					RootDevices: []string{"0000:86:00.1"},
				},
				DeviceType: "vfio-pci",
			},
		}
		policy2 := &sriovnetworkv1.SriovNetworkNodePolicy{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SriovNetworkNodePolicy",
				APIVersion: "sriovnetwork.openshift.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-2",
				Namespace: namespace,
			},
			Spec: sriovnetworkv1.SriovNetworkNodePolicySpec{
				ResourceName: "resource_2",
				NodeSelector: map[string]string{
					"feature.node.kubernetes.io/network-sriov.capable": "true",
				},
				Priority: 99,
				Mtu:      9000,
				NumVfs:   6,
				NicSelector: sriovnetworkv1.SriovNetworkNicSelector{
					Vendor:      "8086",
					RootDevices: []string{"0000:86:00.1"},
				},
			},
		}

		JustBeforeEach(func() {
			// get global framework variables
			oprctx.Cleanup()
		})

		DescribeTable("should config sriov", 
			func(policy *sriovnetworkv1.SriovNetworkNodePolicy) {
				// get global framework variables
				f := framework.Global
				var err error
				By("wait for the node state ready")
				nodeList := &corev1.NodeList{}
				lo := &dynclient.MatchingLabels{
					"feature.node.kubernetes.io/network-sriov.capable": "true",
				}
				err = f.Client.List(goctx.TODO(), nodeList, lo)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(nodeList.Items)).To(Equal(1))

				name := nodeList.Items[0].GetName()
				nodeState := &sriovnetworkv1.SriovNetworkNodeState{}
				err = WaitForSriovNetworkNodeStateReady(nodeState, f.Client, namespace, name, RetryInterval, Timeout*3)
				Expect(err).NotTo(HaveOccurred())

				By("apply node policy CR")
				err = f.Client.Create(goctx.TODO(), policy, &framework.CleanupOptions{TestContext: &oprctx, Timeout: ApiTimeout, RetryInterval: RetryInterval})
				Expect(err).NotTo(HaveOccurred())

				By("generate the config for device plugin")
				time.Sleep(3 * time.Second)
				config := &corev1.ConfigMap{}
				err = WaitForNamespacedObject(config, f.Client, namespace, "device-plugin-config", RetryInterval, Timeout)
				Expect(err).NotTo(HaveOccurred())
				err = ValidateDevicePluginConfig(policy, config.Data["config.json"])

				By("wait for the node state ready")
				err = WaitForSriovNetworkNodeStateReady(nodeState, f.Client, namespace, name, RetryInterval, Timeout*3)
				Expect(err).NotTo(HaveOccurred())

				By("provision the cni and device plugin daemonsets")
				cniDaemonSet := &appsv1.DaemonSet{}
				err = WaitForDaemonSetReady(cniDaemonSet, f.Client, namespace, "sriov-cni", RetryInterval, Timeout)
				Expect(err).NotTo(HaveOccurred())

				dpDaemonSet := &appsv1.DaemonSet{}
				err = WaitForDaemonSetReady(dpDaemonSet, f.Client, namespace, "sriov-device-plugin", RetryInterval, Timeout)
				Expect(err).NotTo(HaveOccurred())

				By("update the spec of SriovNetworkNodeState CR")
				found := false
				for _, address := range policy.Spec.NicSelector.RootDevices {
					for _, iface := range nodeState.Spec.Interfaces {
						if iface.PciAddress == address {
							found = true
							Expect(iface.NumVfs).To(Equal(policy.Spec.NumVfs))
							Expect(iface.Mtu).To(Equal(policy.Spec.Mtu))
							Expect(iface.VfGroups[0].DeviceType).To(Equal(policy.Spec.DeviceType))
							Expect(iface.VfGroups[0].ResourceName).To(Equal(policy.Spec.ResourceName))
						}
					}
				}
				Expect(found).To(BeTrue())

				By("update the status of SriovNetworkNodeState CR")
				found = false
				for _, address := range policy.Spec.NicSelector.RootDevices {
					for _, iface := range nodeState.Status.Interfaces {
						if iface.PciAddress == address {
							found = true
							Expect(iface.NumVfs).To(Equal(policy.Spec.NumVfs))
							Expect(iface.Mtu).To(Equal(policy.Spec.Mtu))
							Expect(len(iface.VFs)).To(Equal(policy.Spec.NumVfs))
							for _, vf := range iface.VFs {
								if policy.Spec.DeviceType == "netdevice" || policy.Spec.DeviceType == ""{
									Expect(vf.Mtu).To(Equal(policy.Spec.Mtu))
								}
								if policy.Spec.DeviceType == "vfio" {
									Expect(vf.Driver).To(Equal(policy.Spec.DeviceType))
								} 
							}
							break
						}
					}
				}
				Expect(found).To(BeTrue())
			},
			Entry("Set MTU and vfio driver", policy1),
			Entry("Set MTU and default driver", policy2),
		)
	})

	Context("with single vf index policy", func() {
		policy1 := &sriovnetworkv1.SriovNetworkNodePolicy{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SriovNetworkNodePolicy",
				APIVersion: "sriovnetwork.openshift.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-1",
				Namespace: namespace,
			},
			Spec: sriovnetworkv1.SriovNetworkNodePolicySpec{
				ResourceName: "resource_1",
				NodeSelector: map[string]string{
					"feature.node.kubernetes.io/network-sriov.capable": "true",
				},
				Priority: 99,
				Mtu:      9000,
				NumVfs:   6,
				NicSelector: sriovnetworkv1.SriovNetworkNicSelector{
					PfNames:     []string{"ens803f1#0-5"},
					Vendor:      "8086",
					RootDevices: []string{"0000:86:00.1"},
				},
				DeviceType: "vfio-pci",
			},
		}
		policy2 := &sriovnetworkv1.SriovNetworkNodePolicy{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SriovNetworkNodePolicy",
				APIVersion: "sriovnetwork.openshift.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-1",
				Namespace: namespace,
			},
			Spec: sriovnetworkv1.SriovNetworkNodePolicySpec{
				ResourceName: "resource_1",
				NodeSelector: map[string]string{
					"feature.node.kubernetes.io/network-sriov.capable": "true",
				},
				Priority: 99,
				Mtu:      9000,
				NumVfs:   6,
				NicSelector: sriovnetworkv1.SriovNetworkNicSelector{
					PfNames:     []string{"ens803f1#0-0"},
					Vendor:      "8086",
					RootDevices: []string{"0000:86:00.1"},
				},
			},
		}

		JustBeforeEach(func() {
			// get global framework variables
			oprctx.Cleanup()
		})

		DescribeTable("should config sriov", 
			func(policy *sriovnetworkv1.SriovNetworkNodePolicy) {
				// get global framework variables
				f := framework.Global
				var err error
				By("wait for the node state ready")
				nodeList := &corev1.NodeList{}
				lo := &dynclient.MatchingLabels{
					"feature.node.kubernetes.io/network-sriov.capable": "true",
				}
				err = f.Client.List(goctx.TODO(), nodeList, lo)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(nodeList.Items)).To(Equal(1))

				name := nodeList.Items[0].GetName()
				nodeState := &sriovnetworkv1.SriovNetworkNodeState{}
				err = WaitForSriovNetworkNodeStateReady(nodeState, f.Client, namespace, name, RetryInterval, Timeout*3)
				Expect(err).NotTo(HaveOccurred())

				By("apply node policy CR")
				err = f.Client.Create(goctx.TODO(), policy, &framework.CleanupOptions{TestContext: &oprctx, Timeout: ApiTimeout, RetryInterval: RetryInterval})
				Expect(err).NotTo(HaveOccurred())

				By("generate the config for device plugin")
				time.Sleep(3 * time.Second)
				config := &corev1.ConfigMap{}
				err = WaitForNamespacedObject(config, f.Client, namespace, "device-plugin-config", RetryInterval, Timeout)
				Expect(err).NotTo(HaveOccurred())
				err = ValidateDevicePluginConfig(policy, config.Data["config.json"])

				By("wait for the node state ready")
				err = WaitForSriovNetworkNodeStateReady(nodeState, f.Client, namespace, name, RetryInterval, Timeout*3)
				Expect(err).NotTo(HaveOccurred())

				By("provision the cni and device plugin daemonsets")
				cniDaemonSet := &appsv1.DaemonSet{}
				err = WaitForDaemonSetReady(cniDaemonSet, f.Client, namespace, "sriov-cni", RetryInterval, Timeout)
				Expect(err).NotTo(HaveOccurred())

				dpDaemonSet := &appsv1.DaemonSet{}
				err = WaitForDaemonSetReady(dpDaemonSet, f.Client, namespace, "sriov-device-plugin", RetryInterval, Timeout)
				Expect(err).NotTo(HaveOccurred())

				By("update the spec of SriovNetworkNodeState CR")
				found := false
				for _, address := range policy.Spec.NicSelector.RootDevices {
					for _, iface := range nodeState.Spec.Interfaces {
						if iface.PciAddress == address {
							found = true
							Expect(iface.NumVfs).To(Equal(policy.Spec.NumVfs))
							Expect(iface.Mtu).To(Equal(policy.Spec.Mtu))
							Expect(iface.VfGroups[0].DeviceType).To(Equal(policy.Spec.DeviceType))
							Expect(iface.VfGroups[0].ResourceName).To(Equal(policy.Spec.ResourceName))
							
							pfName, rngStart, rngEnd, err := sriovnetworkv1.ParsePFName(policy.Spec.NicSelector.PfNames[0])
							Expect(err).NotTo(HaveOccurred())
							rng := strconv.Itoa(rngStart) + "-" + strconv.Itoa(rngEnd)
							Expect(iface.Name).To(Equal(pfName))
							Expect(iface.VfGroups[0].VfRange).To(Equal(rng))
						}
					}
				}
				Expect(found).To(BeTrue())

				By("update the status of SriovNetworkNodeState CR")
				found = false
				for _, address := range policy.Spec.NicSelector.RootDevices {
					for _, iface := range nodeState.Status.Interfaces {
						if iface.PciAddress == address {
							found = true
							Expect(iface.NumVfs).To(Equal(policy.Spec.NumVfs))
							Expect(iface.Mtu).To(Equal(policy.Spec.Mtu))
							Expect(len(iface.VFs)).To(Equal(policy.Spec.NumVfs))
							for _, vf := range iface.VFs {
								if policy.Spec.DeviceType == "netdevice" || policy.Spec.DeviceType == ""{
									Expect(vf.Mtu).To(Equal(policy.Spec.Mtu))
								}
								if policy.Spec.DeviceType == "vfio" {
									Expect(vf.Driver).To(Equal(policy.Spec.DeviceType))
								} 
							}
							break
						}
					}
				}
				Expect(found).To(BeTrue())
			},
			Entry("Set one PF with VF range", policy1),
			Entry("Set one PF with VF range", policy2),
		)
	})
})
