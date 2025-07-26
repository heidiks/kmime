package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/term"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

func getKubeConfig() (*kubernetes.Clientset, *rest.Config, error) {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get user home directory: %w", err)
	}
	kubeconfigPath := filepath.Join(userHomeDir, ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return clientset, config, nil
}

func getPod(clientset *kubernetes.Clientset, namespace, podName string) (*v1.Pod, error) {
	pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod '%s' in namespace '%s': %w", podName, namespace, err)
	}
	return pod, nil
}

func generateNewPodName(originalName, prefix, suffix, user string) string {
	var nameParts []string
	if prefix != "" {
		nameParts = append(nameParts, prefix)
	}
	nameParts = append(nameParts, originalName)
	if suffix != "" {
		nameParts = append(nameParts, suffix)
	}
	if user != "" {
		nameParts = append(nameParts, user)
	}
	nameParts = append(nameParts, fmt.Sprintf("%d", time.Now().UnixNano()%10000))

	fullName := strings.Join(nameParts, "-")
	if len(fullName) > 63 {
		fullName = fullName[:63]
	}
	return strings.Trim(fullName, "-")
}

func clonePod(originalPod *v1.Pod, user string, command []string, prefix, suffix string, newLabels map[string]string, newEnvs []v1.EnvVar) *v1.Pod {
	podName := generateNewPodName(originalPod.Name, prefix, suffix, user)

	finalLabels := make(map[string]string)
	for k, v := range originalPod.Labels {
		finalLabels[k] = v
	}
	for k, v := range newLabels {
		finalLabels[k] = v
	}

	newPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Namespace:   originalPod.Namespace,
			Labels:      finalLabels,
			Annotations: originalPod.Annotations,
		},
		Spec: *originalPod.Spec.DeepCopy(),
	}
	newPod.Spec.RestartPolicy = v1.RestartPolicyNever
	if len(newPod.Spec.Containers) > 0 {
		newPod.Spec.Containers[0].Command = command
		newPod.Spec.Containers[0].Args = nil
		newPod.Spec.Containers[0].TTY = true
		newPod.Spec.Containers[0].Stdin = true

		envMap := make(map[string]v1.EnvVar)
		for _, env := range newPod.Spec.Containers[0].Env {
			envMap[env.Name] = env
		}
		for _, env := range newEnvs {
			envMap[env.Name] = env
		}

		var finalEnvs []v1.EnvVar
		for _, env := range envMap {
			finalEnvs = append(finalEnvs, env)
		}
		newPod.Spec.Containers[0].Env = finalEnvs
	}
	newPod.Spec.NodeName = ""
	newPod.Spec.ServiceAccountName = originalPod.Spec.ServiceAccountName
	return newPod
}

func createPod(clientset *kubernetes.Clientset, pod *v1.Pod) (*v1.Pod, error) {
	createdPod, err := clientset.CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create pod '%s': %w", pod.Name, err)
	}
	return createdPod, nil
}

func deletePod(clientset *kubernetes.Clientset, namespace, podName string) error {
	err := clientset.CoreV1().Pods(namespace).Delete(context.TODO(), podName, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete pod '%s': %w", podName, err)
	}
	return nil
}

func waitForPodRunning(clientset *kubernetes.Clientset, namespace, podName string, timeout time.Duration) error {
	watcher, err := clientset.CoreV1().Pods(namespace).Watch(context.TODO(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", podName),
	})
	if err != nil {
		return fmt.Errorf("could not watch pod %s: %w", podName, err)
	}
	defer watcher.Stop()

	for {
		select {
		case event := <-watcher.ResultChan():
			if event.Type == watch.Error {
				return fmt.Errorf("watch error: %v", event.Object)
			}
			pod, ok := event.Object.(*v1.Pod)
			if !ok {
				return fmt.Errorf("unexpected object type in watch: %T", event.Object)
			}
			switch pod.Status.Phase {
			case v1.PodRunning, v1.PodSucceeded:
				return nil
			case v1.PodFailed:
				return fmt.Errorf("pod terminated unexpectedly with phase %s", pod.Status.Phase)
			}
		case <-time.After(timeout):
			return fmt.Errorf("timeout waiting for pod %s to be running", podName)
		}
	}
}

type terminalSizeQueue struct {
	resizeChan chan remotecommand.TerminalSize
}

func (t *terminalSizeQueue) Next() *remotecommand.TerminalSize {
	size, ok := <-t.resizeChan
	if !ok {
		return nil
	}
	return &size
}

func attachToPod(clientset *kubernetes.Clientset, config *rest.Config, namespace, podName string, command []string) error {
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("attach")
	req.VersionedParams(&v1.PodAttachOptions{
		Container: "",
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, http.MethodPost, req.URL())
	if err != nil {
		return fmt.Errorf("failed to create SPDY executor: %w", err)
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to set terminal to raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	resizeChan := make(chan remotecommand.TerminalSize)
	sizeQueue := &terminalSizeQueue{resizeChan: resizeChan}

	go func() {
		defer close(resizeChan)
		width, height, _ := term.GetSize(int(os.Stdout.Fd()))
		resizeChan <- remotecommand.TerminalSize{Width: uint16(width), Height: uint16(height)}
		for {
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()
			select {
			case <-ticker.C:
				newWidth, newHeight, _ := term.GetSize(int(os.Stdout.Fd()))
				if newWidth != width || newHeight != height {
					width = newWidth
					height = newHeight
					resizeChan <- remotecommand.TerminalSize{Width: uint16(width), Height: uint16(height)}
				}
			}
		}
	}()

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:             os.Stdin,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		Tty:               true,
		TerminalSizeQueue: sizeQueue,
	})

	return err
}

func getUserIdentifier() (string, error) {
	var identifier string
	cmd := exec.Command("git", "config", "--global", "--get", "user.email")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		identifier = string(output)
	} else {
		hostname, err := os.Hostname()
		if err != nil {
			return "", fmt.Errorf("failed to get hostname: %w", err)
		}
		identifier = hostname
	}

	sanitized := strings.TrimSpace(identifier)
	sanitized = strings.ReplaceAll(sanitized, "@", "-")
	sanitized = strings.ReplaceAll(sanitized, ".", "-")
	reg := regexp.MustCompile("[^a-zA-Z0-9-]+")
	sanitized = reg.ReplaceAllString(sanitized, "")
	sanitized = strings.Trim(sanitized, "-")

	return strings.ToLower(sanitized), nil
}
