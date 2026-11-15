package main

import (
	"context"
	"log"
	"time"

	"github.com/goliatone/go-router"
)

func serve(ctx context.Context, server router.ManagedServer, addr string, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	managed, err := server.StartManaged(context.WithoutCancel(ctx), router.ManagedOptions{
		Address: addr, Network: "tcp4", DrainTimeout: timeout,
	})
	if err != nil {
		return err
	}
	serving := managed.Serving()
	for {
		select {
		case <-serving:
			log.Printf("RPC web debug serving at http://%s", managed.Address())
			serving = nil
		case <-managed.Done():
			return managed.Wait(context.Background())
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			if err := managed.Stop(shutdown); err != nil {
				return err
			}
			return managed.Wait(shutdown)
		}
	}
}
