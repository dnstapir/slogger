package main

import (
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

type Config struct {
	BootTime    time.Time
	ApiConfig   ApiConfig   `yaml:"apiserver"`
	LogConfig   LogConfig   `yaml:"log"`
	TapirConfig TapirConfig `yaml:"tapir"`
	MqttHandler *MqttHandler
}

type TapirConfig struct {
	MqttConfig MqttConfig `yaml:"mqtt"`
}

type MqttConfig struct {
	ClientID string `yaml:"client_id"`
	Server   string `yaml:"server"`
	QoS      int    `yaml:"qos"`
}

type ApiConfig struct {
	Address string `yaml:"address"`
	Key     string `yaml:"key"`
}

type LogConfig struct {
	File string `yaml:"file"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	config.BootTime = time.Now()

	return &config, nil
}
