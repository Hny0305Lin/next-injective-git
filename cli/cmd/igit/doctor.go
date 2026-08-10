package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/environment"
)

func cmdDoctor(cfg config.Config, args []string) error {
	mode := environment.ModePush
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--clone":
			mode = environment.ModeClone
		case "--push":
			mode = environment.ModePush
		case "--json":
			jsonOutput = true
		default:
			return fmt.Errorf("usage: igit doctor [--clone|--push] [--json]")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report := environment.Run(ctx, cfg, mode)
	if jsonOutput {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	} else {
		printDoctorReport(report)
	}
	if !report.Healthy() {
		return fmt.Errorf("%s environment is incomplete (%d failed checks)", mode, len(report.Failures()))
	}
	return nil
}

func printDoctorReport(report environment.Report) {
	fmt.Printf("igit doctor (%s)\n\n", report.Mode)
	for _, check := range report.Checks {
		fmt.Printf("%-4s %-22s %s\n", check.Status, check.Name, check.Detail)
	}
	var fixes []environment.Check
	for _, check := range report.Checks {
		if check.Fix != "" && check.Status != environment.StatusOK && check.Status != environment.StatusSkip {
			fixes = append(fixes, check)
		}
	}
	if len(fixes) > 0 {
		fmt.Println("\nTo fix:")
		for _, check := range fixes {
			fmt.Printf("  %-22s %s\n", check.Name+":", check.Fix)
		}
	}
}
