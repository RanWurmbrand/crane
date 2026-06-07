package e2e

import (
	"path/filepath"

	"github.com/konveyor/crane/e2e-tests/config"
	. "github.com/konveyor/crane/e2e-tests/framework"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cluster-level export filtering", func() {
	It("[CA-7] Should not export CRB with subject from another namespace", Label("cluster-admin"), func() {
		appName := "nginx-with-serviceaccount"
		testNamespace := "simple-nginx-nopv"
		forigenNamespace := "forigen-name-space"
		serviceName := "my-" + appName
		testSaName := "nginx-sa"
		forigenSa := "forigen-nginx-sa"
		testSubject := "--serviceaccount=" + testNamespace + ":" + testSaName
		forigenSubject := "--serviceaccount=" + forigenNamespace + ":" + forigenSa
		readClusterRole := "crane-e2e-pod-reader"
		readClusterRoleBindingName := "reader-crane-e2e-pod-binding"
		forigenClusterRoleBinding := "forigen-reader-pod-binding"
		relatedResources := []string{
			"ClusterRole_*" + readClusterRole + "*.yaml",
			"ClusterRoleBinding_*" + readClusterRoleBindingName + "*.yaml",
		}
		unrelatedResource := []string{"ClusterRoleBinding_" + forigenClusterRoleBinding + "*.yaml"}
		scenario := NewMigrationScenario(
			appName,
			testNamespace,
			config.K8sDeployBin,
			config.CraneBin,
			config.SourceContext,
			config.TargetContext,
		)
		srcApp := scenario.SrcApp
		tgtApp := scenario.TgtApp
		kubectlSrc := scenario.KubectlSrc
		kubectlTgt := scenario.KubectlTgt
		runner := scenario.Crane
		paths, err := NewScenarioPaths("crane-ca7-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			ResourceCleanup([]KubectlRunner{kubectlSrc, kubectlTgt}, []Deletable{
				ClusterRoleBinding{Name: readClusterRoleBindingName},
				ClusterRoleBinding{Name: forigenClusterRoleBinding},
				ClusterRole{Name: readClusterRole},
			})
		})

		DeferCleanup(func() {
			ScenarioCleanup(paths, srcApp, tgtApp, kubectlSrc, kubectlTgt, testNamespace)
		})

		DeferCleanup(func() {
			DeleteNameSpace(kubectlSrc, forigenNamespace)
		})

		By("Deploying app with ServiceAccount on source cluster")
		prepareSrcErr := PrepareSourceApp(srcApp, kubectlSrc)
		Expect(prepareSrcErr).NotTo(HaveOccurred())

		By("Creating foreign namespace on source")
		Expect(kubectlSrc.CreateNamespace(forigenNamespace)).NotTo(HaveOccurred())

		By("Creating ServiceAccount in foreign namespace")
		foreignSA := ServiceAccount{Name: forigenSa, Namespace: forigenNamespace}
		Expect(foreignSA.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating ClusterRole with pod read permissions")
		cr := ClusterRole{Name: readClusterRole, Permission: "read"}
		Expect(cr.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating ClusterRoleBinding referencing app's ServiceAccount")
		testCRB := ClusterRoleBinding{Name: readClusterRoleBindingName, ClusterRoleName: readClusterRole, Subject: testSubject}
		Expect(testCRB.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating ClusterRoleBinding referencing foreign namespace ServiceAccount")
		foreignCRB := ClusterRoleBinding{Name: forigenClusterRoleBinding, ClusterRoleName: readClusterRole, Subject: forigenSubject}
		Expect(foreignCRB.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Waiting for source pods and endpoints to drain")
		WaitForSourceQuiesce(kubectlSrc, testNamespace, "app="+appName, serviceName)

		By("Running crane export, transform, apply")
		Expect(RunPipeline(&runner, testNamespace, paths)).NotTo(HaveOccurred())

		By("Verifying foreign ClusterRoleBinding is not in export _cluster directory")
		exportClusterPath := filepath.Join(paths.ExportDir, "resources", testNamespace, "_cluster")
		//we dont expect to find the orphan clusterRole on the export dir.
		Expect(ValidateDirResources(exportClusterPath, unrelatedResource)).To(HaveOccurred())

		By("Verifying linked ClusterRole and ClusterRoleBinding exist in export, transform, and output")
		Expect(ValidatePipelineClusterResources(paths, testNamespace, relatedResources, nil)).NotTo(HaveOccurred())
	})
})
