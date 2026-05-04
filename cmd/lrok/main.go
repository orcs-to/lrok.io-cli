package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/orcs-to/lrok.io-cli/internal/apiclient"
	"github.com/orcs-to/lrok.io-cli/internal/client"
	"github.com/orcs-to/lrok.io-cli/internal/config"
	versionpkg "github.com/orcs-to/lrok.io-cli/internal/version"
)

// version is set by the release pipeline via -ldflags "-X main.version=...".
// Defaults to "dev" for local `go build` / `go install` invocations.
var version = "dev"

const usage = `lrok - public URLs for your local server

Usage:
  lrok login [--token TOKEN]         save your API token
  lrok logout                        forget the saved API token
  lrok http <port> [--hint X]        tunnel http://localhost:<port>
  lrok tcp <port>                    tunnel raw TCP from localhost:<port>
  lrok reserve <name> [--desc T]     reserve a subdomain for your account
  lrok unreserve <name>              release a reserved subdomain
  lrok reservations                  list your reservations
  lrok status                        show plan + active tunnels
  lrok config show                   print saved config (token redacted)
  lrok update                        check for a newer release
  lrok version                       print version

Flags (lrok http):
  --tunnel ADDR        tunnel server address (default "tunnel.lrok.io:7000")
  --hint NAME          preferred subdomain (your reservation, or first-come)
  --token TOKEN        override saved token
  --basic-auth U:P     gate the public URL behind HTTP Basic Auth (user:pass)
  --insecure           disable TLS on the tunnel connection (local dev only;
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
	case "logout":
		runLogout(os.Args[2:])
	case "http":
		runHTTP(os.Args[2:])
	case "tcp":
		runTCP(os.Args[2:])
	case "reserve":
		runReserve(os.Args[2:])
	case "unreserve":
		runUnreserve(os.Args[2:])
	case "reservations":
		runListReservations(os.Args[2:])
	case "status":
		runStatus(os.Args[2:])
	case "config":
		runConfig(os.Args[2:])
	case "update":
		runUpdate(os.Args[2:])
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
	basicAuth := fs.String("basic-auth", "", "gate the public URL with HTTP Basic Auth (user:pass)")
	insecure := fs.Bool("insecure", false, "disable TLS on the tunnel connection (dev only)")
	_ = fs.Parse(reorderFlags(args, map[string]bool{
		"--tunnel": true, "-tunnel": true,
		"--hint": true, "-hint": true,
		"--token": true, "-token": true,
		"--basic-auth": true, "-basic-auth": true,
	}))

	if fs.NArg() < 1 {
		fmt.Fprint(os.Stderr, "missing port\n\n")
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	// Validate --basic-auth client-side so we fail fast with a clear message
	// instead of round-tripping to the edge for the same complaint.
	ba := strings.TrimSpace(*basicAuth)
	if ba != "" {
		u, p, ok := splitBasicAuthArg(ba)
		if !ok || u == "" || p == "" {
			fmt.Fprintln(os.Stderr, "--basic-auth must be 'user:pass' with non-empty user and password")
			os.Exit(2)
		}
	}

	token := requireToken(*tokenFlag)

	port := fs.Arg(0)
	cfg := client.Config{
		TunnelAddr:  *tunnelAddr,
		LocalTarget: "127.0.0.1:" + port,
		Hint:        *hint,
		AuthToken:   token,
		BasicAuth:   ba,
		Insecure:    *insecure,
	}

	if err := client.Run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// runTCP handles `lrok tcp <port>` — opens a raw TCP tunnel to the
// edge-allocated public port. Distinct from runHTTP so the two grow flags
// independently and merges with HTTP-only changes stay disjoint.
func runTCP(args []string) {
	fs := flag.NewFlagSet("tcp", flag.ExitOnError)
	tunnelAddr := fs.String("tunnel", "tunnel.lrok.io:7000", "tunnel server address")
	tokenFlag := fs.String("token", "", "override saved token")
	insecure := fs.Bool("insecure", false, "disable TLS on the tunnel connection (dev only)")
	_ = fs.Parse(reorderFlags(args, map[string]bool{
		"--tunnel": true, "-tunnel": true,
		"--token": true, "-token": true,
	}))

	if fs.NArg() < 1 {
		fmt.Fprint(os.Stderr, "missing port\n\nusage: lrok tcp <port>\n")
		os.Exit(2)
	}

	token := requireToken(*tokenFlag)
	port := fs.Arg(0)

	cfg := client.Config{
		TunnelAddr:  *tunnelAddr,
		LocalTarget: "127.0.0.1:" + port,
		AuthToken:   token,
		Mode:        "tcp",
		Insecure:    *insecure,
	}

	if err := client.Run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// splitBasicAuthArg parses the "user:pass" form. Split on the first colon
// only so passwords containing ':' are preserved (RFC 7617).
func splitBasicAuthArg(s string) (user, pass string, ok bool) {
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

// runLogout removes the saved API token from the config file.
// Idempotent: succeeds silently if no token was set.
func runLogout(args []string) {
	fs := flag.NewFlagSet("logout", flag.ExitOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "usage: lrok logout") }
	_ = fs.Parse(args)

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error reading config:", err)
		os.Exit(1)
	}
	if cfg.Token == "" {
		// No-op, but still report so users know the state.
		fmt.Println("Signed out")
		return
	}
	cfg.Token = ""
	if err := config.Save(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error saving config:", err)
		os.Exit(1)
	}
	fmt.Println("Signed out")
}

// redactToken keeps the leading "ak_" (or first 3 chars) and last 4
// characters and redacts the rest. Empty input returns "(none)".
func redactToken(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return "(none)"
	}
	// Short tokens: just show last 4 (or whole tail) with leading dots.
	if len(t) <= 8 {
		n := 4
		if len(t) < n {
			n = len(t)
		}
		return "***" + t[len(t)-n:]
	}
	prefix := t[:3]
	last4 := t[len(t)-4:]
	return prefix + "..." + last4
}

// runConfig handles `lrok config show`.
func runConfig(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(os.Stderr, "usage: lrok config show")
		if len(args) == 0 {
			os.Exit(2)
		}
		return
	}
	switch args[0] {
	case "show":
		runConfigShow(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand: %s\nusage: lrok config show\n", args[0])
		os.Exit(2)
	}
}

func runConfigShow(args []string) {
	fs := flag.NewFlagSet("config show", flag.ExitOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "usage: lrok config show") }
	_ = fs.Parse(args)

	p, err := config.Path()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error locating config:", err)
		os.Exit(1)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error reading config:", err)
		os.Exit(1)
	}
	fmt.Printf("config: %s\n", p)
	fmt.Printf("token = %s\n", redactToken(cfg.Token))
}

// runStatus prints account info, plan quotas, and active tunnels.
func runStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	tokenFlag := fs.String("token", "", "override saved token")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: lrok status [--token TOKEN]")
	}
	_ = fs.Parse(reorderFlags(args, map[string]bool{
		"--token": true, "-token": true,
	}))

	// Manual token check so we can give the friendlier "Run `lrok login`" message.
	token := strings.TrimSpace(*tokenFlag)
	if token == "" {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error reading config:", err)
			os.Exit(1)
		}
		token = cfg.Token
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "Run `lrok login` first.")
		os.Exit(1)
	}

	c := apiclient.New(token)
	c.BaseURL = apiBaseURL() // honour LROK_API for tests

	fmt.Printf("Signed in as %s\n", redactToken(token))

	plan, err := c.GetPlan()
	if err != nil {
		fmt.Fprintln(os.Stderr, "couldn't fetch plan:", err)
	} else {
		fmt.Printf("Tunnels:      %s\n", quotaString(plan.TunnelUsed, plan.TunnelQuota))
		fmt.Printf("Reservations: %s\n", quotaString(plan.ReservationUsed, plan.ReservationQuota))
	}

	tunnels, err := c.ListMyTunnels()
	if err != nil {
		fmt.Fprintln(os.Stderr, "couldn't fetch tunnels:", err)
		os.Exit(1)
	}
	fmt.Println()
	if len(tunnels) == 0 {
		fmt.Println("No active tunnels — try `lrok http 3000`")
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SUBDOMAIN\tPUBLIC URL\tAGE")
	now := time.Now()
	for _, t := range tunnels {
		age := "—"
		if !t.StartedAt.IsZero() {
			age = humanDuration(now.Sub(t.StartedAt))
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", t.Subdomain, t.PublicURL, age)
	}
	_ = tw.Flush()
}

// quotaString renders a "used / quota" pair, treating -1 as unlimited.
func quotaString(used, quota int) string {
	if quota < 0 {
		return fmt.Sprintf("%d used / unlimited", used)
	}
	return fmt.Sprintf("%d used / %d", used, quota)
}

// humanDuration formats a duration in a compact, readable way.
func humanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// apiBaseURL allows tests / power users to point the CLI at a different
// control plane without baking it into apiclient.New (which would be a
// breaking change for other agents using it). Defaults to the production URL.
func apiBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("LROK_API")); v != "" {
		return v
	}
	return apiclient.DefaultBaseURL
}

// runUpdate checks GitHub for the latest release and prints upgrade
// instructions. It does NOT replace the binary — the help text says so.
func runUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr,
			"usage: lrok update\n\n"+
				"Checks GitHub for a newer release. This command does not replace\n"+
				"the binary in place; it guides you to re-run the official installer.")
	}
	_ = fs.Parse(args)

	fmt.Printf("current version: %s\n", version)

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	tag, htmlURL, err := versionpkg.FetchLatestTag(ctx)
	if err != nil {
		fmt.Println("couldn't check latest version:", err)
		return
	}
	if tag == "" {
		fmt.Println("couldn't check latest version (rate limited or no release found)")
		return
	}

	fmt.Printf("latest release:  %s\n", tag)
	cmp := versionpkg.Compare(version, tag)

	switch {
	case version == "dev":
		fmt.Println()
		fmt.Println("You're running a dev build. To install the latest release:")
		fmt.Println("  " + versionpkg.InstallHint())
	case cmp < 0:
		fmt.Println()
		fmt.Printf("A newer version is available. Re-run the installer to upgrade:\n  %s\n",
			versionpkg.InstallHint())
		if u := versionpkg.AssetURL(tag); u != "" {
			fmt.Printf("\nOr download directly for %s/%s:\n  %s\n",
				runtime.GOOS, runtime.GOARCH, u)
		}
		if htmlURL != "" {
			fmt.Printf("\nRelease notes: %s\n", htmlURL)
		}
	default:
		fmt.Println()
		fmt.Println("You're up to date.")
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
