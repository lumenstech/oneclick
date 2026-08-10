package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/lumenstech/oneclick/internal/analyzer"
	"github.com/lumenstech/oneclick/internal/deploy"
	"github.com/lumenstech/oneclick/internal/hardware"
	"github.com/lumenstech/oneclick/internal/source"
)

const version = "0.1.0"

type planIdentityFlags struct {
	SourceRef        string
	SourceType       string
	SourceRevision   string
	ExpectedPlanHash string
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "analyze":
		analyzeCmd(os.Args[2:])
	case "preflight":
		preflightCmd(os.Args[2:])
	case "deploy":
		deployCmd(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
}

func analyzeCmd(args []string) {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	format := fs.String("format", "yaml", "yaml or json")
	identity := addPlanIdentityFlags(fs)
	src, parseArgs := splitLeadingSource(args)
	_ = fs.Parse(parseArgs)
	if fs.NArg() > 0 {
		src = fs.Arg(0)
	}
	c, plan := preparePlan(src, identity)
	defer c.Close()
	switch strings.ToLower(*format) {
	case "json":
		printJSON(plan)
	case "yaml", "yml":
		printPlanYAML(plan)
	default:
		fatal(fmt.Errorf("unknown format: %s", *format))
	}
}

func preflightCmd(args []string) {
	fs := flag.NewFlagSet("preflight", flag.ExitOnError)
	identity := addPlanIdentityFlags(fs)
	src, parseArgs := splitLeadingSource(args)
	_ = fs.Parse(parseArgs)
	if fs.NArg() > 0 {
		src = fs.Arg(0)
	}
	c, plan := preparePlan(src, identity)
	defer c.Close()
	result := hardware.Check(plan, c.Path)
	printJSON(result)
	if !result.Fits {
		os.Exit(3)
	}
}

func deployCmd(args []string) {
	fs := flag.NewFlagSet("deploy", flag.ExitOnError)
	yes := fs.Bool("yes", false, "confirm deployment")
	port := fs.Int("port", 0, "host port for Dockerfile executor; maps to the sole detected container port when available")
	identity := addPlanIdentityFlags(fs)
	src, parseArgs := splitLeadingSource(args)
	_ = fs.Parse(parseArgs)
	if fs.NArg() > 0 {
		src = fs.Arg(0)
	}
	c, plan := preparePlan(src, identity)
	defer c.Close()
	if c.Dirty {
		fatal(fmt.Errorf("refusing to deploy a dirty local Git working tree; commit or stash changes before deployment"))
	}
	pre := hardware.Check(plan, c.Path)
	if !pre.Fits {
		printJSON(pre)
		fatal(fmt.Errorf("hardware preflight failed"))
	}
	if err := deploy.Run(plan, c.Path, deploy.Options{Confirm: *yes, Port: *port}); err != nil {
		fatal(err)
	}
}

func addPlanIdentityFlags(fs *flag.FlagSet) *planIdentityFlags {
	out := &planIdentityFlags{}
	fs.StringVar(&out.SourceRef, "source-ref", "", "canonical logical source reference used for plan hashing")
	fs.StringVar(&out.SourceType, "source-type", "", "logical source type override: github or local")
	fs.StringVar(&out.SourceRevision, "source-revision", "", "immutable logical source revision used for plan hashing")
	fs.StringVar(&out.ExpectedPlanHash, "expected-plan-hash", "", "refuse if the analyzed plan does not match this 64-character hash")
	return out
}

func preparePlan(src string, identity *planIdentityFlags) (source.Checkout, analyzer.Plan) {
	c, err := source.Prepare(src)
	if err != nil {
		fatal(err)
	}
	ref, sourceType, revision := c.Ref, c.Type, c.Revision
	if identity.SourceRef != "" {
		ref = identity.SourceRef
	}
	if identity.SourceType != "" {
		sourceType = strings.ToLower(identity.SourceType)
		if sourceType != "github" && sourceType != "local" {
			c.Close()
			fatal(fmt.Errorf("source-type must be github or local"))
		}
	}
	if identity.SourceRevision != "" {
		revision = identity.SourceRevision
	}
	plan, err := analyzer.Analyze(c.Path, ref, sourceType, revision, c.Dirty)
	if err != nil {
		c.Close()
		fatal(err)
	}
	plan.PlanHash = analyzer.StablePlanHash(plan)
	if identity.ExpectedPlanHash != "" {
		if !isHexLen(identity.ExpectedPlanHash, 64) {
			c.Close()
			fatal(fmt.Errorf("expected-plan-hash must be 64 hexadecimal characters"))
		}
		if !strings.EqualFold(plan.PlanHash, identity.ExpectedPlanHash) {
			c.Close()
			fatal(fmt.Errorf("plan hash mismatch: approved %s, analyzed %s", identity.ExpectedPlanHash, plan.PlanHash))
		}
	}
	return c, plan
}

func splitLeadingSource(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return ".", args
}

func isHexLen(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') && !(r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fatal(err)
	}
}

func printPlanYAML(p analyzer.Plan) {
	fmt.Printf("version: %d\n", p.Version)
	fmt.Printf("plan_hash: %s\n", q(p.PlanHash))
	fmt.Printf("app:\n  name: %s\n", q(p.App.Name))
	fmt.Printf("source:\n  type: %s\n  ref: %s\n", q(p.Source.Type), q(p.Source.Ref))
	if p.Source.Revision != "" {
		fmt.Printf("  revision: %s\n", q(p.Source.Revision))
	}
	if p.Source.Dirty {
		fmt.Println("  dirty: true")
	}
	fmt.Printf("runtime:\n  primary: %s\n  executor: %s\n  deployable: %t\n  detected:\n", q(p.Runtime.Primary), q(p.Runtime.Executor), p.Runtime.Deployable)
	for _, s := range p.Runtime.Detected {
		fmt.Printf("    - %s\n", q(s))
	}
	fmt.Printf("requirements:\n  cpu_cores_min: %d\n  memory_mb_min: %d\n  disk_mb_min: %d\n  gpu_required: %t\n", p.Requirements.CPUCoresMin, p.Requirements.MemoryMBMin, p.Requirements.DiskMBMin, p.Requirements.GPURequired)
	fmt.Println("ports:")
	for _, n := range p.Ports {
		fmt.Printf("  - %d\n", n)
	}
	fmt.Println("secrets:")
	for _, s := range p.Secrets {
		fmt.Printf("  - %s\n", q(s))
	}
	fmt.Printf("deployment:\n  rollback_supported: %t\n", p.Deployment.RollbackSupported)
	if len(p.Warnings) > 0 {
		fmt.Println("warnings:")
		for _, s := range p.Warnings {
			fmt.Printf("  - %s\n", q(s))
		}
	}
}

func q(s string) string { b, _ := json.Marshal(s); return string(b) }
func fatal(err error)   { fmt.Fprintln(os.Stderr, "oneclick:", err); os.Exit(1) }
func usage() {
	cmds := []string{
		"analyze <path|github-url> [--format=yaml|json] [source identity flags]",
		"preflight <path|github-url> [--expected-plan-hash=...] [source identity flags]",
		"deploy <path|github-url> --yes [--port=3000] [--expected-plan-hash=...] [source identity flags]",
		"version",
	}
	sort.Strings(cmds)
	fmt.Fprintln(os.Stderr, "OneClick v"+version)
	fmt.Fprintln(os.Stderr)
	for _, c := range cmds {
		fmt.Fprintln(os.Stderr, "  oneclick "+c)
	}
}
