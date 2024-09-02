package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	flag "github.com/spf13/pflag"

	"github.com/dnstapir/tapir"
	"github.com/spf13/viper"
)

var TEMExiter = func(args ...interface{}) {
	log.Printf("TEMExiter: [placeholderfunction w/o real cleanup]")
	log.Printf("TEMExiter: Exit message: %s", fmt.Sprintf(args[0].(string), args[1:]...))
	os.Exit(1)
}

func main() {
	flag.BoolVarP(&tapir.GlobalCF.Debug, "debug", "d", false, "Debug mode")
	flag.BoolVarP(&tapir.GlobalCF.Verbose, "verbose", "v", false, "Verbose mode")
	flag.Parse()

	var cfgFile string
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigFile(tapir.DefaultSloggerCfgFile)
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	} else {
		TEMExiter("Could not load config %s: Error: %v", tapir.DefaultSloggerCfgFile, err)
	}

	// Load configuration
	config, err := LoadConfig(tapir.DefaultSloggerCfgFile)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	log.Printf("Config loaded: %+v", config)
	// Initialize logger
	logger := NewLogger(config.LogConfig.File)

	// Initialize MQTT handler
	mqttHandler, err := NewMqttHandler(config, logger)
	if err != nil {
		log.Fatalf("Error initializing MQTT handler: %v", err)
	}
	config.MqttHandler = mqttHandler

	// Start MQTT handler
	go mqttHandler.Start()

	// Initialize API handler
	//apiHandler := NewAPIHandler(config, mqttHandler)

	var done_ch = make(chan struct{})
	// Start API handler
	// go apiHandler.Start(config)
	go APIhandler(config, done_ch)

	// Handle termination signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	// Cleanup
	mqttHandler.Stop()
	logger.Close()
	log.Println("tapir-slogger stopped")
}
