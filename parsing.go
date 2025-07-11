package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	v1 "k8s.io/api/core/v1"
)

func parseLabels(labels []string) (map[string]string, error) {
	labelMap := make(map[string]string)
	for _, l := range labels {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid label format: %s, expected key=value", l)
		}
		labelMap[parts[0]] = parts[1]
	}
	return labelMap, nil
}

func parseEnvFile(filePath string) ([]v1.EnvVar, error) {
	if filePath == "" {
		return nil, nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not open env file %s: %w", filePath, err)
	}
	defer file.Close()

	var envs []v1.EnvVar
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			if strings.HasSuffix(line, "=") {
				envs = append(envs, v1.EnvVar{Name: parts[0], Value: ""})
			}
			continue
		}
		envs = append(envs, v1.EnvVar{Name: parts[0], Value: parts[1]})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading env file %s: %w", filePath, err)
	}

	return envs, nil
}
