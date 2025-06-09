package main

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func newMainModel(dupes []DupeSet) tea.Model {
	m := m_model{exitMsg: "", dupes: dupes}
	return m
}

type m_model struct {
	showFinalPrompt bool
	shouldExit      bool
	Pos             int
	exitMsg         string
	dupes           []DupeSet
	toDelete        []string
}

func (m m_model) IsSelected(path string) bool {
	return slices.Contains(m.toDelete, path)
}

func (m m_model) Init() tea.Cmd {
	return tea.EnterAltScreen
}

func (m m_model) View() string {
	if m.showFinalPrompt {
		return "press ENTER to perform actions\nESC to go back"
	}
	curr := m.Curr()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d/%d\n", m.Pos+1, len(m.dupes)))
	for i, path := range curr.Paths {
		b.WriteString("\n")
		if i == m.Curr().Pos {
			b.WriteString("  ")
		}
		b.WriteString(path)
		if m.IsSelected(path) {
			b.WriteString(" !")
		}
	}
	b.WriteString("\n\n" + string(curr.SixelImg))
	return b.String()
}

func (m m_model) Curr() *DupeSet {
	return &m.dupes[m.Pos]
}

func (m m_model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmds []tea.Cmd
	)

	oldPos1 := m.Pos

	switch msg := msg.(type) {

	case tea.KeyMsg:
		key := msg.String()
		curr := m.Curr()

		if m.showFinalPrompt && key != "esc" && key != "enter" {
			if key != "left" {
				break
			}
			if m.showFinalPrompt {
				m.showFinalPrompt = false
				break
			}
		}

		switch key {
		case "left":
			if m.Pos != 0 {
				m.Pos--
			}
		case "right":
			m.Pos++
			if m.Pos == len(m.dupes) {
				m.showFinalPrompt = true
				m.Pos--
				return m, tea.ClearScreen
			}
		case "up":
			curr.Pos--
			if curr.Pos == -1 {
				curr.Pos = len(curr.Paths) - 1
			}
		case "down":
			curr.Pos++
			if curr.Pos == len(curr.Paths) {
				curr.Pos = 0
			}

		case "enter":
			if m.showFinalPrompt {
				return m, tea.Quit
			}
			target := curr.Paths[curr.Pos]
			if m.IsSelected(target) {
				for i, str := range m.toDelete {
					if str == target {
						m.toDelete = slices.Delete(m.toDelete, i, i+1)
					}
				}
			} else {
				m.toDelete = append(m.toDelete, target)
			}

		case "esc":
			if m.showFinalPrompt {
				m.showFinalPrompt = false
			}

		case "ctrl+c", "ctrl+d", "q":
			cmds = append(cmds, tea.ClearScreen, tea.Quit)
			return m, tea.Batch(cmds...)
		}

	case tea.WindowSizeMsg:
	}

	if oldPos1 != m.Pos {
		cmds = append(cmds, tea.ClearScreen)
	}

	return m, tea.Batch(cmds...)
}
