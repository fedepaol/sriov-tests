package operator

import (
	goctx "context"
	// "encoding/json"
	// "fmt"
	// "reflect"
	"strings"
	// "testing"
	"time"

	// dptypes "github.com/intel/sriov-network-device-plugin/pkg/types"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	// "github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	appsv1 "k8s.io/api/apps/v1"
	// corev1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/api/errors"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	// "k8s.io/apimachinery/pkg/util/wait"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"

	// "github.com/openshift/sriov-network-operator/pkg/apis"
	netattdefv1 "github.com/openshift/sriov-network-operator/pkg/apis/k8s/v1"
	sriovnetworkv1 "github.com/openshift/sriov-network-operator/pkg/apis/sriovnetwork/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	. "github.com/openshift/sriov-tests/pkg/util"
)

var _ = Describe("Operator", func() {
	BeforeEach(func() {
		// get global framework variables
		f := framework.Global
		// wait for sriov-network-operator to be ready
		deploy := &appsv1.Deployment{}
		err := WaitForNamespacedObject(deploy, f.Client, namespace, "sriov-network-operator", RetryInterval, Timeout)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("with SriovNetwork", func() {
		specs := map[string]sriovnetworkv1.SriovNetworkSpec{
			"test-0": {
				ResourceName: "resource_1",
				IPAM:         `{"type":"host-local","subnet":"10.56.217.0/24","rangeStart":"10.56.217.171","rangeEnd":"10.56.217.181","routes":[{"dst":"0.0.0.0/0"}],"gateway":"10.56.217.1"}`,
				Vlan:         100,
			},
			"test-1": {
				ResourceName:     "resource_1",
				IPAM:             `{"type":"host-local","subnet":"10.56.217.0/24","rangeStart":"10.56.217.171","rangeEnd":"10.56.217.181","routes":[{"dst":"0.0.0.0/0"}],"gateway":"10.56.217.1"}`,
				NetworkNamespace: "default",
			},
			"test-2": {
				ResourceName: "resource_1",
				IPAM:         `{"type":"host-local","subnet":"10.56.217.0/24","rangeStart":"10.56.217.171","rangeEnd":"10.56.217.181","routes":[{"dst":"0.0.0.0/0"}],"gateway":"10.56.217.1"}`,
				SpoofChk:     "on",
			},
			"test-3": {
				ResourceName: "resource_1",
				IPAM:         `{"type":"host-local","subnet":"10.56.217.0/24","rangeStart":"10.56.217.171","rangeEnd":"10.56.217.181","routes":[{"dst":"0.0.0.0/0"}],"gateway":"10.56.217.1"}`,
				Trust:        "on",
			},
			"test-4": {
				ResourceName: "resource_1",
				IPAM:         `{"type":"host-local","subnet":"10.56.217.0/24","rangeStart":"10.56.217.171","rangeEnd":"10.56.217.181","routes":[{"dst":"0.0.0.0/0"}],"gateway":"10.56.217.1"}`,
			},
		}
		sriovnets := GenerateSriovNetworkCRs(namespace, specs)
		DescribeTable("should be possible to create net-att-def",
			func(cr *sriovnetworkv1.SriovNetwork) {
				var err error
				expect := GenerateExpectedNetConfig(cr)

				By("Create the SriovNetwork Custom Resource")
				// get global framework variables
				f := framework.Global
				err = f.Client.Create(goctx.TODO(), cr, &framework.CleanupOptions{TestContext: &oprctx, Timeout: ApiTimeout, RetryInterval: RetryInterval})
				Expect(err).NotTo(HaveOccurred())
				ns := namespace
				if cr.Spec.NetworkNamespace != "" {
					ns = cr.Spec.NetworkNamespace
				}
				netAttDef := &netattdefv1.NetworkAttachmentDefinition{}
				err = WaitForNamespacedObject(netAttDef, f.Client, ns, cr.GetName(), RetryInterval, Timeout)
				Expect(err).NotTo(HaveOccurred())
				anno := netAttDef.GetAnnotations()

				Expect(anno["k8s.v1.cni.cncf.io/resourceName"]).To(Equal("openshift.io/" + cr.Spec.ResourceName))
				Expect(strings.TrimSpace(netAttDef.Spec.Config)).To(Equal(expect))

				By("Delete the SriovNetwork Custom Resource")
				found := &sriovnetworkv1.SriovNetwork{}
				err = f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: cr.GetNamespace(), Name: cr.GetName()}, found)
				Expect(err).NotTo(HaveOccurred())
				err = f.Client.Delete(goctx.TODO(), found, []dynclient.DeleteOption{}...)
				Expect(err).NotTo(HaveOccurred())

				netAttDef = &netattdefv1.NetworkAttachmentDefinition{}
				err = WaitForNamespacedObjectDeleted(netAttDef, f.Client, ns, cr.GetName(), RetryInterval, Timeout)
				Expect(err).NotTo(HaveOccurred())
			},
			Entry("with vlan flag", sriovnets[0]),
			Entry("with networkNamespace flag", sriovnets[1]),
			Entry("with SpoofChk flag on", sriovnets[2]),
			Entry("with Trust flag on", sriovnets[3]),
		)

		newSpecs := map[string]sriovnetworkv1.SriovNetworkSpec{
			"new-0": {
				ResourceName: "resource_1",
				IPAM:         `{"type":"dhcp"}`,
				Vlan:         200,
			},
			"new-1": {
				ResourceName: "resource_1",
				IPAM:         `{"type":"host-local","subnet":"10.56.217.0/24","rangeStart":"10.56.217.171","rangeEnd":"10.56.217.181","routes":[{"dst":"0.0.0.0/0"}],"gateway":"10.56.217.1"}`,
			},
			"new-2": {
				ResourceName: "resource_1",
				IPAM:         `{"type":"host-local","subnet":"10.56.217.0/24","rangeStart":"10.56.217.171","rangeEnd":"10.56.217.181","routes":[{"dst":"0.0.0.0/0"}],"gateway":"10.56.217.1"}`,
				SpoofChk:     "on",
			},
			"new-3": {
				ResourceName: "resource_1",
				IPAM:         `{"type":"host-local","subnet":"10.56.217.0/24","rangeStart":"10.56.217.171","rangeEnd":"10.56.217.181","routes":[{"dst":"0.0.0.0/0"}],"gateway":"10.56.217.1"}`,
				Trust:        "on",
			},
		}
		newsriovnets := GenerateSriovNetworkCRs(namespace, newSpecs)

		DescribeTable("should be possible to update net-att-def",
			func(old, new sriovnetworkv1.SriovNetwork) {
				f := framework.Global
				old.Name = new.GetName()
				err := f.Client.Create(goctx.TODO(), &old, &framework.CleanupOptions{TestContext: &oprctx, Timeout: ApiTimeout, RetryInterval: RetryInterval})
				Expect(err).NotTo(HaveOccurred())
				found := &sriovnetworkv1.SriovNetwork{}
				expect := GenerateExpectedNetConfig(&new)

				err = f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: old.GetNamespace(), Name: old.GetName()}, found)
				Expect(err).NotTo(HaveOccurred())
				found.Spec = new.Spec
				found.Annotations = new.Annotations
				err = f.Client.Update(goctx.TODO(), found)
				Expect(err).NotTo(HaveOccurred())
				ns := namespace
				if new.Spec.NetworkNamespace != "" {
					ns = new.Spec.NetworkNamespace
				}

				time.Sleep(time.Second * 1)
				netAttDef := &netattdefv1.NetworkAttachmentDefinition{}
				err = WaitForNamespacedObject(netAttDef, f.Client, ns, old.GetName(), RetryInterval, Timeout)
				Expect(err).NotTo(HaveOccurred())
				anno := netAttDef.GetAnnotations()

				Expect(anno["k8s.v1.cni.cncf.io/resourceName"]).To(Equal("openshift.io/" + new.Spec.ResourceName))
				Expect(strings.TrimSpace(netAttDef.Spec.Config)).To(Equal(expect))
			},
			Entry("with vlan flag and ipam updated", *sriovnets[4], *newsriovnets[0]),
			Entry("with networkNamespace flag", *sriovnets[4], *newsriovnets[1]),
			Entry("with SpoofChk flag on", *sriovnets[4], *newsriovnets[2]),
			Entry("with Trust flag on", *sriovnets[4], *newsriovnets[3]),
		)
	})
})
