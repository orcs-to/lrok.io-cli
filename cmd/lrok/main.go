package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/orcs-to/lrok.io-cli/internal/client"
)

const usage = `lrok - public URLs for your local server

Usage:
  lrok http <port>              expose http://localhost:<port>
  lrok http <port> --hint NAME  request NAME.lrok.io (best effort)

Flags:
  --tunnel ADDR   tunnel server address (default "tunnel.lrok.io:7000")
  --hint NAME     preferred subdomain
  --token TOKEN   auth token (ignored in milestone 1)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	switch os.Args[1] {
	case "http":
		runHTTP(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Fprint(os.Stdout, usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

func runHTTP(args []string) {
	fs := flag.NewFlagSet("http", flag.ExitOnError)
	tunnelAddr := fs.String("tunnel", "tunnel.lrok.io:7000", "tunnel server address")
	hint := fs.String("hint", "", "preferred subdomain")
	token := fs.String("token", "", "auth token")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "missing port\n")
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	port := fs.Arg(0)

	cfg := client.Config{
		TunnelAddr:  *tunnelAddr,
		LocalTarget: "127.0.0.1:" + port,
		Hint:        *hint,
		AuthToken:   *token,
	}

	if err := client.Run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
