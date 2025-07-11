package main

import (
	"encoding/json"
	"os"
	"time"
)

type logEntry struct {
	Timestamp  time.Time         `json:"timestamp"`
	NewPodName string            `json:"new_pod_name"`
	SourcePod  string            `json:"source_pod"`
	Namespace  string            `json:"namespace"`
	User       string            `json:"user"`
	Command    []string          `json:"command"`
	Prefix     string            `json:"prefix,omitempty"`
	Suffix     string            `json:"suffix,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	EnvFile    string            `json:"env_file,omitempty"`
}

const logFileName = "kmime_log.json"

func appendLog(entry logEntry) error {
	var entries []logEntry

	if _, err := os.Stat(logFileName); err == nil {
		file, err := os.ReadFile(logFileName)
		if err != nil {
			return err
		}
		if len(file) > 0 {
			if err := json.Unmarshal(file, &entries); err != nil {
				return err
			}
		}
	}

	entries = append(entries, entry)

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(logFileName, data, 0644)
}
