package main

import (
	"fmt"
	"gdxsv/pkg/config"
	"gdxsv/pkg/lobby"
	"os"
	"os/signal"
)

func mainLobby() {
	app := lobby.NewApp()
	go app.Serve()
	sv := lobby.NewServer(app)
	go sv.ListenAndServe(stripHost(config.Conf.LobbyAddr))

	c := make(chan os.Signal, 1)
	signal.Notify(c)
	s := <-c
	fmt.Println("Got signal:", s)
	app.Quit()
}
