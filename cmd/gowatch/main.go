package main

import (
	"context"
	"fmt"
	"time"

	"github.com/moby/moby/client"
)

func main() {
	// Colher informações do container
	// Observar alterações feitas
	// Exibir isso em TUI
	ctx := context.Background()
	apiClient, err := client.New(client.FromEnv)
	if err != nil {
		panic(err)
	}
	defer apiClient.Close()

	containers, err := apiClient.ContainerList(ctx, client.ContainerListOptions{})
	if err != nil {
		panic(err)
	}

	for _, container := range containers.Items {
		fmt.Println(container.Names)
	}
	fmt.Println("fim da listagem dos containers")
}

func loop() {
	for {
		time.Sleep(time.Second)
		fmt.Println("um segundo se passou...")
	}
}
