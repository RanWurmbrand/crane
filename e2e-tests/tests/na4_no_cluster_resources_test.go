package e2e

import (
	"path/filepath"

	"github.com/konveyor/crane/e2e-tests/config"
	. "github.com/konveyor/crane/e2e-tests/framework"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Namespace-admin cluster-level migration", func() {
	It("[NA-4] Should migrate namespace-only workload as namespace-admin without split apply", Label("namespace-admin"), func() {
		appName := "simple-nginx-nopv"
		namespace := "simple-nginx-nopv"
		serviceName := "my-" + appName
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
		srcAppNonAdmin, tgtAppNonAdmin := NonAdminApps(scenario)
		kubectlSrc := scenario.KubectlSrc
		kubectlTgt := scenario.KubectlTgt
		paths, err := NewScenarioPaths("crane-na4-*")
		runner := scenario.CraneNonAdmin
		Expect(err).NotTo(HaveOccurred())

		By("Granting namespace-admin permissions to non-admin user on source and target")
		kubectlSrcNonAdmin, kubectlTgtNonAdmin, rbacCleanup, err := SetupNamespaceAdminUsersForScenario(scenario, namespace)
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(rbacCleanup)
		DeferCleanup(func() {
			ScenarioCleanup(paths, srcAppNonAdmin, tgtAppNonAdmin, kubectlSrc, kubectlTgt, namespace)
		})

		By("Deploying namespace-only app as namespace-admin on source cluster")
		err = PrepareSourceApp(srcAppNonAdmin, kubectlSrcNonAdmin)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for source pods and endpoints to drain")
		WaitForSourceQuiesce(kubectlSrcNonAdmin, namespace, "app="+appName, serviceName)

		By("Running crane export, transform, apply as namespace-admin")
		Expect(RunPipeline(&runner, namespace, paths)).NotTo(HaveOccurred())

		By("Verifying no cluster resources in output _cluster directory")
		Expect(AssertNoClusterResources(filepath.Join(paths.OutputDir, "resources", "_cluster"))).NotTo(HaveOccurred())

		By("Applying namespace resources to target as namespace-admin")
		Expect(NonAdminApplyOutput(kubectlTgtNonAdmin, paths.OutputDir, namespace)).NotTo(HaveOccurred())

		By("Scaling target deployment and validating app")
		ScaleAndValidateTargetApp(kubectlTgtNonAdmin, tgtAppNonAdmin, namespace, appName)

	})

})
