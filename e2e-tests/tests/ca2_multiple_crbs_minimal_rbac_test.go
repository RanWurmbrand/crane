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
	It("[CA-2] Should export ClusterRole and ClusterRoleBinding for linked ServiceAccount", Label("cluster-admin"), func() {
		appName := "nginx-with-serviceaccount"
		namespace := "simple-nginx-nopv"
		serviceName := "my-" + appName
		firstSa := "nginx-sa"
		secondSa := "nginx-sa-2"
		baseSubject := "--serviceaccount=" + namespace + ":"
		clusterRoleName := "crane-e2e-pod-reader"
		firstClusterRoleBindingName := "crane-e2e-pod-reader-binding-1"
		secondClusterRoleBindingName := "crane-e2e-pod-reader-binding-2"
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
		runner := scenario.Crane
		paths, err := NewScenarioPaths("crane-ca2-*")

		resourcesPatterns := []string{"ClusterRole_*.yaml", "ClusterRoleBinding_*.yaml"}
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			ResourceCleanup([]KubectlRunner{kubectlSrc, kubectlTgt}, []Deletable{
				ClusterRoleBinding{Name: firstClusterRoleBindingName},
				ClusterRoleBinding{Name: secondClusterRoleBindingName},
				ClusterRole{Name: clusterRoleName},
			})
		})

		DeferCleanup(func() {
			ScenarioCleanup(paths, srcApp, tgtApp, kubectlSrc, kubectlTgt, namespace)
		})

		By("Deploying app with ServiceAccount on source cluster")
		prepareSrcErr := PrepareSourceApp(srcApp, kubectlSrc)
		Expect(prepareSrcErr).NotTo(HaveOccurred())

		By("Creating second ServiceAccount in namespace")
		_, err = kubectlSrc.Run("create", "serviceaccount", secondSa, "-n", namespace)
		Expect(err).NotTo(HaveOccurred())

		By("Creating ClusterRole with pod read permissions")
		cr := ClusterRole{Name: clusterRoleName, Permission: "read"}
		Expect(cr.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating first ClusterRoleBinding referencing first ServiceAccount")
		crb1 := ClusterRoleBinding{Name: firstClusterRoleBindingName, ClusterRoleName: clusterRoleName, Subject: baseSubject + firstSa}
		Expect(crb1.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating second ClusterRoleBinding referencing second ServiceAccount")
		crb2 := ClusterRoleBinding{Name: secondClusterRoleBindingName, ClusterRoleName: clusterRoleName, Subject: baseSubject + secondSa}
		Expect(crb2.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Waiting for source pods and endpoints to drain")
		WaitForSourceQuiesce(kubectlSrc, namespace, "app="+appName, serviceName)

		By("Running crane export, transform, apply")
		Expect(RunPipeline(&runner, namespace, paths)).NotTo(HaveOccurred())

		By("Verifying no resources failed to export")
		failuresDir := filepath.Join(paths.ExportDir, "failures", namespace)
		hasFiles, _, err := utils.HasFilesRecursively(failuresDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasFiles).To(BeFalse())

		By("Verifying ClusterRole and ClusterRoleBindings exist in export, transform, and output _cluster directories")
		Expect(ValidatePipelineClusterResources(paths, namespace, resourcesPatterns, nil)).NotTo(HaveOccurred())

		By("Dry-run applying output manifests on target")
		Expect(CreateNamespaceAndDryRun(kubectlTgt, namespace, paths.OutputDir)).NotTo(HaveOccurred())

		By("Applying migrated manifests to target cluster")
		validateErr := ApplyOutputToTarget(kubectlTgt, namespace, paths.OutputDir)
		Expect(validateErr).NotTo(HaveOccurred())

		By("Scaling target deployment and validating app")
		ScaleAndValidateTargetApp(kubectlTgt, tgtApp, namespace, appName)

		By("Verifying both ClusterRoleBindings on target reference correct ClusterRole and ServiceAccounts")
		Expect(ValidateClusterRBAC(kubectlTgt, namespace, []ExpectedClusterRoleBinding{
			{ClusterRoleBindingName: firstClusterRoleBindingName, ClusterRoleName: clusterRoleName, SubjectName: firstSa},
			{ClusterRoleBindingName: secondClusterRoleBindingName, ClusterRoleName: clusterRoleName, SubjectName: secondSa},
		})).NotTo(HaveOccurred())
	})
})
