package e2e

import (
	"path/filepath"

	"github.com/konveyor/crane/e2e-tests/config"
	. "github.com/konveyor/crane/e2e-tests/framework"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Namespace-admin cluster-level migration", func() {
	It("[NA-3] Should export namespace resources and record failures under limited RBAC", Label("namespace-admin"), func() {
		appName := "simple-nginx-nopv"
		namespace := "simple-nginx-nopv"
		serviceName := "my-" + appName
		subject := "--serviceaccount=" + namespace + ":default"
		scenario := NewMigrationScenario(
			appName,
			namespace,
			config.K8sDeployBin,
			config.CraneBin,
			config.SourceContext,
			config.TargetContext,
		)
		clusterRoleBindingName := "crane-e2e-pod-reader-binding"
		clusterRoleName := "crane-e2e-pod-reader"
		forbiddenResourcesPatterns := []string{"clusterroles.yaml", "clusterrolebindings.yaml"}
		if scenario.KubectlSrcNonAdmin.Context == "" {
			Skip("source-nonadmin-context is required for non-admin role migration test")
		}
		if scenario.KubectlTgtNonAdmin.Context == "" {
			Skip("target-nonadmin-context is required for non-admin role migration test")
		}
		srcAppNonAdmin, tgtAppNonAdmin := NonAdminApps(scenario)
		kubectlSrc := scenario.KubectlSrc
		kubectlTgt := scenario.KubectlTgt
		paths, err := NewScenarioPaths("crane-na3-*")
		runner := scenario.CraneNonAdmin
		Expect(err).NotTo(HaveOccurred())

		By("Granting namespace-admin permissions to non-admin user on source and target")
		kubectlSrcNonAdmin, _, rbacCleanup, err := SetupNamespaceAdminUsersForScenario(scenario, namespace)
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(rbacCleanup)
		DeferCleanup(func() {
			ResourceCleanup([]KubectlRunner{kubectlSrc, kubectlTgt}, []Deletable{
				ClusterRoleBinding{Name: clusterRoleBindingName},
				ClusterRole{Name: clusterRoleName},
			})
		})
		DeferCleanup(func() {
			ScenarioCleanup(paths, srcAppNonAdmin, tgtAppNonAdmin, kubectlSrc, kubectlTgt, namespace)
		})

		By("Deploying app as namespace-admin on source cluster")
		err = PrepareSourceApp(srcAppNonAdmin, kubectlSrcNonAdmin)
		Expect(err).NotTo(HaveOccurred())

		By("Creating ClusterRole as cluster-admin")
		cr := ClusterRole{Name: clusterRoleName, Permission: "read"}
		Expect(cr.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Creating ClusterRoleBinding as cluster-admin")
		crb := ClusterRoleBinding{Name: clusterRoleBindingName, ClusterRoleName: clusterRoleName, Subject: subject}
		Expect(crb.Create(kubectlSrc)).NotTo(HaveOccurred())

		By("Waiting for source pods and endpoints to drain")
		WaitForSourceQuiesce(kubectlSrcNonAdmin, namespace, "app="+appName, serviceName)

		By("Running crane export, transform, apply as namespace-admin")
		Expect(RunPipeline(&runner, namespace, paths)).NotTo(HaveOccurred())

		By("Verifying cluster resources are recorded in failures directory")
		err = ValidateDirResources(filepath.Join(paths.ExportDir, "failures", namespace), forbiddenResourcesPatterns)
		Expect(err).NotTo(HaveOccurred())

	})

})
