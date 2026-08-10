package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/lumenstech/oneclick/internal/node"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println(node.Version)
		return
	}
	cfg, err := node.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	id, enrolled, err := node.LoadOrCreateIdentity(cfg.StateDir)
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if !enrolled {
		st, err := node.Enroll(ctx, cfg, id)
		if err != nil {
			log.Fatal(err)
		}
		if err := node.SaveState(cfg.StateDir, st); err != nil {
			log.Fatal(err)
		}
		id.State = st
		log.Printf("oneclick-node enrolled as %s", st.NodeID)
	}
	svc := node.NewService(cfg, id)
	if err := svc.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
