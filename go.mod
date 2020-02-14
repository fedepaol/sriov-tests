module github.com/openshift/sriov-tests

go 1.13

require (
	github.com/ajeddeloh/go-json v0.0.0-20170920214419-6a2fe990e083 // indirect
	github.com/coreos/ignition v0.35.0 // indirect
	github.com/go-openapi/spec v0.19.2 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/intel/sriov-network-device-plugin v3.0.1-0.20191017093954-bf28fdc3e2d9+incompatible
	github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega v1.7.1
	github.com/openshift/api v3.9.1-0.20191213091414-3fbf6bcf78e8+incompatible // indirect
	github.com/openshift/client-go v0.0.0-20191205152420-9faca5198b4f
	github.com/openshift/machine-config-operator v4.2.0-alpha.0.0.20190917115525-033375cbe820+incompatible
	github.com/openshift/sriov-network-operator v0.0.0-20200201042246-2f3f3ce1e6eb
	github.com/operator-framework/operator-sdk v0.12.1-0.20191112211508-82fc57de5e5b
	github.com/vincent-petithory/dataurl v0.0.0-20191104211930-d1553a71de50 // indirect
	go4.org v0.0.0-20180809161055-417644f6feb5 // indirect
	golang.org/x/crypto v0.0.0-20200109152110-61a87790db17 // indirect
	golang.org/x/net v0.0.0-20191209160850-c0dbc17a3553 // indirect
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d // indirect
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	k8s.io/api v0.17.0
	k8s.io/apimachinery v0.17.0
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/utils v0.0.0-20200109141947-94aeca20bf09
	sigs.k8s.io/controller-runtime v0.3.0
)

replace (
	k8s.io/api => k8s.io/api v0.0.0-20190918195907-bd6ac527cfd2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190918201827-3de75813f604
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190817020851-f2f3a405f61d
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20190918200908-1e17798da8c1
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.0.0-20190918202139-0b14c719ca62
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190918200256-06eb1244587a
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.0.0-20190918203125-ae665f80358a
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.0.0-20190918202959-c340507a5d48
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20190612205613-18da4a14b22b
	k8s.io/component-base => k8s.io/component-base v0.0.0-20190918200425-ed2f0867c778
	k8s.io/cri-api => k8s.io/cri-api v0.0.0-20190817025403-3ae76f584e79
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.0.0-20190918203248-97c07dcbb623
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.0.0-20190918201136-c3a845f1fbb2
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.0.0-20190918202837-c54ce30c680e
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.0.0-20190918202429-08c8357f8e2d
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.0.0-20190918202713-c34a54b3ec8e
	k8s.io/kubelet => k8s.io/kubelet v0.0.0-20190918202550-958285cf3eef
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.0.0-20190918203421-225f0541b3ea
	k8s.io/metrics => k8s.io/metrics v0.0.0-20190918202012-3c1ca76f5bda
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.0.0-20190918201353-5cc279503896
)
