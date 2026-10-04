package main

import (
	"fmt"
	"os"

	"github.com/cli/go-gh/v2/pkg/api"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	client, err := api.DefaultRESTClient()
	if err != nil {
		return err
	}
	var user struct{ Login string }
	if err := client.Get("user", &user); err != nil {
		return err
	}
	fmt.Printf("gh argus, authenticated as %s\n", user.Login)
	return nil
}
