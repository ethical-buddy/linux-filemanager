package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"golang.org/x/term"
)

type FileManager struct {
	app        *tview.Application
	list       *tview.List
	details    *tview.TextView
	path       string
	items      []string
	oldState   *term.State // To store original terminal state
	flex       *tview.Flex
	vimView    *tview.TextView
	vimRunning bool
}

func NewFileManager(path string) *FileManager {
	fm := &FileManager{
		app:     tview.NewApplication(),
		list:    tview.NewList(),
		details: tview.NewTextView(),
		path:    path,
		flex:    tview.NewFlex(),
	}

	fm.loadItems()
	return fm
}

func (fm *FileManager) loadItems() {
	fm.list.Clear()
	entries, err := os.ReadDir(fm.path)
	if err != nil {
		panic(err)
	}

	var dirs, files []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		} else {
			files = append(files, entry.Name())
		}
	}

	sort.Strings(dirs)
	sort.Strings(files)

	fm.items = append(dirs, files...)
	for _, item := range fm.items {
		color := tcell.ColorWhite
		info, err := os.Stat(filepath.Join(fm.path, item))
		if err != nil {
			continue
		}
		if info.IsDir() {
			color = tcell.ColorBlue
		} else if (info.Mode() & os.ModeSymlink) != 0 {
			color = tcell.ColorFuchsia
		}
		fm.list.AddItem(item, "", 0, nil).SetMainTextColor(color)
	}

	fm.updateDetails()
}

func (fm *FileManager) navigate(item string) {
	newPath := filepath.Join(fm.path, item)
	info, err := os.Stat(newPath)
	if err != nil {
		return
	}

	if info.IsDir() {
		fm.path = newPath
		fm.loadItems()
	} else {
		fm.openInVim(newPath)
	}
}

func (fm *FileManager) openInVim(filepath string) {
	if fm.vimRunning {
		return
	}

	fm.vimRunning = true

	// Save current terminal state
	oldState, err := term.GetState(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	fm.oldState = oldState

	// Exit the tview application before opening Vim
	fm.app.Suspend(func() {
		cmd := exec.Command("vim", filepath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			panic(err)
		}

		if fm.oldState != nil {
			if err := term.Restore(int(os.Stdin.Fd()), fm.oldState); err != nil {
				panic(err)
			}
		}

		fm.vimRunning = false
		fm.loadItems() // Reload items after returning from Vim
	})
}

func (fm *FileManager) updateDetails() {
	selectedIndex := fm.list.GetCurrentItem()
	if selectedIndex < 0 || selectedIndex >= len(fm.items) {
		fm.details.SetText("")
		return
	}

	selectedItem := fm.items[selectedIndex]
	fullPath := filepath.Join(fm.path, selectedItem)

	info, err := os.Stat(fullPath)
	if err != nil {
		fm.details.SetText(fmt.Sprintf("Error retrieving details: %v", err))
		return
	}

	fileType := "File"
	if info.IsDir() {
		fileType = "Directory"
	} else if (info.Mode() & os.ModeSymlink) != 0 {
		fileType = "Symlink"
	} else if (info.Mode() & os.ModeNamedPipe) != 0 {
		fileType = "Named Pipe"
	} else if (info.Mode() & os.ModeSocket) != 0 {
		fileType = "Socket"
	} else if (info.Mode() & os.ModeDevice) != 0 {
		fileType = "Device"
	}

	permissions := info.Mode().String()
	owner := info.Sys().(*syscall.Stat_t).Uid
	group := info.Sys().(*syscall.Stat_t).Gid
	modTime := info.ModTime().Format(time.RFC1123)
	size := info.Size()

	details := fmt.Sprintf(
		"Name: %s\nType: %s\nSize: %d bytes\nPermissions: %s\nOwner: %d\nGroup: %d\nModified: %s\n",
		info.Name(),
		fileType,
		size,
		permissions,
		owner,
		group,
		modTime,
	)
	fm.details.SetText(details)
}

func (fm *FileManager) deleteSelectedItem() {
	selectedIndex := fm.list.GetCurrentItem()
	if selectedIndex < 0 || selectedIndex >= len(fm.items) {
		return
	}

	selectedItem := fm.items[selectedIndex]
	fullPath := filepath.Join(fm.path, selectedItem)

	err := os.RemoveAll(fullPath) // Remove files or directories
	if err != nil {
		fm.details.SetText(fmt.Sprintf("Error deleting file: %v", err))
		return
	}

	fm.loadItems()
}

func (fm *FileManager) run() {
	fm.list.SetSelectedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		fm.navigate(mainText)
	})

	fm.list.SetChangedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		fm.updateDetails()
	})

	fm.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if fm.vimRunning {
			return event
		}

		switch event.Key() {
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			fm.path = filepath.Dir(fm.path)
			fm.loadItems()
			return nil
		case tcell.KeyCtrlD:
			fm.deleteSelectedItem()
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case 'q':
				fm.app.Stop()
				return nil
			}
		}
		return event
	})

	fm.flex.SetDirection(tview.FlexColumn).
		AddItem(fm.list, 0, 1, true).
		AddItem(fm.details, 0, 1, false)

	fm.app.SetRoot(fm.flex, true)

	if err := fm.app.Run(); err != nil {
		panic(err)
	}
}

func main() {
	fm := NewFileManager(".")
	fm.run()
}
