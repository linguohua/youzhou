package main

import (
	"flag"
	"io/ioutil"
	"youzhou/anchor"
	"youzhou/win"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"gopkg.in/yaml.v2"
)

var listenAddr string

type ServerCfg struct {
	ListenAddr    string                 `yaml:"listen_addr"`
	HardcoreDelay int                    `yaml:"hardcore_delay"`
	AnchorConfigs []*anchor.AnchorConfig `yaml:"anchors"`
}

func loadConfig(configFilePath string) *ServerCfg {
	data, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Fatal(err)
	}

	serverCfg := ServerCfg{}
	err = yaml.Unmarshal(data, &serverCfg)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	log.Infof("server configs:%+v", serverCfg)
	return &serverCfg
}

func main() {
	configFilePath := flag.String("path", "./yz.yaml", "config file path")
	flag.Parse()

	// Echo instance
	e := echo.New()
	if l, ok := e.Logger.(*log.Logger); ok {
		l.SetHeader("${time_rfc3339} ${level}")
	}

	cfg := loadConfig(*configFilePath)
	listenAddr = cfg.ListenAddr

	anchor.Register(e, cfg.AnchorConfigs, cfg.HardcoreDelay)
	win.Register(e)

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Start server
	e.Logger.Fatal(e.Start(listenAddr))
}
