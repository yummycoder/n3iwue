package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/free5gc/n3iwue/internal/logger"
	"github.com/free5gc/n3iwue/pkg/factory"
	"github.com/free5gc/n3iwue/pkg/service"
)

func main() {
	configPath := flag.String("config", "./config/n3ue.yaml", "path to the N3IWUE YAML configuration file")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [-config path]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}

	if err := factory.Initialize(*configPath); err != nil {
		logger.CfgLog.Fatalf("%+v", err)
	}
	service.Start()
}
