package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/orcs-to/lrok.io-cli/internal/apiclient"
)

// runDomain dispatches the `lrok domain ...` subcommands.
func runDomain(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stderr, domainUsage)
		if len(args) == 0 {
			os.Exit(2)
		}
		return
	}
	switch args[0] {
	case "add":
		runDomainAdd(args[1:])
	case "verify":
		runDomainVerify(args[1:])
	case "remove", "rm":
		runDomainRemove(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown domain subcommand: %s\n\n%s", args[0], domainUsage)
		os.Exit(2)
	}
}

const domainUsage = `usage:
  lrok domain add <host> --target <subdomain>   register a custom domain
  lrok domain verify <host>                     check the TXT record + activate
  lrok domain remove <host>                     unregister a custom domain
  lrok domains                                  list your custom domains
`

func runDomainAdd(args []string) {
	fs := flag.NewFlagSet("domain add", flag.ExitOnError)
	target := fs.String("target", "", "subdomain or reservation on lrok.io to point at (required)")
	tokenFlag := fs.String("token", "", "override saved token")
	_ = fs.Parse(reorderFlags(args, map[string]bool{
		"--target": true, "-target": true,
		"--token": true, "-token": true,
	}))
	if fs.NArg() < 1 || strings.TrimSpace(*target) == "" {
		fmt.Fprintln(os.Stderr, "usage: lrok domain add <host> --target <subdomain>")
		os.Exit(2)
	}
	host := strings.ToLower(strings.TrimSpace(fs.Arg(0)))

	c := apiclient.New(requireToken(*tokenFlag))
	c.BaseURL = apiBaseURL()
	cd, err := c.CreateDomain(host, *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "domain add failed:", err)
		os.Exit(1)
	}
	fmt.Printf("Registered %s (target: %s)\n", cd.Host, cd.Target)
	fmt.Println()
	fmt.Println("Verification — complete BOTH steps, then run:")
	fmt.Printf("  lrok domain verify %s\n", cd.Host)
	fmt.Println()
	fmt.Println("1. TXT record")
	fmt.Printf("     name : _lrok-verify.%s\n", cd.Host)
	fmt.Printf("     value: %s\n", cd.VerifyToken)
	fmt.Println()
	fmt.Println("2. CNAME record")
	fmt.Printf("     name : %s\n", cd.Host)
	fmt.Printf("     value: lrok.io\n")
}

func runDomainVerify(args []string) {
	fs := flag.NewFlagSet("domain verify", flag.ExitOnError)
	tokenFlag := fs.String("token", "", "override saved token")
	_ = fs.Parse(reorderFlags(args, map[string]bool{
		"--token": true, "-token": true,
	}))
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: lrok domain verify <host>")
		os.Exit(2)
	}
	host := strings.ToLower(strings.TrimSpace(fs.Arg(0)))

	c := apiclient.New(requireToken(*tokenFlag))
	c.BaseURL = apiBaseURL()
	res, err := c.VerifyDomain(host)
	if err != nil {
		fmt.Fprintln(os.Stderr, "verify failed:", err)
		os.Exit(1)
	}
	if !res.Verified {
		// Treat as a soft failure: not exit 1 noisy stderr, since DNS
		// propagation is often the real cause and re-running succeeds.
		fmt.Printf("not verified yet: %s\n", res.Error)
		os.Exit(1)
	}
	fmt.Printf("Verified %s — Traefik will route within ~30 seconds.\n", host)
}

func runDomainRemove(args []string) {
	fs := flag.NewFlagSet("domain remove", flag.ExitOnError)
	tokenFlag := fs.String("token", "", "override saved token")
	_ = fs.Parse(reorderFlags(args, map[string]bool{
		"--token": true, "-token": true,
	}))
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: lrok domain remove <host>")
		os.Exit(2)
	}
	host := strings.ToLower(strings.TrimSpace(fs.Arg(0)))

	c := apiclient.New(requireToken(*tokenFlag))
	c.BaseURL = apiBaseURL()
	if err := c.DeleteDomain(host); err != nil {
		fmt.Fprintln(os.Stderr, "domain remove failed:", err)
		os.Exit(1)
	}
	fmt.Printf("Removed %s\n", host)
}

func runListDomains(args []string) {
	fs := flag.NewFlagSet("domains", flag.ExitOnError)
	tokenFlag := fs.String("token", "", "override saved token")
	_ = fs.Parse(args)

	c := apiclient.New(requireToken(*tokenFlag))
	c.BaseURL = apiBaseURL()
	list, err := c.ListDomains()
	if err != nil {
		fmt.Fprintln(os.Stderr, "list failed:", err)
		os.Exit(1)
	}
	if len(list) == 0 {
		fmt.Println("No custom domains. Add one with `lrok domain add <host> --target <subdomain>`.")
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "HOST\tTARGET\tVERIFIED\tAGE")
	now := time.Now()
	for _, d := range list {
		state := "pending"
		if d.Verified {
			state = "yes"
		}
		age := "—"
		if !d.CreatedAt.IsZero() {
			age = humanDuration(now.Sub(d.CreatedAt))
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", d.Host, d.Target, state, age)
	}
	_ = tw.Flush()
}
