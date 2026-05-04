package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

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
	_ = fs.Parse(reorderFlags(args, map[string]bool{
		"--tunnel": true, "-tunnel": true,
		"--hint": true, "-hint": true,
		"--token": true, "-token": true,
	}))

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

// reorderFlags lets users put flags after positional args (matches ngrok UX).
// stdlib flag.Parse stops at the first non-flag arg, so `lrok http 3000 --hint x`
// would otherwise drop `--hint x`. We move all flag tokens before positionals.
func reorderFlags(args []string, flagsWithValue map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			flags = append(flags, args[i:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			pos = append(pos, a)
			continue
		}
		flags = append(flags, a)
		name := strings.SplitN(a, "=", 2)[0]
		if !strings.Contains(a, "=") && flagsWithValue[name] && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, pos...)
}
