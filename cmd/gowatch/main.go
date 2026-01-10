package main

import (
	"context"
	"log"
	"time"

	"github.com/b92c/gowatch/internal/docker"
	"github.com/b92c/gowatch/internal/ui"
	"github.com/moby/moby/client"
)

func main() {
	ctx := context.Background()
	apiClient, err := client.New(client.FromEnv)
	if err != nil {
		log.Fatalf("Failed to create Docker client: %v", err)
	}
	defer apiClient.Close()

	dashboard := ui.NewDashboard()

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				containers, err := docker.WatchContainers(ctx, apiClient)
				if err != nil {
					log.Printf("Error watching containers: %v", err)
					continue
				}
				dashboard.Update(containers)
			case <-ctx.Done():
				return
			}
		}
	}()

	if err := dashboard.Run(); err != nil {
		log.Fatalf("Error running dashboard: %v", err)
	}
}
