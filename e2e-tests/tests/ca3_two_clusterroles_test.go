package e2e

import (
	"path/filepath"

	"github.com/konveyor/crane/e2e-tests/config"
	. "github.com/konveyor/crane/e2e-tests/framework"
	"github.com/konveyor/crane/e2e-tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cluster-level RBAC export", func() {
	It("[CA-3] Should export two ClusterRoles and two ClusterRoleBindings for one Deployment", Label("cluster-admin"), func() {
		appName := "nginx-with-serviceaccount"
		namespace := "simple-nginx-nopv"
		serviceName := "my-" + appName
		saName := "nginx-sa"
		readClusterRole := "crane-e2e-pod-reader"
		writeClusterRole := "crane-e2e-pod-writer"
		readClusterRoleBindingName := "reader-crane-e2e-pod-binding"
		writeClusterRoleBindingName := "writer-crane-e2e-pod-binding"
		subject := "--serviceaccount=" + namespace + ":" + saName
		scenario := NewMigrationScenario(
			appName,
			namespace,
			config.K8sDeployBin,
			config.CraneBin,
			config.SourceContext,
			config.TargetContext,
		)
		srcApp := scenario.SrcApp
		tgtApp := scenario.TgtApp
		kubectlSrc := scenario.KubectlSrc
		kubectlTgt := scenario.KubectlTgt

		paths, err := NewScenarioPaths("crane-ca3-*")
		runner := scenario.Crane
		resourcesPatterns := []string{"ClusterRole", "ClusterRoleBinding"}
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			ResourceCleanup([]KubectlRunner{kubectlSrc, kubectlTgt}, []Resource{
				ClusterRoleBinding{Name: readClusterRoleBindingName},
				ClusterRoleBinding{Name: writeClusterRoleBindingName},
				ClusterRole{Name: readClusterRole},
				ClusterRole{Name: writeClusterRole},
			})
		})

		DeferCleanup(func() {
			ScenarioCleanup(paths, srcApp, tgtApp, kubectlSrc, kubectlTgt, namespace)
		})

		By("Deploying app with ServiceAccount on source cluster")
		Expect(PrepareSourceApp(srcApp, kubectlSrc)).NotTo(HaveOccurred())

		By("Creating ClusterRole with pod read permissions")
		readCR := ClusterRole{Name: readClusterRole, Permission: "read"}
		Expect(readCR.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating ClusterRole with pod write permissions")
		writeCR := ClusterRole{Name: writeClusterRole, Permission: "write"}
		Expect(writeCR.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating ClusterRoleBinding for read ClusterRole")
		readCRB := ClusterRoleBinding{Name: readClusterRoleBindingName, ClusterRoleName: readClusterRole, Subject: subject}
		Expect(readCRB.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating ClusterRoleBinding for write ClusterRole")
		writeCRB := ClusterRoleBinding{Name: writeClusterRoleBindingName, ClusterRoleName: writeClusterRole, Subject: subject}
		Expect(writeCRB.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Waiting for source pods and endpoints to drain")
		WaitForSourceQuiesce(kubectlSrc, namespace, "app="+appName, serviceName)

		By("Running crane export, transform, apply")
		Expect(RunCranePipelineWithChecks(runner, namespace, paths)).NotTo(HaveOccurred())

		By("Verifying no resources failed to export")
		failuresDir := filepath.Join(paths.ExportDir, "failures", namespace)
		hasFiles, _, err := utils.HasFilesRecursively(failuresDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasFiles).To(BeFalse())

		By("Verifying ClusterRoles and ClusterRoleBindings exist in export, transform, and output _cluster directories")
		Expect(ValidatePipelineClusterResources(paths, namespace, resourcesPatterns, nil)).NotTo(HaveOccurred())

		By("Dry-run applying output manifests on target")
		Expect(CreateNamespaceAndDryRun(kubectlTgt, namespace, paths.OutputDir)).NotTo(HaveOccurred())

		By("Applying migrated manifests to target cluster")
		Expect(ApplyOutputToTarget(kubectlTgt, namespace, paths.OutputDir)).NotTo(HaveOccurred())

		By("Scaling target deployment and validating app")
		Expect(kubectlTgt.ScaleDeployment(namespace, appName, 1)).NotTo(HaveOccurred())
		Eventually(tgtApp.Validate, "2m", "10s").Should(Succeed())

		By("Verifying both ClusterRoleBindings on target reference correct ClusterRoles and ServiceAccount")
		Expect(ValidateClusterRBAC(kubectlTgt, namespace, []ExpectedClusterRoleBinding{
			{ClusterRoleBindingName: readClusterRoleBindingName, ClusterRoleName: readClusterRole, SubjectName: saName},
			{ClusterRoleBindingName: writeClusterRoleBindingName, ClusterRoleName: writeClusterRole, SubjectName: saName},
		})).NotTo(HaveOccurred())
	})
})
