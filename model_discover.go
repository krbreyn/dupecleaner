package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type countMsg int

const (
	FileFoundCount countMsg = iota + 1
	FolderScannedCount
	ImageFoundCount
	FileHashedCount
	DuplicatesFoundCount
	ImageConvertedCount
	AllDoneMsg
)

func newDiscoverModel(sub chan countMsg, dir string) tea.Model {
	m := d_model{exitMsg: "", sub: sub, dir: dir}
	return m
}

func listenForCounts(sub chan countMsg) tea.Cmd {
	return func() tea.Msg {
		return <-sub
	}
}

type d_model struct {
	exitMsg    string
	shouldExit bool
	counts     struct {
		FilesFound, FoldersScanned, ImagesFound, FilesHashed, DuplicatesFound, ImagesConverted int
	}
	sub  chan countMsg
	dir  string
	done bool
}

func (m d_model) Init() tea.Cmd {
	return tea.Batch(tea.EnterAltScreen, listenForCounts(m.sub))
}

func (m d_model) View() string {
	var ret string
	ret += fmt.Sprintf("scanning %s\n%d folders scanned\n%d files found\n%d images found\n%d images hashed\n%d duplicates found\n%d images converted\n", m.dir, m.counts.FoldersScanned, m.counts.FilesFound, m.counts.ImagesFound, m.counts.FilesHashed, m.counts.DuplicatesFound, m.counts.ImagesConverted)
	if m.done {
		ret += "All done! Press ENTER to continue."
	}
	return ret
}

func (m d_model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {

	case countMsg:
		switch msg {
		case FileFoundCount:
			m.counts.FilesFound++
		case FolderScannedCount:
			m.counts.FoldersScanned++
		case ImageFoundCount:
			m.counts.ImagesFound++
		case FileHashedCount:
			m.counts.FilesHashed++
		case DuplicatesFoundCount:
			m.counts.DuplicatesFound++
		case ImageConvertedCount:
			m.counts.ImagesConverted++
		case AllDoneMsg:
			m.done = true
		}
		return m, listenForCounts(m.sub)

	case tea.KeyMsg:
		key := msg.String()

		switch key {

		case "ctrl+c", "ctrl+d":
			m.shouldExit = true
			return m, tea.Quit

		case "enter":
			if m.done {
				m.shouldExit = false
				return m, tea.Quit
			}
		}

	case tea.WindowSizeMsg:
	}

	return m, tea.Batch(cmds...)
}
