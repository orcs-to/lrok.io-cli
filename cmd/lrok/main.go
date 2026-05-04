package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/orcs-to/lrok.io-cli/internal/apiclient"
	"github.com/orcs-to/lrok.io-cli/internal/client"
	"github.com/orcs-to/lrok.io-cli/internal/config"
)

// version is set by the release pipeline via -ldflags "-X main.version=...".
// Defaults to "dev" for local `go build` / `go install` invocations.
var version = "dev"

const usage = `lrok - public URLs for your local server

Usage:
  lrok login [--token TOKEN]         save your API token
  lrok http <port> [--hint X]        tunnel http://localhost:<port>
  lrok reserve <name> [--desc T]     reserve a subdomain for your account
  lrok unreserve <name>              release a reserved subdomain
  lrok reservations                  list your reservations
  lrok version                       print version

Flags (lrok http):
  --tunnel ADDR   tunnel server address (default "tunnel.lrok.io:7000")
  --hint NAME     preferred subdomain (your reservation, or first-come)
  --token TOKEN   override saved token
  --insecure      disable TLS on the tunnel connection (local dev only;
                  also enabled by LROK_INSECURE=1)

Create a token at https://lrok.io/dashboard/tokens
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	switch os.Args[1] {
	case "login":
		runLogin(os.Args[2:])
	case "http":
		runHTTP(os.Args[2:])
	case "reserve":
		runReserve(os.Args[2:])
	case "unreserve":
		runUnreserve(os.Args[2:])
	case "reservations":
		runListReservations(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println(version)
	case "-h", "--help", "help":
		fmt.Fprint(os.Stdout, usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

// requireToken loads the user's API token from --token, env, or config.
// Exits with a clear message if missing.
func requireToken(tokenFlag string) string {
	token := strings.TrimSpace(tokenFlag)
	if token == "" {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error reading config:", err)
			os.Exit(1)
		}
		token = cfg.Token
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "no API token configured. Run `lrok login` (token from https://lrok.io/dashboard/tokens) or pass --token")
		os.Exit(1)
	}
	return token
}

func runReserve(args []string) {
	fs := flag.NewFlagSet("reserve", flag.ExitOnError)
	desc := fs.String("desc", "", "optional description")
	tokenFlag := fs.String("token", "", "override saved token")
	_ = fs.Parse(reorderFlags(args, map[string]bool{
		"--desc": true, "-desc": true,
		"--token": true, "-token": true,
	}))
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: lrok reserve <name> [--desc TEXT]")
		os.Exit(2)
	}
	name := strings.ToLower(strings.TrimSpace(fs.Arg(0)))

	c := apiclient.New(requireToken(*tokenFlag))
	res, err := c.CreateReservation(name, *desc)
	if err != nil {
		fmt.Fprintln(os.Stderr, "reserve failed:", err)
		os.Exit(1)
	}
	fmt.Printf("Reserved https://%s.lrok.io\n", res.Subdomain)
}

func runUnreserve(args []string) {
	fs := flag.NewFlagSet("unreserve", flag.ExitOnError)
	tokenFlag := fs.String("token", "", "override saved token")
	_ = fs.Parse(reorderFlags(args, map[string]bool{
		"--token": true, "-token": true,
	}))
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: lrok unreserve <name>")
		os.Exit(2)
	}
	name := strings.ToLower(strings.TrimSpace(fs.Arg(0)))
	c := apiclient.New(requireToken(*tokenFlag))
	if err := c.DeleteReservation(name); err != nil {
		fmt.Fprintln(os.Stderr, "unreserve failed:", err)
		os.Exit(1)
	}
	fmt.Printf("Released %s.lrok.io\n", name)
}

func runListReservations(args []string) {
	fs := flag.NewFlagSet("reservations", flag.ExitOnError)
	tokenFlag := fs.String("token", "", "override saved token")
	_ = fs.Parse(args)

	c := apiclient.New(requireToken(*tokenFlag))
	list, err := c.ListReservations()
	if err != nil {
		fmt.Fprintln(os.Stderr, "list failed:", err)
		os.Exit(1)
	}
	if len(list) == 0 {
		fmt.Println("No reservations. Reserve one with `lrok reserve <name>`.")
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SUBDOMAIN\tCREATED\tDESCRIPTION")
	for _, r := range list {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Subdomain, r.CreatedAt.Local().Format("2006-01-02 15:04"), r.Description)
	}
	_ = tw.Flush()
}

func runLogin(args []string) {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	tokenFlag := fs.String("token", "", "API token (or omit to be prompted)")
	_ = fs.Parse(args)

	token := strings.TrimSpace(*tokenFlag)
	if token == "" {
		fmt.Print("Paste your API token: ")
		r := bufio.NewReader(os.Stdin)
		line, err := r.ReadString('\n')
		if err != nil {
			fmt.Fprintln(os.Stderr, "error reading token:", err)
			os.Exit(1)
		}
		token = strings.TrimSpace(line)
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "no token provided")
		os.Exit(1)
	}

	if err := config.Save(&config.Config{Token: token}); err != nil {
		fmt.Fprintln(os.Stderr, "error saving token:", err)
		os.Exit(1)
	}

	p, _ := config.Path()
	fmt.Printf("Saved to %s\n", p)
}

func runHTTP(args []string) {
	fs := flag.NewFlagSet("http", flag.ExitOnError)
	tunnelAddr := fs.String("tunnel", "tunnel.lrok.io:7000", "tunnel server address")
	hint := fs.String("hint", "", "preferred subdomain")
	tokenFlag := fs.String("token", "", "override saved token")
	insecure := fs.Bool("insecure", false, "disable TLS on the tunnel connection (dev only)")
	_ = fs.Parse(reorderFlags(args, map[string]bool{
		"--tunnel": true, "-tunnel": true,
		"--hint": true, "-hint": true,
		"--token": true, "-token": true,
	}))

	if fs.NArg() < 1 {
		fmt.Fprint(os.Stderr, "missing port\n\n")
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	token := requireToken(*tokenFlag)

	port := fs.Arg(0)
	cfg := client.Config{
		TunnelAddr:  *tunnelAddr,
		LocalTarget: "127.0.0.1:" + port,
		Hint:        *hint,
		AuthToken:   token,
		Insecure:    *insecure,
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
