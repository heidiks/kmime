package main

import (
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

var rootCmd = &cobra.Command{
	Use:   "kmime [source-pod] [command]",
	Short: "kmime creates a temporary, interactive pod by cloning an existing one.",
	Long: `kmime helps in debugging and running one-off tasks in Kubernetes.

It copies the specifications of an existing pod (like environment variables,
volumes, and service accounts) to create a new pod in interactive mode.
This is useful for tasks like running batch jobs or exploring a pod's environment
without altering the original pod.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var commandToRun []string
		if len(args) > 1 {
			commandToRun = args[1:]
		} else {
			commandToRun = []string{"bash"}
		}

		namespace, _ := cmd.Flags().GetString("namespace")
		prefix, _ := cmd.Flags().GetString("prefix")
		suffix, _ := cmd.Flags().GetString("suffix")
		labelStrs, _ := cmd.Flags().GetStringArray("label")
		envFile, _ := cmd.Flags().GetString("env-file")
		preview, _ := cmd.Flags().GetBool("preview")

		labels, err := parseLabels(labelStrs)
		if err != nil {
			log.Fatalf("Error processing labels: %v", err)
		}

		envs, err := parseEnvFile(envFile)
		if err != nil {
			log.Fatalf("Error processing env file: %v", err)
		}

		skipIdentification, _ := cmd.Flags().GetBool("skip-identification")
		var user string
		if !skipIdentification {
			user, err = getUserIdentifier()
			if err != nil {
				log.Fatalf("Error getting user identifier: %v", err)
			}
		}

		if preview {
			clientset, _, err := getKubeConfig()
			if err != nil {
				log.Fatalf("Could not get Kubernetes config: %v", err)
			}
			originalPod, err := getPod(clientset, namespace, args[0])
			if err != nil {
				log.Fatalf("Could not get source pod: %v", err)
			}

			podSpec := clonePod(originalPod, user, commandToRun, prefix, suffix, labels, envs)
			yamlData, err := yaml.Marshal(podSpec)
			if err != nil {
				log.Fatalf("Could not marshal pod spec to YAML: %v", err)
			}

			fileName := "kmime-preview.yaml"
			err = os.WriteFile(fileName, yamlData, 0644)
			if err != nil {
				log.Fatalf("Could not write YAML to file: %v", err)
			}
			fmt.Printf("Pod specification saved to %s\n", fileName)
			return
		}

		params := &kmimeParams{
			sourcePod:    args[0],
			commandToRun: commandToRun,
			namespace:    namespace,
			prefix:       prefix,
			suffix:       suffix,
			labels:       labels,
			envs:         envs,
			user:         user,
			envFile:      envFile,
		}

		p := tea.NewProgram(NewModel(params))
		if _, err := p.Run(); err != nil {
			fmt.Printf("An error occurred during execution: %v\n", err)
			os.Exit(1)
		}
	},
}

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Displays the execution history of kmime.",
	Run: func(cmd *cobra.Command, args []string) {
		model, err := NewHistoryModel()
		if err != nil {
			log.Fatalf("Error creating history view: %v", err)
		}

		p := tea.NewProgram(model)
		if _, err := p.Run(); err != nil {
			fmt.Printf("An error occurred during execution: %v\n", err)
			os.Exit(1)
		}
	},
}

func Execute() {
	rootCmd.AddCommand(historyCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringP("namespace", "n", "", "Namespace of the source pod (required)")
	rootCmd.MarkFlagRequired("namespace")
	rootCmd.Flags().String("prefix", "", "Prefix for the new pod's name")
	rootCmd.Flags().String("suffix", "", "Suffix for the new pod's name")
	rootCmd.Flags().StringArrayP("label", "l", []string{}, "Add a label to the new pod (e.g., -l key=value)")
	rootCmd.Flags().String("env-file", "", "Path to a file with environment variables to add to the pod")
	rootCmd.Flags().Bool("skip-identification", false, "Skip appending user identification to the pod name")
	rootCmd.Flags().Bool("preview", false, "Preview the generated pod specification as YAML without creating it")
}

func main() {
	Execute()
}
