package main

import (
	"log"
	"os"

	"ectorus/internal/ecscan"
)

func main() {
	cfg, err := ecscan.ParseFlags(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	if err := ecscan.Run(cfg); err != nil {
		log.Fatal(err)
	}
}
