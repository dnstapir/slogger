package main

import (
	"log"
	"path/filepath"

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
	msg, err := h.engine.PubSubToTopic(statusTopic, nil, validatorkey, nil)
	if err != nil {
		TEMExiter("Error adding topic %s to MQTT Engine: %v", statusTopic, err)
	}
	log.Printf("MQTT Engine %s: Topic status for MQTT engine %s: %+v", h.engine.Creator, msg)

	log.Printf("MQTT Engine %s: Starting", h.engine.Creator)
	_, _, inbox, err := h.engine.StartEngine()
	if err != nil {
		log.Fatalf("Error starting MQTT engine: %v", err)
	}

	for msg := range inbox {
		log.Printf("MQTT Engine %s: Received message: %+v", h.engine.Creator, msg)
		if msg.Error {
			log.Printf("Error in received message: %v", msg.ErrorMsg)
			continue
		}

		h.logger.LogStatus(msg.Data.TapirFunctionStatus)
		h.updateStatus(msg.Data.TapirFunctionStatus)
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
