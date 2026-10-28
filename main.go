// Twill is a Git client drawn by MyGo's native UI; repository work happens in
// Go and the window is a view function, with no web page.
package main

import (
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"twill/internal/service"
	"twill/internal/shell"
)

func main() {
	repo := &service.RepoService{}
	actions := service.NewMenuActions(repo)
	app := shell.New(repo, actions)

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "Twill",
			Width:           1360,
			Height:          840,
			MinWidth:        960,
			MinHeight:       600,
			BackgroundColor: "light-dark(#ffffff, #0b0b0c)",
			StateKey:        "main",
			Content:         ui.View(app.View),
		})
		app.Attach(win)
		service.Attach(repo, win)

		// Dropping a folder (or any file inside a repository) opens it.
		win.OnFileDrop(func(e *mygo.FileDropEvent) {
			if len(e.Paths) > 0 {
				actions.OpenDropped(e.Paths[0])
			}
		})

		// A path on the command line opens that repository at startup, which
		// also makes "Open With" and scripted launches work. Without one, the
		// repository from the previous session comes back.
		arg := ""
		if len(os.Args) > 1 {
			arg = os.Args[1]
		}
		service.OpenInitial(repo, arg)

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
