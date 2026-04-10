package main

import (
	"dogclaw/internal/config"
	"fmt"
)

func main() {
	s := config.DefaultSettings()
	s.MaxContextLength = 54321
	
	cfg, err := config.ConfigFromSettings(s)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	fmt.Printf("Config MaxContextLength: %d\n", cfg.MaxContextLength)
	if cfg.MaxContextLength == 54321 {
		fmt.Println("SUCCESS: MaxContextLength correctly mapped to Config")
	} else {
		fmt.Println("FAILURE: MaxContextLength NOT correctly mapped")
	}
}
