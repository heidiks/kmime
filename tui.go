package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("69"))
	statusStyle  = lipgloss.NewStyle().MarginLeft(1)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)

type (
	errorMsg struct{ err error }

	kubeConnectedMsg struct {
		clientset *kubernetes.Clientset
		config    *rest.Config
	}
	podFetchedMsg   struct{}
	podCreatedMsg   struct{ podName string }
	podRunningMsg   struct{ podName string }
	attachMsg       struct{}
	podAttachedMsg  struct{}
	podCleanedUpMsg struct{ podName string }
	finalSuccessMsg struct{ message string }
)

type model struct {
	params *kmimeParams

	spinner    spinner.Model
	statusText string
	done       bool
	err        error

	clientset  *kubernetes.Clientset
	config     *rest.Config
	newPodName string
	namespace  string
}

type kmimeParams struct {
	sourcePod    string
	commandToRun []string
	namespace    string
	prefix       string
	suffix       string
	labels       map[string]string
	envs         []v1.EnvVar
	user         string
	envFile      string
}

func NewModel(params *kmimeParams) model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle
	return model{
		params:     params,
		spinner:    s,
		statusText: "Connecting to Kubernetes cluster...",
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, connectToKubeCmd)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case errorMsg:
		m.err = msg.err
		return m, tea.Quit

	case kubeConnectedMsg:
		m.clientset = msg.clientset
		m.config = msg.config
		m.statusText = fmt.Sprintf("Fetching source pod '%s'...", m.params.sourcePod)
		return m, fetchPodCmd(m.clientset, m.params.namespace, m.params.sourcePod)

	case podFetchedMsg:
		m.statusText = "Generating new pod specification..."
		return m, createPodCmd(m)

	case podCreatedMsg:
		m.newPodName = msg.podName
		m.statusText = fmt.Sprintf("Waiting for pod '%s' to start...", m.newPodName)
		return m, waitForPodCmd(m.clientset, m.params.namespace, m.newPodName)

	case podRunningMsg:
		m.newPodName = msg.podName
		m.statusText = fmt.Sprintf("Attaching to pod '%s'...", m.newPodName)
		return m, tea.Sequence(
			tea.EnterAltScreen,
			func() tea.Msg { return attachMsg{} },
		)

	case attachMsg:
		time.Sleep(1 * time.Second)
		err := attachToPod(m.clientset, m.config, m.params.namespace, m.newPodName, m.params.commandToRun)
		if err != nil && !strings.Contains(err.Error(), "exit status") {
			return m, func() tea.Msg { return errorMsg{err} }
		}
		return m, tea.Sequence(
			tea.ExitAltScreen,
			func() tea.Msg { return podAttachedMsg{} },
		)

	case podAttachedMsg:
		m.statusText = fmt.Sprintf("Cleaning up pod '%s'...", m.newPodName)
		return m, cleanupPodCmd(m.clientset, m.params.namespace, m.newPodName)

	case podCleanedUpMsg:
		m.statusText = fmt.Sprintf("Pod '%s' removed successfully.", m.newPodName)
		return m, func() tea.Msg {
			time.Sleep(1 * time.Second)
			return finalSuccessMsg{message: "Session finished successfully!"}
		}

	case finalSuccessMsg:
		m.statusText = msg.message
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

func (m model) View() string {
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("\nError: %v\n", m.err))
	}

	if m.done {
		return successStyle.Render(fmt.Sprintf("\n%s\n", m.statusText))
	}

	return fmt.Sprintf("\n %s %s\n", m.spinner.View(), statusStyle.Render(m.statusText))
}

func connectToKubeCmd() tea.Msg {
	time.Sleep(1 * time.Second)
	clientset, config, err := getKubeConfig()
	if err != nil {
		return errorMsg{err}
	}
	return kubeConnectedMsg{clientset, config}
}

func fetchPodCmd(clientset *kubernetes.Clientset, namespace, sourcePod string) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(1 * time.Second)
		_, err := getPod(clientset, namespace, sourcePod)
		if err != nil {
			return errorMsg{err}
		}
		return podFetchedMsg{}
	}
}

func createPodCmd(m model) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(1 * time.Second)
		originalPod, _ := getPod(m.clientset, m.params.namespace, m.params.sourcePod)
		newPodSpec := clonePod(originalPod, m.params.user, m.params.commandToRun, m.params.prefix, m.params.suffix, m.params.labels, m.params.envs)

		createdPod, err := createPod(m.clientset, newPodSpec)
		if err != nil {
			return errorMsg{err}
		}

		entry := logEntry{
			Timestamp:  time.Now(),
			NewPodName: createdPod.Name,
			SourcePod:  m.params.sourcePod,
			Namespace:  m.params.namespace,
			User:       m.params.user,
			Command:    m.params.commandToRun,
			Prefix:     m.params.prefix,
			Suffix:     m.params.suffix,
			Labels:     m.params.labels,
			EnvFile:    m.params.envFile,
		}
		if err := appendLog(entry); err != nil {
			log.Printf("Warning: could not write to log file: %v", err)
		}

		return podCreatedMsg{podName: createdPod.Name}
	}
}

func waitForPodCmd(clientset *kubernetes.Clientset, namespace, podName string) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(1 * time.Second)
		err := waitForPodRunning(clientset, namespace, podName, time.Minute*2)
		if err != nil {
			return errorMsg{err}
		}
		return podRunningMsg{podName: podName}
	}
}

func cleanupPodCmd(clientset *kubernetes.Clientset, namespace, podName string) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(1 * time.Second)
		if err := deletePod(clientset, namespace, podName); err != nil {
			return errorMsg{fmt.Errorf("failed to clean up pod '%s': %w", podName, err)}
		}
		return podCleanedUpMsg{podName: podName}
	}
}
