/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package node

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/kubernetes/test/e2e/framework"
	e2ekubelet "k8s.io/kubernetes/test/e2e/framework/kubelet"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2eskipper "k8s.io/kubernetes/test/e2e/framework/skipper"
	"k8s.io/kubernetes/test/e2e/storage/utils"
	testutils "k8s.io/kubernetes/test/utils"
	admissionapi "k8s.io/pod-security-admission/api"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

// This test checks if node-problem-detector (NPD) runs fine without error on
// up to 10 nodes in the cluster. NPD's functionality is tested in e2e_node tests.
var _ = SIGDescribe("NodeProblemDetector", func() {
	const (
		pollInterval = 1 * time.Second
		pollTimeout  = 1 * time.Minute
	)
	f := framework.NewDefaultFramework("node-problem-detector")
	f.NamespacePodSecurityLevel = admissionapi.LevelPrivileged

	ginkgo.BeforeEach(func(ctx context.Context) {
		e2eskipper.SkipUnlessProviderIs("gce", "gke") // TODO(upodroid) remove this SKIP
		e2enode.WaitForTotalHealthy(ctx, f.ClientSet, time.Minute)
	})

	ginkgo.It("should run without error", func(ctx context.Context) {
		ginkgo.By("Getting all nodes and their SSH-able IP addresses")
		readyNodes, err := e2enode.GetBoundedReadySchedulableNodes(ctx, f.ClientSet, 10)
		framework.ExpectNoError(err)
		nodes := readyNodes.Items
		hostExec := utils.NewHostExec(f)
		ginkgo.DeferCleanup(hostExec.Cleanup)

		isStandaloneMode := make(map[string]bool)
		cpuUsageStats := make(map[string][]float64)
		uptimeStats := make(map[string][]float64)
		rssStats := make(map[string][]float64)
		workingSetStats := make(map[string][]float64)

		// Some tests suites running for days.
		// This test is not marked as Disruptive or Serial so we do not want to
		// restart the kubelet during the test to check for KubeletStart event
		// detection. We use heuristic here to check if we need to validate for the
		// KubeletStart event since there is no easy way to check when test has actually started.
		checkForKubeletStart := false

		for _, node := range nodes {
			cpuUsageStats[node.Name] = []float64{}
			uptimeStats[node.Name] = []float64{}
			rssStats[node.Name] = []float64{}
			workingSetStats[node.Name] = []float64{}

			cmd := "systemctl status node-problem-detector.service"
			result, err := hostExec.Execute(ctx, cmd, &node)
			framework.Logf("output from cmd on node %s, value:%+v", node.Name, result)

			isStandaloneMode[node.Name] = (err == nil && result.Code == 0)

			if isStandaloneMode[node.Name] {
				ginkgo.By(fmt.Sprintf("Check node %q has node-problem-detector process", node.Name))
				// Using brackets "[n]" is a trick to prevent grep command itself from
				// showing up, because string text "[n]ode-problem-detector" does not
				// match regular expression "[n]ode-problem-detector".
				psCmd := "ps aux | grep [n]ode-problem-detector"
				result, err := hostExec.Execute(ctx, psCmd, &node)
				framework.ExpectNoError(err)
				gomega.Expect(result.Code).To(gomega.Equal(0))
				gomega.Expect(result.Stdout).To(gomega.ContainSubstring("node-problem-detector"))

				ginkgo.By(fmt.Sprintf("Check node-problem-detector is running fine on node %q", node.Name))
				journalctlCmd := "sudo journalctl -r -u node-problem-detector"
				result, err = hostExec.Execute(ctx, journalctlCmd, &node)
				framework.ExpectNoError(err)
				gomega.Expect(result.Code).To(gomega.Equal(0))
				gomega.Expect(result.Stdout).NotTo(gomega.ContainSubstring("node-problem-detector.service: Failed"))

				// We only will check for the KubeletStart even if parsing of date here succeeded.
				ginkgo.By(fmt.Sprintf("Check when node-problem-detector started on node %q", node.Name))
				npdStartTimeCommand := "sudo systemctl show --timestamp=utc node-problem-detector -P ActiveEnterTimestamp"
				result, err = hostExec.Execute(ctx, npdStartTimeCommand, &node)
				framework.ExpectNoError(err)
				gomega.Expect(result.Code).To(gomega.Equal(0))

				// The time format matches the systemd format.
				// 'utc': 'Day YYYY-MM-DD HH:MM:SS UTC (see https://www.freedesktop.org/software/systemd/man/systemd.time.html)
				st, err := time.Parse("Mon 2006-01-02 15:04:05 MST", result.Stdout)
				if err != nil {
					framework.Logf("Failed to parse when NPD started. Got exit code: %v and stdout: %v, error: %v. Will skip check for kubelet start event.", result.Code, result.Stdout, err)
				} else {
					checkForKubeletStart = time.Since(st) < time.Hour
				}

				cpuUsage, uptime := getCPUStat(ctx, f, hostExec, node)
				cpuUsageStats[node.Name] = append(cpuUsageStats[node.Name], cpuUsage)
				uptimeStats[node.Name] = append(uptimeStats[node.Name], uptime)

			}
			ginkgo.By(fmt.Sprintf("Inject log to trigger DockerHung on node %q", node.Name))
			log := "INFO: task docker:12345 blocked for more than 120 seconds."
			injectLogCmd := "sudo sh -c \"echo 'kernel: " + log + "' >> /dev/kmsg\""
			result, err = hostExec.Execute(ctx, injectLogCmd, &node)
			framework.ExpectNoError(err)
			gomega.Expect(result.Code).To(gomega.Equal(0))
		}

		ginkgo.By("Check node-problem-detector can post conditions and events to API server")
		for _, node := range nodes {
			ginkgo.By(fmt.Sprintf("Check node-problem-detector posted KernelDeadlock condition on node %q", node.Name))
			gomega.Eventually(ctx, func() error {
				return verifyNodeCondition(ctx, f, "KernelDeadlock", v1.ConditionTrue, "DockerHung", node.Name)
			}, pollTimeout, pollInterval).Should(gomega.Succeed())

			ginkgo.By(fmt.Sprintf("Check node-problem-detector posted DockerHung event on node %q", node.Name))
			eventListOptions := metav1.ListOptions{FieldSelector: fields.Set{"involvedObject.kind": "Node"}.AsSelector().String()}
			gomega.Eventually(ctx, func(ctx context.Context) error {
				return verifyEvents(ctx, f, eventListOptions, 1, "DockerHung", node.Name)
			}, pollTimeout, pollInterval).Should(gomega.Succeed())

			if checkForKubeletStart {
				// Node problem detector reports kubelet start events automatically starting from NPD v0.7.0+.
				// Since Kubelet may be restarted for a few times after node is booted. We just check the event
				// is detected, but do not check how many times Kubelet is started.
				//
				// Some test suites run for hours and KubeletStart event will already be cleaned up
				ginkgo.By(fmt.Sprintf("Check node-problem-detector posted KubeletStart event on node %q", node.Name))
				gomega.Eventually(ctx, func(ctx context.Context) error {
					return verifyEventExists(ctx, f, eventListOptions, "KubeletStart", node.Name)
				}, pollTimeout, pollInterval).Should(gomega.Succeed())
			} else {
				ginkgo.By("KubeletStart event will NOT be checked")
			}
		}

		ginkgo.By("Gather node-problem-detector cpu and memory stats")
		numIterations := 60
		for i := 1; i <= numIterations; i++ {
			for _, node := range nodes {
				if isStandaloneMode[node.Name] {
					rss, workingSet := getMemoryStat(ctx, f, hostExec, node)
					rssStats[node.Name] = append(rssStats[node.Name], rss)
					workingSetStats[node.Name] = append(workingSetStats[node.Name], workingSet)
					if i == numIterations {
						cpuUsage, uptime := getCPUStat(ctx, f, hostExec, node)
						cpuUsageStats[node.Name] = append(cpuUsageStats[node.Name], cpuUsage)
						uptimeStats[node.Name] = append(uptimeStats[node.Name], uptime)
					}
				} else {
					cpuUsage, rss, workingSet := getNpdPodStat(ctx, f, node.Name)
					cpuUsageStats[node.Name] = append(cpuUsageStats[node.Name], cpuUsage)
					rssStats[node.Name] = append(rssStats[node.Name], rss)
					workingSetStats[node.Name] = append(workingSetStats[node.Name], workingSet)
				}
			}
			time.Sleep(time.Second)
		}

		cpuStatsMsg := "CPU (core):"
		rssStatsMsg := "RSS (MB):"
		workingSetStatsMsg := "WorkingSet (MB):"
		for i, node := range nodes {
			if isStandaloneMode[node.Name] {
				// When in standalone mode, NPD is running as systemd service. We
				// calculate its cpu usage from cgroup cpuacct value differences.
				cpuUsage := cpuUsageStats[node.Name][1] - cpuUsageStats[node.Name][0]
				totaltime := uptimeStats[node.Name][1] - uptimeStats[node.Name][0]
				cpuStatsMsg += fmt.Sprintf(" %s[%.3f];", nodes[i].Name, cpuUsage/totaltime)
			} else {
				sort.Float64s(cpuUsageStats[node.Name])
				cpuStatsMsg += fmt.Sprintf(" %s[%.3f|%.3f|%.3f];", nodes[i].Name,
					cpuUsageStats[node.Name][0], cpuUsageStats[node.Name][len(cpuUsageStats[node.Name])/2], cpuUsageStats[node.Name][len(cpuUsageStats[node.Name])-1])
			}

			sort.Float64s(rssStats[node.Name])
			rssStatsMsg += fmt.Sprintf(" %s[%.1f|%.1f|%.1f];", nodes[i].Name,
				rssStats[node.Name][0], rssStats[node.Name][len(rssStats[node.Name])/2], rssStats[node.Name][len(rssStats[node.Name])-1])

			sort.Float64s(workingSetStats[node.Name])
			workingSetStatsMsg += fmt.Sprintf(" %s[%.1f|%.1f|%.1f];", nodes[i].Name,
				workingSetStats[node.Name][0], workingSetStats[node.Name][len(workingSetStats[node.Name])/2], workingSetStats[node.Name][len(workingSetStats[node.Name])-1])
		}
		framework.Logf("Node-Problem-Detector CPU and Memory Stats:\n\t%s\n\t%s\n\t%s", cpuStatsMsg, rssStatsMsg, workingSetStatsMsg)
	})
})

func verifyEvents(ctx context.Context, f *framework.Framework, options metav1.ListOptions, num int, reason, nodeName string) error {
	events, err := f.ClientSet.CoreV1().Events(metav1.NamespaceDefault).List(ctx, options)
	if err != nil {
		return err
	}
	count := 0
	for _, event := range events.Items {
		if event.Reason != reason || event.Source.Host != nodeName {
			continue
		}
		count += int(event.Count)
	}
	if count != num {
		return fmt.Errorf("expect event number %d, got %d: %v", num, count, events.Items)
	}
	return nil
}

func verifyEventExists(ctx context.Context, f *framework.Framework, options metav1.ListOptions, reason, nodeName string) error {
	events, err := f.ClientSet.CoreV1().Events(metav1.NamespaceDefault).List(ctx, options)
	if err != nil {
		return err
	}
	for _, event := range events.Items {
		if event.Reason == reason && event.Source.Host == nodeName && event.Count > 0 {
			return nil
		}
	}
	return fmt.Errorf("Event %s does not exist: %v", reason, events.Items)
}

func verifyNodeCondition(ctx context.Context, f *framework.Framework, condition v1.NodeConditionType, status v1.ConditionStatus, reason, nodeName string) error {
	node, err := f.ClientSet.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	_, c := testutils.GetNodeCondition(&node.Status, condition)
	if c == nil {
		return fmt.Errorf("node condition %q not found", condition)
	}
	if c.Status != status || c.Reason != reason {
		return fmt.Errorf("unexpected node condition %q: %+v", condition, c)
	}
	return nil
}

func getMemoryStat(ctx context.Context, f *framework.Framework, hostExec utils.HostExec, node v1.Node) (rss, workingSet float64) {
	var memCmd string

	isCgroupV2 := isHostRunningCgroupV2(ctx, f, hostExec, node)
	if isCgroupV2 {
		memCmd = "cat /sys/fs/cgroup/system.slice/node-problem-detector.service/memory.current && cat /sys/fs/cgroup/system.slice/node-problem-detector.service/memory.stat"
	} else {
		memCmd = "cat /sys/fs/cgroup/memory/system.slice/node-problem-detector.service/memory.usage_in_bytes && cat /sys/fs/cgroup/memory/system.slice/node-problem-detector.service/memory.stat"
	}

	result, err := hostExec.Execute(ctx, memCmd, &node)
	framework.ExpectNoError(err)
	gomega.Expect(result.Code).To(gomega.Equal(0))
	lines := strings.Split(result.Stdout, "\n")

	memoryUsage, err := strconv.ParseFloat(lines[0], 64)
	framework.ExpectNoError(err)

	var rssToken, inactiveFileToken string
	if isCgroupV2 {
		// Use Anon memory for RSS as cAdvisor on cgroupv2
		// see https://github.com/google/cadvisor/blob/a9858972e75642c2b1914c8d5428e33e6392c08a/container/libcontainer/handler.go#L799
		rssToken = "anon"
		inactiveFileToken = "inactive_file"
	} else {
		rssToken = "total_rss"
		inactiveFileToken = "total_inactive_file"
	}

	var totalInactiveFile float64
	for _, line := range lines[1:] {
		tokens := strings.Split(line, " ")

		if tokens[0] == rssToken {
			rss, err = strconv.ParseFloat(tokens[1], 64)
			framework.ExpectNoError(err)
		}
		if tokens[0] == inactiveFileToken {
			totalInactiveFile, err = strconv.ParseFloat(tokens[1], 64)
			framework.ExpectNoError(err)
		}
	}

	workingSet = memoryUsage
	if workingSet < totalInactiveFile {
		workingSet = 0
	} else {
		workingSet -= totalInactiveFile
	}

	// Convert to MB
	rss = rss / 1024 / 1024
	workingSet = workingSet / 1024 / 1024
	return
}

func getCPUStat(ctx context.Context, f *framework.Framework, hostExec utils.HostExec, node v1.Node) (usage, uptime float64) {
	var cpuCmd string
	if isHostRunningCgroupV2(ctx, f, hostExec, node) {
		cpuCmd = " cat /sys/fs/cgroup/cpu.stat | grep 'usage_usec' | sed 's/[^0-9]*//g' && cat /proc/uptime | awk '{print $1}'"
	} else {
		cpuCmd = "cat /sys/fs/cgroup/cpu/system.slice/node-problem-detector.service/cpuacct.usage && cat /proc/uptime | awk '{print $1}'"
	}

	result, err := hostExec.Execute(ctx, cpuCmd, &node)
	framework.ExpectNoError(err)
	gomega.Expect(result.Code).To(gomega.Equal(0))
	lines := strings.Split(result.Stdout, "\n")

	usage, err = strconv.ParseFloat(lines[0], 64)
	framework.ExpectNoError(err, "Cannot parse float for usage")
	uptime, err = strconv.ParseFloat(lines[1], 64)
	framework.ExpectNoError(err, "Cannot parse float for uptime")

	// Convert from nanoseconds to seconds
	usage *= 1e-9
	return
}

func isHostRunningCgroupV2(ctx context.Context, f *framework.Framework, hostExec utils.HostExec, node v1.Node) bool {
	result, err := hostExec.Execute(ctx, "stat -fc %T /sys/fs/cgroup/", &node)
	framework.ExpectNoError(err)
	gomega.Expect(result.Code).To(gomega.Equal(0))

	// 0x63677270 == CGROUP2_SUPER_MAGIC
	// https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html
	return strings.Contains(result.Stdout, "cgroup2") || strings.Contains(result.Stdout, "0x63677270")
}

func getNpdPodStat(ctx context.Context, f *framework.Framework, nodeName string) (cpuUsage, rss, workingSet float64) {
	summary, err := e2ekubelet.GetStatsSummary(ctx, f.ClientSet, nodeName)
	framework.ExpectNoError(err)

	hasNpdPod := false
	for _, pod := range summary.Pods {
		if !strings.HasPrefix(pod.PodRef.Name, "node-problem-detector") {
			continue
		}
		cpuUsage = float64(*pod.CPU.UsageNanoCores) * 1e-9
		rss = float64(*pod.Memory.RSSBytes) / 1024 / 1024
		workingSet = float64(*pod.Memory.WorkingSetBytes) / 1024 / 1024
		hasNpdPod = true
		break
	}
	if !hasNpdPod {
		framework.Failf("No node-problem-detector pod is present in %+v", summary.Pods)
	}
	return
}
