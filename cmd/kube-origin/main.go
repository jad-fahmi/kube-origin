package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jad-fahmi/kube-origin/internal/origin"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "kube-origin:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("kube-origin", flag.ContinueOnError)
	flags.SetOutput(stderr)
	contextName := flags.String("context", "", "context to explain (defaults to current-context)")
	kubeconfig := flags.String("kubeconfig", "", "path to a kubeconfig file (defaults to KUBECONFIG or ~/.kube/config)")
	jsonOutput := flags.Bool("json", false, "write machine-readable JSON")
	showVersion := flags.Bool("version", false, "print the version and exit")
	if err := flags.Parse(args); errors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments; see --help")
	}
	if *showVersion {
		_, err := fmt.Fprintf(stdout, "kube-origin %s\n", version)
		return err
	}

	report, err := origin.Inspect(origin.Options{
		Context:    *contextName,
		Kubeconfig: *kubeconfig,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return origin.WriteJSON(stdout, report)
	}
	return origin.WriteText(stdout, report)
}
