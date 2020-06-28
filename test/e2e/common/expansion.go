/*
Copyright 2016 The Kubernetes Authors.

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

package common

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	imageutils "k8s.io/kubernetes/test/utils/image"

	"github.com/onsi/ginkgo"
)

// These tests exercise the Kubernetes expansion syntax $(VAR).
// For more information, see:
// https://github.com/kubernetes/community/blob/master/contributors/design-proposals/node/expansion.md
var _ = framework.KubeDescribe("Variable Expansion", func() {
	f := framework.NewDefaultFramework("var-expansion")

	/*
		Release : v1.9
		Testname: Environment variables, expansion
		Description: Create a Pod with environment variables. Environment variables defined using previously defined environment variables MUST expand to proper values.
	*/
	framework.ConformanceIt("should allow composing env vars into new env vars [NodeConformance]", func() {
		podName := "var-expansion-" + string(uuid.NewUUID())
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   podName,
				Labels: map[string]string{"name": podName},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:    "dapi-container",
						Image:   imageutils.GetE2EImage(imageutils.BusyBox),
						Command: []string{"sh", "-c", "env"},
						Env: []v1.EnvVar{
							{
								Name:  "FOO",
								Value: "foo-value",
							},
							{
								Name:  "BAR",
								Value: "bar-value",
							},
							{
								Name:  "FOOBAR",
								Value: "$(FOO);;$(BAR)",
							},
						},
					},
				},
				RestartPolicy: v1.RestartPolicyNever,
			},
		}

		f.TestContainerOutput("env composition", pod, 0, []string{
			"FOO=foo-value",
			"BAR=bar-value",
			"FOOBAR=foo-value;;bar-value",
		})
	})

	/*
		Release : v1.9
		Testname: Environment variables, command expansion
		Description: Create a Pod with environment variables and container command using them. Container command using the  defined environment variables MUST expand to proper values.
	*/
	framework.ConformanceIt("should allow substituting values in a container's command [NodeConformance]", func() {
		podName := "var-expansion-" + string(uuid.NewUUID())
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   podName,
				Labels: map[string]string{"name": podName},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:    "dapi-container",
						Image:   imageutils.GetE2EImage(imageutils.BusyBox),
						Command: []string{"sh", "-c", "TEST_VAR=wrong echo \"$(TEST_VAR)\""},
						Env: []v1.EnvVar{
							{
								Name:  "TEST_VAR",
								Value: "test-value",
							},
						},
					},
				},
				RestartPolicy: v1.RestartPolicyNever,
			},
		}

		f.TestContainerOutput("substitution in container's command", pod, 0, []string{
			"test-value",
		})
	})

	/*
		Release : v1.9
		Testname: Environment variables, command argument expansion
		Description: Create a Pod with environment variables and container command arguments using them. Container command arguments using the  defined environment variables MUST expand to proper values.
	*/
	framework.ConformanceIt("should allow substituting values in a container's args [NodeConformance]", func() {
		podName := "var-expansion-" + string(uuid.NewUUID())
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   podName,
				Labels: map[string]string{"name": podName},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:    "dapi-container",
						Image:   imageutils.GetE2EImage(imageutils.BusyBox),
						Command: []string{"sh", "-c"},
						Args:    []string{"TEST_VAR=wrong echo \"$(TEST_VAR)\""},
						Env: []v1.EnvVar{
							{
								Name:  "TEST_VAR",
								Value: "test-value",
							},
						},
					},
				},
				RestartPolicy: v1.RestartPolicyNever,
			},
		}

		f.TestContainerOutput("substitution in container's args", pod, 0, []string{
			"test-value",
		})
	})

	/*
		    Testname: var-expansion-subpath
		    Description: Make sure a container's subpath can be set using an
			expansion of environment variables.
	*/
	ginkgo.It("should allow substituting values in a volume subpath [sig-storage]", func() {
		podName := "var-expansion-" + string(uuid.NewUUID())
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   podName,
				Labels: map[string]string{"name": podName},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:    "dapi-container",
						Image:   imageutils.GetE2EImage(imageutils.BusyBox),
						Command: []string{"sh", "-c", "test -d /testcontainer/" + podName + ";echo $?"},
						Env: []v1.EnvVar{
							{
								Name:  "POD_NAME",
								Value: podName,
							},
						},
						VolumeMounts: []v1.VolumeMount{
							{
								Name:        "workdir1",
								MountPath:   "/logscontainer",
								SubPathExpr: "$(POD_NAME)",
							},
							{
								Name:      "workdir2",
								MountPath: "/testcontainer",
							},
						},
					},
				},
				RestartPolicy: v1.RestartPolicyNever,
				Volumes: []v1.Volume{
					{
						Name: "workdir1",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{Path: "/tmp"},
						},
					},
					{
						Name: "workdir2",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{Path: "/tmp"},
						},
					},
				},
			},
		}

		f.TestContainerOutput("substitution in volume subpath", pod, 0, []string{
			"0",
		})
	})

	/*
		    Testname: var-expansion-subpath-with-backticks
		    Description: Make sure a container's subpath can not be set using an
			expansion of environment variables when backticks are supplied.
	*/
	ginkgo.It("should fail substituting values in a volume subpath with backticks [sig-storage][Slow]", func() {

		podName := "var-expansion-" + string(uuid.NewUUID())
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   podName,
				Labels: map[string]string{"name": podName},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "dapi-container",
						Image: imageutils.GetE2EImage(imageutils.BusyBox),
						Env: []v1.EnvVar{
							{
								Name:  "POD_NAME",
								Value: "..",
							},
						},
						VolumeMounts: []v1.VolumeMount{
							{
								Name:        "workdir1",
								MountPath:   "/logscontainer",
								SubPathExpr: "$(POD_NAME)",
							},
						},
					},
				},
				RestartPolicy: v1.RestartPolicyNever,
				Volumes: []v1.Volume{
					{
						Name: "workdir1",
						VolumeSource: v1.VolumeSource{
							EmptyDir: &v1.EmptyDirVolumeSource{},
						},
					},
				},
			},
		}

		// Pod should fail
		testPodFailSubpath(f, pod)
	})

	/*
		    Testname: var-expansion-subpath-with-absolute-path
		    Description: Make sure a container's subpath can not be set using an
			expansion of environment variables when absolute path is supplied.
	*/
	ginkgo.It("should fail substituting values in a volume subpath with absolute path [sig-storage][Slow]", func() {

		podName := "var-expansion-" + string(uuid.NewUUID())
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   podName,
				Labels: map[string]string{"name": podName},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "dapi-container",
						Image: imageutils.GetE2EImage(imageutils.BusyBox),
						Env: []v1.EnvVar{
							{
								Name:  "POD_NAME",
								Value: "/tmp",
							},
						},
						VolumeMounts: []v1.VolumeMount{
							{
								Name:        "workdir1",
								MountPath:   "/logscontainer",
								SubPathExpr: "$(POD_NAME)",
							},
						},
					},
				},
				RestartPolicy: v1.RestartPolicyNever,
				Volumes: []v1.Volume{
					{
						Name: "workdir1",
						VolumeSource: v1.VolumeSource{
							EmptyDir: &v1.EmptyDirVolumeSource{},
						},
					},
				},
			},
		}

		// Pod should fail
		testPodFailSubpath(f, pod)
	})

	/*
	   Testname: var-expansion-subpath-ready-from-failed-state
	   Description: Verify that a failing subpath expansion can be modified during the lifecycle of a container.
	*/
	ginkgo.It("should verify that a failing subpath expansion can be modified during the lifecycle of a container [sig-storage][Slow]", func() {

		podName := "var-expansion-" + string(uuid.NewUUID())
		containerName := "dapi-container"
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        podName,
				Labels:      map[string]string{"name": podName},
				Annotations: map[string]string{"notmysubpath": "mypath"},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:    containerName,
						Image:   imageutils.GetE2EImage(imageutils.BusyBox),
						Command: []string{"sh", "-c", "tail -f /dev/null"},
						Env: []v1.EnvVar{
							{
								Name:  "POD_NAME",
								Value: "foo",
							},
							{
								Name: "ANNOTATION",
								ValueFrom: &v1.EnvVarSource{
									FieldRef: &v1.ObjectFieldSelector{
										APIVersion: "v1",
										FieldPath:  "metadata.annotations['mysubpath']",
									},
								},
							},
						},
						VolumeMounts: []v1.VolumeMount{
							{
								Name:        "workdir1",
								MountPath:   "/subpath_mount",
								SubPathExpr: "$(ANNOTATION)/$(POD_NAME)",
							},
							{
								Name:      "workdir2",
								MountPath: "/volume_mount",
							},
						},
					},
				},
				Volumes: []v1.Volume{
					{
						Name: "workdir1",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{Path: "/tmp"},
						},
					},
					{
						Name: "workdir2",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{Path: "/tmp"},
						},
					},
				},
			},
		}

		ginkgo.By("creating the pod with failed condition")
		var podClient *framework.PodClient = f.PodClient()
		pod = podClient.Create(pod)

		err := e2epod.WaitTimeoutForPodRunningInNamespace(f.ClientSet, pod.Name, pod.Namespace, framework.PodStartShortTimeout)
		framework.ExpectError(err, "while waiting for pod to be running")

		ginkgo.By("updating the pod")
		podClient.Update(podName, func(pod *v1.Pod) {
			pod.ObjectMeta.Annotations = map[string]string{"mysubpath": "mypath"}
		})

		ginkgo.By("waiting for pod running")
		err = e2epod.WaitTimeoutForPodRunningInNamespace(f.ClientSet, pod.Name, pod.Namespace, framework.PodStartShortTimeout)
		framework.ExpectNoError(err, "while waiting for pod to be running")

		ginkgo.By("deleting the pod gracefully")
		err = e2epod.DeletePodWithWait(f.ClientSet, pod)
		framework.ExpectNoError(err, "failed to delete pod")
	})

	/*
		    Testname: var-expansion-subpath-test-writes
		    Description: Verify that a subpath expansion can be used to write files into subpaths.
			1.	valid subpathexpr starts a container running
			2.	test for valid subpath writes
			3.	successful expansion of the subpathexpr isn't required for volume cleanup

	*/
	ginkgo.It("should succeed in writing subpaths in container [sig-storage][Slow]", func() {

		podName := "var-expansion-" + string(uuid.NewUUID())
		containerName := "dapi-container"
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        podName,
				Labels:      map[string]string{"name": podName},
				Annotations: map[string]string{"mysubpath": "mypath"},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:    containerName,
						Image:   imageutils.GetE2EImage(imageutils.BusyBox),
						Command: []string{"sh", "-c", "tail -f /dev/null"},
						Env: []v1.EnvVar{
							{
								Name:  "POD_NAME",
								Value: "foo",
							},
							{
								Name: "ANNOTATION",
								ValueFrom: &v1.EnvVarSource{
									FieldRef: &v1.ObjectFieldSelector{
										APIVersion: "v1",
										FieldPath:  "metadata.annotations['mysubpath']",
									},
								},
							},
						},
						VolumeMounts: []v1.VolumeMount{
							{
								Name:        "workdir1",
								MountPath:   "/subpath_mount",
								SubPathExpr: "$(ANNOTATION)/$(POD_NAME)",
							},
							{
								Name:      "workdir2",
								MountPath: "/volume_mount",
							},
						},
					},
				},
				RestartPolicy: v1.RestartPolicyNever,
				Volumes: []v1.Volume{
					{
						Name: "workdir1",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{Path: "/tmp"},
						},
					},
					{
						Name: "workdir2",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{Path: "/tmp"},
						},
					},
				},
			},
		}

		ginkgo.By("creating the pod")
		var podClient *framework.PodClient = f.PodClient()
		pod = podClient.Create(pod)

		ginkgo.By("waiting for pod running")
		err := e2epod.WaitTimeoutForPodRunningInNamespace(f.ClientSet, pod.Name, pod.Namespace, framework.PodStartShortTimeout)
		framework.ExpectNoError(err, "while waiting for pod to be running")

		ginkgo.By("creating a file in subpath")
		cmd := "touch /volume_mount/mypath/foo/test.log"
		_, _, err = f.ExecShellInPodWithFullOutput(pod.Name, cmd)
		if err != nil {
			framework.Failf("expected to be able to write to subpath")
		}

		ginkgo.By("test for file in mounted path")
		cmd = "test -f /subpath_mount/test.log"
		_, _, err = f.ExecShellInPodWithFullOutput(pod.Name, cmd)
		if err != nil {
			framework.Failf("expected to be able to verify file")
		}

		ginkgo.By("updating the annotation value")
		podClient.Update(podName, func(pod *v1.Pod) {
			pod.ObjectMeta.Annotations["mysubpath"] = "mynewpath"
		})

		ginkgo.By("waiting for annotated pod running")
		err = e2epod.WaitTimeoutForPodRunningInNamespace(f.ClientSet, pod.Name, pod.Namespace, framework.PodStartShortTimeout)
		framework.ExpectNoError(err, "while waiting for annotated pod to be running")

		ginkgo.By("deleting the pod gracefully")
		err = e2epod.DeletePodWithWait(f.ClientSet, pod)
		framework.ExpectNoError(err, "failed to delete pod")
	})
})

func testPodFailSubpath(f *framework.Framework, pod *v1.Pod) {
	var podClient *framework.PodClient = f.PodClient()
	pod = podClient.Create(pod)

	defer func() {
		e2epod.DeletePodWithWait(f.ClientSet, pod)
	}()

	err := e2epod.WaitTimeoutForPodRunningInNamespace(f.ClientSet, pod.Name, pod.Namespace, framework.PodStartShortTimeout)
	framework.ExpectError(err, "while waiting for pod to be running")
}
