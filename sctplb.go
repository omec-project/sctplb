// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"time"
)

//var msgChan chan datamodels.SctpLbMessage

type Config struct {
	Info          *Info          `yaml:"info"`
	Configuration *Configuration `yaml:"configuration"`
	Logger        *Logger        `yaml:"logger"`
}

type Logger struct {
}

type Info struct {
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`
}

type Configuration struct {
	ServiceName []string `yaml:"serviceNames,omitempty"`
	NgapIpList  []string `yaml:"ngapIpList,omitempty"`
	NgapPort    int `yaml:"ngappPort,omitempty"`
	SctpGrpcPort    int `yaml:"sctpGrpcPort,omitempty"`
}

const (
	add_op = iota
	modify_op
	delete_op
)

var SimappConfig Config

func InitConfigFactory(f string) error {
	if content, err := ioutil.ReadFile(f); err != nil {
		CfgLog.Errorf("Readfile failed called ", err)
		return err
	} else {
		SimappConfig = Config{}

		if yamlErr := yaml.Unmarshal(content, &SimappConfig); yamlErr != nil {
			CfgLog.Errorf("yaml parsing failed ", yamlErr)
			return yamlErr
		}
	}
	if SimappConfig.Configuration == nil {
		CfgLog.Errorf("Configuration Parsing Failed ", SimappConfig.Configuration)
		return nil
	}
	return nil
}

func main() {
	log.Println("SCTP LB started")

	InitConfigFactory("./config/sctplb.yaml")


	//Read messages from SCTP Sockets and push it on channel
	log.Println("SCTP Port ", SimappConfig.Configuration.NgapPort," grpc port : ", SimappConfig.Configuration.SctpGrpcPort)
	serviceRun(SimappConfig.Configuration.NgapIpList, SimappConfig.Configuration.NgapPort)

	for _, name := range SimappConfig.Configuration.ServiceName {
		go dispatchAddServer(name)
	}

	for {
		time.Sleep(100 * time.Second)
	}
	CfgLog.Errorf("Testing log %+v", 1)
}
