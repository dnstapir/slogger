// Copyright 2024 Johan Stenstam, johan.stenstam@internetstiftelsen.se
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

	meng, err := tapir.NewMqttEngine("tapir-slogger", config.TapirConfig.MqttConfig.ClientID, tapir.TapirSub, nil, log.Default())
	if err != nil {
		log.Fatalf("Error initializing MQTT engine: %v", err)
	}
	config.MqttEngine = meng
	log.Printf("MQTT Engine: Starting")
	_, _, _, err = meng.StartEngine()
	if err != nil {
		log.Fatalf("Error starting MQTT engine: %v", err)
	}

	// Initialize Status Receiver
	srecv, err := NewStatusReceiver(config, logger)
	if err != nil {
		log.Fatalf("Error initializing Status Receiver: %v", err)
	}
	config.StatusReceiver = srecv

	// Start MQTT handler
	go srecv.Start()

	// Initialize PubKeyReceiver
	pkeyrecv, err := NewPubKeyReceiver(config, logger)
	if err != nil {
		log.Fatalf("Error initializing PubKey Receiver: %v", err)
	}
	config.PubKeyReceiver = pkeyrecv

	// Start MQTT handler
	go pkeyrecv.Start()

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
	config.MqttEngine.StopEngine()
	logger.Close()
	log.Println("tapir-slogger stopped")
}
