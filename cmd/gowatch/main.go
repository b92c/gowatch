package main

import (
	"fmt"
	"time"
)

func main() {
	// Colher informações do container
	// Observar alterações feitas
	// Exibir isso em TUI
	fmt.Println("Starting application...")

	loop()
	fmt.Println("Application started")
}

func loop() {
	for {
		time.Sleep(time.Second)
		fmt.Println("um segundo se passou...")
	}
}
