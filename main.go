// git-diff-mygo is a Git client whose window is a React page in the OS
// webview and whose repository work happens in Go through MyGo bindings.
package main

import (
	"log"
	"os"

	"github.com/egoist/mygo"

	"git-diff-mygo/internal/service"
)

func main() {
	repo := &service.RepoService{}
	mygo.Bind(repo)
	actions := service.NewMenuActions(repo)

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "Git Diff",
			Width:           1360,
			Height:          840,
			MinWidth:        960,
			MinHeight:       600,
			BackgroundColor: "light-dark(#ffffff, #0b0b0c)",
			StateKey:        "main",
			URL:             "/",
		})
		service.Attach(repo, win)

		// Dropping a folder (or any file inside a repository) opens it.
		win.OnFileDrop(func(e *mygo.FileDropEvent) {
			if len(e.Paths) > 0 {
				actions.OpenDropped(e.Paths[0])
			}
		})

		// A path on the command line opens that repository at startup, which
		// also makes "Open With" and scripted launches work.
		if len(os.Args) > 1 {
			actions.OpenDropped(os.Args[1])
		}

		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Role: mygo.RoleAppMenu},
			{Label: "File", Submenu: []*mygo.MenuItem{
				{
					Label:       "Open Repository…",
					Accelerator: "CmdOrCtrl+O",
					Click: func(*mygo.MenuItem, *mygo.Window) {
						actions.OpenFromMenu()
					},
				},
				{
					Label:       "Refresh",
					Accelerator: "CmdOrCtrl+R",
					Click: func(*mygo.MenuItem, *mygo.Window) {
						actions.RefreshFromMenu()
					},
				},
				mygo.Separator(),
				{Role: mygo.RoleClose},
			}},
			{Role: mygo.RoleEditMenu},
			{Role: mygo.RoleViewMenu},
			{Role: mygo.RoleWindowMenu},
		}))
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
