package main

import (
	"call-tester/internal/engine"
	"call-tester/internal/metrics"
	"call-tester/internal/models"
	"call-tester/internal/modem"
	"call-tester/internal/report"
	"flag"
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	configPath := "config.yaml"
	for i, a := range os.Args {
		if a == "-c" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
			os.Args = append(os.Args[:i], os.Args[i+2:]...)
			break
		}
	}

	switch os.Args[1] {
	case "check":
		cmdCheck(mustLoadConfig(configPath))
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		output := fs.String("o", "reports", "output dir")
		fs.Parse(os.Args[2:])
		if fs.NArg() < 1 {
			fmt.Println("usage: call-tester run <scenario.yaml>")
			os.Exit(1)
		}
		cmdRun(mustLoadConfig(configPath), mustLoadScenario(fs.Arg(0)), *output)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`call-tester — voice/SMS/data testing

Usage: call-tester [-c config.yaml] <command>

Commands:
  check                         Check all modems
  run <scenario.yaml> [-o dir]  Execute scenario
  example-config                Show example config
  example-scenario              Show example scenario`)
}

func cmdCheck(config *models.SystemConfig) {
	fmt.Println("=== Проверка модемов ===")
	for _, cfg := range config.Modems {
		fmt.Printf("'%s' (%s) on %s ... ", cfg.Name, cfg.Model, cfg.ATPort)
		ctrl, err := modem.New(cfg.ATPort, cfg.BaudRate, cfg.Name, cfg.PhoneNumber, cfg.Model, cfg.Operator)
		if err != nil {
			fmt.Printf("✗ %v\n", err)
			continue
		}
		defer ctrl.Close()
		if err := ctrl.Init(); err != nil {
			fmt.Printf("✗ %v\n", err)
			continue
		}
		rssi, _, err := ctrl.GetSignalQuality()
		sig := "?"
		if err == nil {
			sig = models.SignalDBm(rssi)
		}
		fmt.Printf("✓ OK (signal: %s, operator: %s)\n", sig, cfg.Operator)
	}
	fmt.Println()
}

func cmdRun(config *models.SystemConfig, scenario *models.Scenario, outputDir string) {
	if config.Metrics.Enabled {
		addr := config.Metrics.Listen
		if addr == "" {
			addr = "0.0.0.0:9100"
		}
		metrics.Serve(addr)
	}
	reg, err := engine.NewRegistry(config)
	if err != nil {
		log.Fatalf("init: %v", err)
	}
	defer reg.Close()

	rep := engine.Execute(reg, scenario)
	report.SaveJSON(&rep, outputDir)
	report.SaveCSVCalls(&rep, outputDir)
	report.SaveCSVSMS(&rep, outputDir)
	report.SaveCSVData(&rep, outputDir)
	report.PrintSummary(&rep)
}

func mustLoadConfig(path string) *models.SystemConfig {
	d, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	var c models.SystemConfig
	if err := yaml.Unmarshal(d, &c); err != nil {
		log.Fatalf("config parse: %v", err)
	}
	return &c
}

func mustLoadScenario(path string) *models.Scenario {
	d, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("scenario: %v", err)
	}
	var s models.Scenario
	if err := yaml.Unmarshal(d, &s); err != nil {
		log.Fatalf("scenario parse: %v", err)
	}
	return &s
}
