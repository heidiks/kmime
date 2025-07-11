package main

import (
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
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
		sourcePod := args[0]
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

		log.Println("Starting kmime...")

		labels, err := parseLabels(labelStrs)
		if err != nil {
			log.Fatalf("❌ Error parsing labels: %v", err)
		}
		envs, err := parseEnvFile(envFile)
		if err != nil {
			log.Fatalf("❌ Error parsing env file: %v", err)
		}

		skipIdentification, _ := cmd.Flags().GetBool("skip-identification")
		var user string
		if !skipIdentification {
			var err error
			user, err = getUserIdentifier()
			if err != nil {
				log.Fatalf("❌ Error getting user identifier: %v", err)
			}
			log.Printf("✅ User identifier: %s", user)
		}

		log.Println("🔄 Connecting to Kubernetes cluster...")
		clientset, config, err := getKubeConfig()
		if err != nil {
			log.Fatalf("❌ Error connecting to Kubernetes: %v", err)
		}
		log.Println("✅ Successfully connected to Kubernetes.")

		log.Printf("🔄 Fetching source pod '%s' in namespace '%s'...", sourcePod, namespace)
		originalPod, err := getPod(clientset, namespace, sourcePod)
		if err != nil {
			log.Fatalf("❌ Error fetching source pod: %v", err)
		}
		log.Printf("✅ Found source pod '%s'.", originalPod.Name)

		log.Println("🔄 Generating new pod specification...")
		newPod := clonePod(originalPod, user, commandToRun, prefix, suffix, labels, envs)
		log.Printf("✅ New pod spec created with name '%s'.", newPod.Name)

		log.Printf("🚀 Creating pod '%s'...", newPod.Name)
		createdPod, err := createPod(clientset, newPod)
		if err != nil {
			log.Fatalf("❌ Error creating pod: %v", err)
		}
		log.Printf("✅ Pod '%s' created.", createdPod.Name)

		entry := logEntry{
			Timestamp:  time.Now(),
			NewPodName: createdPod.Name,
			SourcePod:  sourcePod,
			Namespace:  namespace,
			User:       user,
			Command:    commandToRun,
			Prefix:     prefix,
			Suffix:     suffix,
			Labels:     labels,
			EnvFile:    envFile,
		}
		if err := appendLog(entry); err != nil {
			log.Printf("⚠️  Could not write to log file: %v", err)
		}

		defer func() {
			log.Printf("🧹 Cleaning up pod '%s'...", createdPod.Name)
			if err := deletePod(clientset, createdPod.Namespace, createdPod.Name); err != nil {
				log.Printf("⚠️  Error deleting pod '%s': %v", createdPod.Name, err)
			} else {
				log.Printf("✅ Pod '%s' deleted.", createdPod.Name)
			}
		}()

		log.Printf("⏳ Waiting for pod '%s' to be running...", createdPod.Name)
		err = waitForPodRunning(clientset, createdPod.Namespace, createdPod.Name, time.Minute*2)
		if err != nil {
			log.Fatalf("❌ Error waiting for pod: %v", err)
		}

		log.Printf("🔗 Attaching to pod '%s'...", createdPod.Name)
		err = attachToPod(clientset, config, createdPod.Namespace, createdPod.Name, commandToRun)
		if err != nil {
			log.Fatalf("❌ Error attaching to pod: %v", err)
		}

		log.Println("✅ Session ended.")
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Println(err)
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
}

func main() {
	Execute()
}
