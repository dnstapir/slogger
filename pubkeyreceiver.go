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
	"strings"

	"github.com/dnstapir/tapir"
	"github.com/spf13/viper"
)

type PubKeyReceiver struct {
	engine    *tapir.MqttEngine
	logger    *Logger
	PubKeyCh  chan tapir.MqttPkgIn
	PopStatus map[string]tapir.TapirFunctionStatus // map[id]status
	EdmStatus map[string]tapir.TapirFunctionStatus // map[id]status
	// ...
}

func NewPubKeyReceiver(config *Config, logger *Logger) (*PubKeyReceiver, error) {
	pubkeyCh := make(chan tapir.MqttPkgIn, 100)

	return &PubKeyReceiver{
		engine:    config.MqttEngine,
		logger:    logger,
		PubKeyCh:  pubkeyCh,
		PopStatus: make(map[string]tapir.TapirFunctionStatus),
		EdmStatus: make(map[string]tapir.TapirFunctionStatus),
	}, nil
}

func (h *PubKeyReceiver) Start() {
	pubkeyTopic := viper.GetString("tapir.keyupload.topic")
	if pubkeyTopic == "" {
		TEMExiter("MQTT Engine %s: MQTT pubkey upload topic not set", h.engine.Creator)
	}

	log.Printf("MQTT Engine %s: Adding topic '%s' to MQTT Engine", h.engine.Creator, pubkeyTopic)

	subch := make(chan tapir.MqttPkgIn, 100)
	_, err := h.engine.SubToTopic(pubkeyTopic, nil, h.PubKeyCh, "struct", false)
	if err != nil {
		TEMExiter("Error adding sub topic %s to MQTT Engine: %v", pubkeyTopic, err)
	}

	for pkg := range subch {
		log.Printf("TAPIR-SLOGGER Pubkey Receiver: Received message on topic %s", pkg.Topic)

		switch {
		case strings.HasPrefix(pkg.Topic, "pubkey/up/"):
			parts := strings.Split(pkg.Topic, "/")
			if len(parts) == 4 {
				edgeId := parts[2]
				edgeComponent := parts[3]

				var pku tapir.PubKeyUpload
				err := json.Unmarshal(pkg.Payload, &pku)
				if err != nil {
					log.Printf("MQTT: failed to decode json: %v", err)
					continue
				}

				log.Printf("Received pubkey upload from sender: %s, component: %s\n%s", edgeId, edgeComponent, pku.JWSMessage)

			} else {
				log.Printf("TAPIR-SLOGGER Pubkey Receiver: Invalid topic format: %s", pkg.Topic)
			}

		default:
			log.Printf("TAPIR-SLOGGER Pubkey Receiver: Received message on unknown topic %s", pkg.Topic)
		}
	}
}

func (h *PubKeyReceiver) Stop() {
	h.engine.StopEngine()
}

func (h *PubKeyReceiver) updateStatus(status tapir.TapirFunctionStatus) {
	h.PopStatus[status.FunctionID] = status
}

func (h *PubKeyReceiver) GetStatus() tapir.SloggerCmdResponse {
	resp := tapir.SloggerCmdResponse{
		PopStatus: h.PopStatus,
		EdmStatus: h.EdmStatus,
	}
	return resp
}
