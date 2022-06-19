package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/cryptic-io/pmux/pmuxlib"

	"gopkg.in/yaml.v2"
)

func main() {

	cfgPath := flag.String("c", "./pmux.yml", "Path to config yaml file")
	flag.Parse()

	cfgB, err := ioutil.ReadFile(*cfgPath)
	if err != nil {
		panic(fmt.Sprintf("couldn't read cfg file at %q: %v", *cfgPath, err))
	}

	var cfg pmuxlib.Config
	if err := yaml.Unmarshal(cfgB, &cfg); err != nil {
		panic(fmt.Sprintf("couldn't parse cfg file: %v", err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigCh := make(chan os.Signal, 2)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

		<-sigCh
		cancel()

		<-sigCh
		fmt.Fprintln(os.Stderr, "forcefully exiting pmux process, there may be zombie child processes being left behind, good luck!")
		os.Stderr.Sync()
		os.Exit(1)
	}()

	pmuxlib.Run(ctx, cfg)
}
