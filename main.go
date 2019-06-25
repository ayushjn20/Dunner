package main

import (
	"github.com/ayushjn20/dunner/cmd"
	"github.com/ayushjn20/dunner/internal/settings"
	G "github.com/ayushjn20/dunner/pkg/global"
)

var version string

func main() {
	settings.Init()
	G.VERSION = version
	cmd.Execute()
}
