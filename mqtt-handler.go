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
	"encoding/json"
	"log"
	"path/filepath"
	"strings"

	"github.com/dnstapir/tapir"
	"github.com/spf13/viper"
)

type MqttHandler struct {
	engine    *tapir.MqttEngine
	logger    *Logger
	PopStatus map[string]tapir.TapirFunctionStatus // map[id]status
	EdmStatus map[string]tapir.TapirFunctionStatus // map[id]status
	// ...
}

func NewMqttHandler(config *Config, logger *Logger) (*MqttHandler, error) {
	statusCh := make(chan tapir.ComponentStatusUpdate, 100)
	engine, err := tapir.NewMqttEngine("tapir-slogger", config.TapirConfig.MqttConfig.ClientID, tapir.TapirSub, statusCh, log.Default())
	if err != nil {
		return nil, err
	}

	return &MqttHandler{
		engine:    engine,
		logger:    logger,
		PopStatus: make(map[string]tapir.TapirFunctionStatus),
		EdmStatus: make(map[string]tapir.TapirFunctionStatus),
	}, nil
}

func (h *MqttHandler) Start() {
	statusTopic := viper.GetString("tapir.status.topic")
	if statusTopic == "" {
		TEMExiter("MQTT Engine %s: MQTT status topic not set", h.engine.Creator)
	}

	keyfile := viper.GetString("tapir.status.validatorkey")
	if keyfile == "" {
		TEMExiter("MQTT Engine %s: MQTT status validator key not set", h.engine.Creator)
	}
	keyfile = filepath.Clean(keyfile)
	validatorkey, err := tapir.FetchMqttValidatorKey(statusTopic, keyfile)
	if err != nil {
		TEMExiter("MQTT Engine %s: Error fetching MQTT validator key for topic %s: %v", h.engine.Creator, statusTopic, err)
	}

	log.Printf("MQTT Engine %s: Adding topic '%s' to MQTT Engine", h.engine.Creator, statusTopic)

	subch := make(chan tapir.MqttPkgIn, 100)
	_, err = h.engine.SubToTopic(statusTopic, validatorkey, subch, "struct", true)
	if err != nil {
		TEMExiter("Error adding sub topic %s to MQTT Engine: %v", statusTopic, err)
	}
	log.Printf("MQTT Engine %s: Starting", h.engine.Creator)
	_, _, _, err = h.engine.StartEngine()
	if err != nil {
		log.Fatalf("Error starting MQTT engine: %v", err)
	}

	for pkg := range subch {

		//		switch mqttMsg.(type) {
		//		case tapir.MqttData:
		//			p := mqttMsg.(tapir.MqttData)
		// log.Printf("MQTT Engine %s: Received message: %+v", h.engine.Creator, p)
		log.Printf("TAPIR-SLOGGER MQTT Handler: Received message on topic %s", pkg.Topic)

		switch {
		case strings.HasPrefix(pkg.Topic, "status/up/"):
			parts := strings.Split(pkg.Topic, "/")
			if len(parts) == 4 {
				edgeId := parts[2]
				edgeComponent := parts[3]

				var tfs tapir.TapirFunctionStatus
				err := json.Unmarshal(pkg.Payload, &tfs)
				if err != nil {
					log.Printf("MQTT: failed to decode json: %v", err)
					continue
				}

				log.Printf("Received status update from sender: %s, component: %s", edgeId, edgeComponent)
				h.logger.LogStatus(edgeId, edgeComponent, tfs)
				h.updateStatus(tfs)
			} else {
				log.Printf("TAPIR-SLOGGER MQTT Handler: Invalid topic format: %s", pkg.Topic)
			}

		default:
			log.Printf("TAPIR-SLOGGER MQTT Handler: Received message on unknown topic %s", pkg.Topic)
		}
	}
}

func (h *MqttHandler) Stop() {
	h.engine.StopEngine()
}

func (h *MqttHandler) updateStatus(status tapir.TapirFunctionStatus) {
	h.PopStatus[status.FunctionID] = status
}

func (h *MqttHandler) GetStatus() tapir.SloggerCmdResponse {
	resp := tapir.SloggerCmdResponse{
		PopStatus: h.PopStatus,
		EdmStatus: h.EdmStatus,
	}
	return resp
}
