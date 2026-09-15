package main

import (
	"log"
	"os"

	"github.com/bytecode/modbus-mapping-gateway/internal/adapter/in/httpapi"
	"github.com/bytecode/modbus-mapping-gateway/internal/adapter/out/jsonstore"
	"github.com/bytecode/modbus-mapping-gateway/internal/adapter/out/modbus"
	"github.com/bytecode/modbus-mapping-gateway/internal/adapter/out/yamlstore"
	"github.com/bytecode/modbus-mapping-gateway/internal/usecase"
)

func main() {
	addr := env("HTTP_ADDR", ":8080")
	mappingFile := env("MAPPING_FILE", "configs/mapping.yaml")
	defaultFile := env("MAPPING_DEFAULT", "configs/mapping.yaml")
	alarmFile := env("ALARM_FILE", "data/alarms.json")

	if err := yamlstore.EnsureDefault(mappingFile, defaultFile); err != nil {
		// if same path, ignore; otherwise try continue when file already exists
		if mappingFile != defaultFile {
			log.Printf("ensure default mapping: %v", err)
		}
	}

	store := yamlstore.New(mappingFile)
	client := modbus.NewClient()
	alarmSvc, err := usecase.NewAlarmService(jsonstore.NewAlarmStore(alarmFile), nil)
	if err != nil {
		log.Fatalf("init alarm store: %v", err)
	}
	svc, err := usecase.NewGatewayService(store, client, alarmSvc)
	if err != nil {
		log.Fatalf("load mapping: %v", err)
	}

	srv := httpapi.NewServer(svc)
	log.Printf("modbus mapping gateway listening on %s, mapping=%s, alarms=%s", addr, mappingFile, alarmFile)
	if err := srv.Router().Run(addr); err != nil {
		log.Fatal(err)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
