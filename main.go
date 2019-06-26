package main

import (
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/kardianos/service"
	"github.com/ziyoubiancheng/xbook/commands"
	"github.com/ziyoubiancheng/xbook/commands/daemon"
	_ "github.com/ziyoubiancheng/xbook/routers"
)

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "service" {
		if os.Args[2] == "install" {
			daemon.Install()
		} else if os.Args[2] == "remove" {
			daemon.Uninstall()
		} else if os.Args[2] == "restart" {
			daemon.Restart()
		}
	}
	commands.RegisterCommand()

	d := daemon.NewDaemon()

	s, err := service.New(d, d.Config())

	if err != nil {
		os.Exit(1)
	}

	s.Run()
}
