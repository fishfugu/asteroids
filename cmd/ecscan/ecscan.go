package ecscan

import (
	"log"
	"os"
)

func main() {
	cfg, err := ParseFlags(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	if err := Run(cfg); err != nil {
		log.Fatal(err)
	}
}
