// SPDX-FileCopyrightText: 2023 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"os"

	"github.com/omec-project/sctplb/logger"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Info          *Info          `yaml:"info"`
	Configuration *Configuration `yaml:"configuration"`
	Logger        *Logger        `yaml:"logger"`
}

type Logger struct{}

type Info struct {
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`
}

type Service struct {
	Uri string `yaml:"uri,omitempty"`
}

type Configuration struct {
	Type         string    `yaml:"type,omitempty" valid:"required,in(grpc)"`
	Services     []Service `yaml:"services,omitempty"`
	NgapIpList   []string  `yaml:"ngapIpList,omitempty"`
	NgapPort     int       `yaml:"ngappPort,omitempty"`
	SctpGrpcPort int       `yaml:"sctpGrpcPort,omitempty"`
}

func InitConfigFactory(f string) (Config, error) {
	var SimappConfig Config
	if content, err := os.ReadFile(f); err != nil {
		logger.CfgLog.Errorf("readfile failed called %v", err)
		return SimappConfig, err
	} else {
		SimappConfig = Config{}

		if yamlErr := yaml.Unmarshal(content, &SimappConfig); yamlErr != nil {
			logger.CfgLog.Errorf("yaml parsing failed %v", yamlErr)
			return SimappConfig, yamlErr
		}
	}
	if SimappConfig.Configuration == nil {
		logger.CfgLog.Errorf("configuration parsing failed %v", SimappConfig.Configuration)
		return SimappConfig, errors.New("configuration parsing failed")
	}
	return SimappConfig, nil
}
