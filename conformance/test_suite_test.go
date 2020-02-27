package conformance

import (
	"flag"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"

	sriovv1 "github.com/openshift/sriov-network-operator/pkg/apis/sriovnetwork/v1"
	testclient "github.com/openshift/sriov-tests/pkg/util/client"
	"github.com/openshift/sriov-tests/pkg/util/namespaces"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	junitPath         *string
	operatorNamespace string
	clients           *testclient.ClientSet
)

func init() {
	junitPath = flag.String("junit", "junit.xml", "the path for the junit format report")
	operatorNamespace = os.Getenv("OPERATOR_NAMESPACE")
	if operatorNamespace == "" {
		operatorNamespace = "openshift-sriov-network-operator"
	}
}

func TestTest(t *testing.T) {
	RegisterFailHandler(Fail)

	rr := []Reporter{}
	if junitPath != nil {
		rr = append(rr, reporters.NewJUnitReporter(*junitPath))
	}
	RunSpecsWithDefaultAndCustomReporters(t, "SRIOV Operator conformance tests", rr)
}

var _ = BeforeSuite(func() {
	clients = testclient.New("", func(scheme *runtime.Scheme) {
		sriovv1.AddToScheme(scheme)
	})

	// create test namespace
	err := namespaces.Create(namespaces.Test, clients)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	err := clients.Namespaces().Delete(namespaces.Test, &metav1.DeleteOptions{})
	Expect(err).ToNot(HaveOccurred())
	err = namespaces.WaitForDeletion(clients, namespaces.Test, 5*time.Minute)
})
