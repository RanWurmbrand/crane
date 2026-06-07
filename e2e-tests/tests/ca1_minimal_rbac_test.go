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
	It("[CA-1] Should export ClusterRole and ClusterRoleBinding for linked ServiceAccount", Label("cluster-admin"), func() {
		appName := "nginx-with-serviceaccount"
		namespace := "simple-nginx-nopv"
		serviceName := "my-" + appName
		saName := "nginx-sa"
		subject := "--serviceaccount=" + namespace + ":" + saName
		clusterRoleBindingName := "crane-e2e-pod-reader-binding"
		clusterRoleName := "crane-e2e-pod-reader"
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
		paths, err := NewScenarioPaths("crane-ca1-*")
		resourcesPatterns := []string{"ClusterRole_*.yaml", "ClusterRoleBinding_*.yaml"}
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() {
			ResourceCleanup([]KubectlRunner{kubectlSrc, kubectlTgt}, []Deletable{
				ClusterRoleBinding{Name: clusterRoleBindingName},
				ClusterRole{Name: clusterRoleName},
			})
		})

		DeferCleanup(func() {
			ScenarioCleanup(paths, srcApp, tgtApp, kubectlSrc, kubectlTgt, namespace)
		})

		By("Deploying app with ServiceAccount on source cluster")
		Expect(PrepareSourceApp(srcApp, kubectlSrc)).NotTo(HaveOccurred())

		By("Creating ClusterRole with pod read permissions")
		_, err = scenario.KubectlSrc.Run("create", "clusterrole", clusterRoleName, "--verb=get,list,watch", "--resource=pods")
		Expect(err).NotTo(HaveOccurred())

		By("Creating ClusterRoleBinding that references the app's ServiceAccount")
		crb := ClusterRoleBinding{Name: clusterRoleBindingName, ClusterRoleName: clusterRoleName, Subject: subject}
		Expect(crb.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Waiting for source pods and endpoints to drain")
		WaitForSourceQuiesce(kubectlSrc, namespace, "app="+appName, serviceName)

		By("Running crane export, transform, apply")
		Expect(RunPipeline(&runner, namespace, paths)).NotTo(HaveOccurred())

		By("Verifying no resources failed to export")
		failuresDir := filepath.Join(paths.ExportDir, "failures", namespace)
		hasFiles, _, err := utils.HasFilesRecursively(failuresDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasFiles).To(BeFalse())

		By("Verifying ClusterRole and ClusterRoleBinding exist in export, transform, and output _cluster directories")
		Expect(ValidatePipelineClusterResources(paths, namespace, resourcesPatterns, nil)).NotTo(HaveOccurred())

		By("Dry-run applying output manifests on target")
		Expect(kubectlTgt.CreateNamespace(namespace)).NotTo(HaveOccurred())
		Expect(kubectlTgt.ValidateApplyDir(paths.OutputDir)).NotTo(HaveOccurred())

		By("Applying migrated manifests to target cluster")
		validateErr := ApplyOutputToTarget(kubectlTgt, namespace, paths.OutputDir)
		Expect(validateErr).NotTo(HaveOccurred())

		By("Scaling target deployment and validating app")
		ScaleAndValidateTargetApp(kubectlTgt, tgtApp, namespace, appName)

		By("Verifying ClusterRoleBinding on target references correct ClusterRole and ServiceAccount")
		Expect(ValidateClusterRBAC(kubectlTgt, namespace, []ExpectedClusterRoleBinding{
			{ClusterRoleBindingName: clusterRoleBindingName, ClusterRoleName: clusterRoleName, SubjectName: saName},
		})).NotTo(HaveOccurred())
	})

})
