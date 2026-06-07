package e2e

import (
	"github.com/konveyor/crane/e2e-tests/config"
	. "github.com/konveyor/crane/e2e-tests/framework"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cluster-level export control", func() {
	It("[CA-5] Should produce no _cluster output for namespace-only workload", Label("tier0"), func() {
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
		srcApp := scenario.SrcApp
		tgtApp := scenario.TgtApp
		kubectlSrc := scenario.KubectlSrc
		kubectlTgt := scenario.KubectlTgt
		paths, err := NewScenarioPaths("crane-ca5-*")
		Expect(err).NotTo(HaveOccurred())
		runner := scenario.Crane

		DeferCleanup(func() {
			ScenarioCleanup(paths, srcApp, tgtApp, kubectlSrc, kubectlTgt, namespace)
		})

		By("Deploying namespace-only app on source cluster")
		Expect(PrepareSourceApp(srcApp, kubectlSrc)).NotTo(HaveOccurred())

		By("Waiting for source pods and endpoints to drain")
		WaitForSourceQuiesce(kubectlSrc, namespace, "app="+appName, serviceName)

		By("Running crane export, transform, apply")
		Expect(RunPipeline(&runner, namespace, paths)).NotTo(HaveOccurred())

		By("Verifying no cluster-scoped resources in export directory")
		Expect(AssertNoClusterResources(paths.ExportDir)).NotTo(HaveOccurred())

		By("Verifying no cluster-scoped resources in output directory")
		Expect(AssertNoClusterResources(paths.OutputDir)).NotTo(HaveOccurred())

		By("Applying migrated manifests to target cluster")
		Expect(ApplyOutputToTarget(kubectlTgt, namespace, paths.OutputDir)).NotTo(HaveOccurred())

		By("Scaling target deployment and validating app")
		ScaleAndValidateTargetApp(kubectlTgt, tgtApp, namespace, appName)
	})

})
