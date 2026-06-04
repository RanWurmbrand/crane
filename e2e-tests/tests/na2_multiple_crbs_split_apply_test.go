package e2e

import (
	"path/filepath"

	"github.com/konveyor/crane/e2e-tests/config"
	. "github.com/konveyor/crane/e2e-tests/framework"
	"github.com/konveyor/crane/e2e-tests/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Namespace-admin cluster-level migration", func() {
	It("[NA-2] Should migrate workload with one CR and two CRBs using split apply", Label("namespace-admin"), func() {
		appName := "simple-nginx-nopv"
		namespace := "simple-nginx-nopv"
		serviceName := "my-" + appName
		firstSa := "nginx-sa"
		secondSa := "nginx-sa-2"
		clusterRoleName := "crane-e2e-pod-reader"
		firstClusterRoleBindingName := "crane-e2e-pod-reader-binding-1"
		secondClusterRoleBindingName := "crane-e2e-pod-reader-binding-2"
		baseSubject := "--serviceaccount=" + namespace + ":"
		scenario := NewMigrationScenario(
			appName,
			namespace,
			config.K8sDeployBin,
			config.CraneBin,
			config.SourceContext,
			config.TargetContext,
		)
		if scenario.KubectlSrcNonAdmin.Context == "" {
			Skip("source-nonadmin-context is required for non-admin role migration test")
		}
		if scenario.KubectlTgtNonAdmin.Context == "" {
			Skip("target-nonadmin-context is required for non-admin role migration test")
		}
		srcAppNonAdmin, tgtAppNonAdmin := scenario.NonAdminApps()
		kubectlSrc := scenario.KubectlSrc
		kubectlTgt := scenario.KubectlTgt
		adminRunner := scenario.Crane
		nonAdminRunner := scenario.CraneNonAdmin
		nonAdminPaths, err := NewScenarioPaths("crane-na2-non-admin*")
		Expect(err).NotTo(HaveOccurred())
		adminPaths, err := NewScenarioPaths("crane-na2-admin*")
		Expect(err).NotTo(HaveOccurred())
		clusterPatterns := []string{"ClusterRole_*.yaml", "ClusterRoleBinding_*.yaml"}
		By("Granting namespace-admin permissions to non-admin user on source and target")
		kubectlSrcNonAdmin, kubectlTgtNonAdmin, rbacCleanup, err := SetupNamespaceAdminUsersForScenario(scenario, namespace)
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(rbacCleanup)
		DeferCleanup(func() {
			ResourceCleanup([]KubectlRunner{kubectlSrc, kubectlTgt}, []Resource{
				ClusterRoleBinding{Name: firstClusterRoleBindingName},
				ClusterRoleBinding{Name: secondClusterRoleBindingName},
				ClusterRole{Name: clusterRoleName},
			})
		})
		DeferCleanup(func() {
			ScenarioCleanup(nonAdminPaths, srcAppNonAdmin, tgtAppNonAdmin, kubectlSrc, kubectlTgt, namespace)
			ScenarioCleanup(adminPaths, srcAppNonAdmin, tgtAppNonAdmin, kubectlSrc, kubectlTgt, namespace)
		})

		By("Deploying app as namespace-admin on source cluster")
		err = PrepareSourceApp(srcAppNonAdmin, kubectlSrcNonAdmin)
		Expect(err).NotTo(HaveOccurred())

		By("Creating ClusterRole as cluster-admin")
		cr := ClusterRole{Name: clusterRoleName, Permission: "read"}
		Expect(cr.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating two ServiceAccounts as namespace-admin")
		sa1 := ServiceAccount{Name: firstSa, Namespace: namespace}
		Expect(sa1.Create(kubectlSrcNonAdmin)).NotTo(HaveOccurred())
		sa2 := ServiceAccount{Name: secondSa, Namespace: namespace}
		Expect(sa2.Create(kubectlSrcNonAdmin)).NotTo(HaveOccurred())

		By("Creating first ClusterRoleBinding as cluster-admin")
		crb1 := ClusterRoleBinding{Name: firstClusterRoleBindingName, ClusterRoleName: clusterRoleName, Subject: baseSubject + firstSa}
		Expect(crb1.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating second ClusterRoleBinding as cluster-admin")
		crb2 := ClusterRoleBinding{Name: secondClusterRoleBindingName, ClusterRoleName: clusterRoleName, Subject: baseSubject + secondSa}
		Expect(crb2.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Waiting for source pods and endpoints to drain")
		WaitForSourceQuiesce(kubectlSrcNonAdmin, namespace, "app="+appName, serviceName)

		By("Running crane export, transform, apply as cluster-admin")
		Expect(RunCranePipelineWithChecks(adminRunner, namespace, adminPaths)).NotTo(HaveOccurred())

		By("Verifying no resources failed to export as cluster-admin")
		failuresDir := filepath.Join(adminPaths.ExportDir, "failures", namespace)
		hasFiles, _, err := utils.HasFilesRecursively(failuresDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasFiles).To(BeFalse())

		By("Verifying ClusterRole and ClusterRoleBindings exist in export, transform, and output")
		Expect(ValidatePipelineClusterResources(adminPaths, namespace, clusterPatterns, nil)).NotTo(HaveOccurred())

		By("Dry-run applying output manifests on target")
		Expect(kubectlTgt.ValidateApplyDir(adminPaths.OutputDir)).NotTo(HaveOccurred())

		By("Applying cluster resources to target as cluster-admin")
		Expect(ApplyOutputToTarget(kubectlTgt, namespace, filepath.Join(adminPaths.OutputDir,
			"resources", "_cluster"))).NotTo(HaveOccurred())

		By("Verifying both ClusterRoleBindings on target reference correct ClusterRole and ServiceAccounts")
		Expect(ValidateClusterRBAC(kubectlTgt, namespace, []ExpectedClusterRoleBinding{
			{ClusterRoleBindingName: firstClusterRoleBindingName, ClusterRoleName: clusterRoleName, SubjectName: firstSa},
			{ClusterRoleBindingName: secondClusterRoleBindingName, ClusterRoleName: clusterRoleName, SubjectName: secondSa},
		})).NotTo(HaveOccurred())

		By("Running crane export, transform, apply as namespace-admin")
		Expect(RunCranePipelineWithChecks(nonAdminRunner, namespace, nonAdminPaths)).NotTo(HaveOccurred())

		By("Verifying cluster resources are in failures directory (expected for namespace-admin)")
		NonAdminfailuresDir := filepath.Join(nonAdminPaths.ExportDir, "failures", namespace)
		//cluster resources
		Expect(utils.AssertKindsInOutput(NonAdminfailuresDir, []string{"CustomResourceDefinition", "ClusterRole"})).NotTo(HaveOccurred())

		By("Verifying no cluster resources in output _cluster directory")
		utils.AssertNoKindsInOutput(filepath.Join(nonAdminPaths.OutputDir, "resources", "_cluster"), []string{"ClusterRole", "ClusterRoleBinding"})

		By("Applying namespace resources to target as namespace-admin")
		Expect(NonAdminApplyOutput(kubectlTgtNonAdmin, nonAdminPaths.OutputDir, namespace)).NotTo(HaveOccurred())

		By("Scaling target deployment and validating app")
		Expect(kubectlTgtNonAdmin.ScaleDeployment(namespace, appName, 1)).NotTo(HaveOccurred())
		Eventually(tgtAppNonAdmin.Validate, "2m", "10s").Should(Succeed())
	})

})
