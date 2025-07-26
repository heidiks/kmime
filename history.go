package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

type historyModel struct {
	table table.Model
}

func (m historyModel) Init() tea.Cmd { return nil }

func (m historyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.table.SetHeight(msg.Height - 4)
		m.table.SetWidth(msg.Width - 4)
		return m, nil
	}
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m historyModel) View() string {
	return baseStyle.Render(m.table.View()) + "\n  Use ↑/↓ to navigate, q to quit\n"
}

func NewHistoryModel() (*historyModel, error) {
	columns := []table.Column{
		{Title: "Timestamp", Width: 20},
		{Title: "New Pod", Width: 30},
		{Title: "Source Pod", Width: 30},
		{Title: "Namespace", Width: 20},
		{Title: "User", Width: 20},
		{Title: "Command", Width: 30},
	}

	var entries []logEntry
	if _, err := os.Stat(logFileName); err == nil {
		file, err := os.ReadFile(logFileName)
		if err != nil {
			return nil, fmt.Errorf("could not read log file: %w", err)
		}
		if len(file) > 0 {
			if err := json.Unmarshal(file, &entries); err != nil {
				return nil, fmt.Errorf("could not parse log file: %w", err)
			}
		}
	}

	var rows []table.Row
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		rows = append(rows, table.Row{
			entry.Timestamp.Format("2006-01-02 15:04:05"),
			entry.NewPodName,
			entry.SourcePod,
			entry.Namespace,
			entry.User,
			strings.Join(entry.Command, " "),
		})
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return &historyModel{table: t}, nil
}
