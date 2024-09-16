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
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"log"
	"strings"

	"github.com/dnstapir/tapir"
	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/v2/jws"
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

	// subch := make(chan tapir.MqttPkgIn, 100)
	_, err := h.engine.SubToTopic(pubkeyTopic, nil, h.PubKeyCh, "struct", false)
	if err != nil {
		TEMExiter("Error adding sub topic %s to MQTT Engine: %v", pubkeyTopic, err)
	}

	cn, caCertPool, clientCert, err := tapir.FetchTapirClientCert(log.Default(), nil)
	if err != nil {
		TEMExiter("Error fetching MQTT client cert: %v", err)
	}
	log.Printf("Common Name: %s, CA Cert Pool: %d, Client Cert: %+v", cn, len(caCertPool.Subjects()), clientCert)

	for pkg := range h.PubKeyCh {
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

				// Parse the client certificate from PEM format
				block, _ := pem.Decode([]byte(pku.ClientCertPEM))
				if block == nil {
					log.Printf("Failed to decode PEM block containing the client certificate")
					continue
				}

				clientCert, err := x509.ParseCertificate(block.Bytes)
				if err != nil {
					log.Printf("Failed to parse client certificate: %v", err)
					continue
				}

				// Verify the client certificate using the caCertPool
				_, err = clientCert.Verify(x509.VerifyOptions{
					Roots: caCertPool,
				})
				if err != nil {
					log.Printf("Failed to verify client certificate: %v", err)
					// List the authorities in the cert pool
					for _, subject := range caCertPool.Subjects() {
						cert, _ := x509.ParseCertificate(subject)
						log.Printf("Cert Pool Authority: Subject: '%s', Issuer: '%s', NotBefore: '%v', NotAfter: '%v'",
							cert.Subject, cert.Issuer, cert.NotBefore, cert.NotAfter)
					}
					log.Printf("Client certificate was signed by: %s", clientCert.Issuer)
					continue
				}

				// Validate the JWS signature using the client certificate
				payload, err := jws.Verify([]byte(pku.JWSMessage), jws.WithKey(jwa.ES256, clientCert.PublicKey))
				if err != nil {
					log.Printf("Failed to verify JWS signature: %v", err)
					continue
				}

				// Print the client public key
				log.Printf("Verified public key: %s", string(payload))

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
