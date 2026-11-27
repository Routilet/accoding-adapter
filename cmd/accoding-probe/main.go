// accoding-probe is a development-only read-only integration probe. It is not
// the future plugin protocol. It intentionally cannot submit code.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/Routilet/accoding-adapter/accoding"
	"io"
	"os"
	"os/signal"
	"time"
)

func run() error {
	op := flag.String("op", "account", "account | contest | import | problem | history | result | prepare")
	contest := flag.String("contest", "", "contest ID")
	problem := flag.String("problem", "", "problem ID")
	service := flag.String("service", "", "empty/student or admin (problemset only)")
	submission := flag.String("submission", "", "submission ID")
	flag.Parse()
	// Avoid command-line credentials and never log raw input.
	var creds struct{ Username, Password string }
	d := json.NewDecoder(io.LimitReader(os.Stdin, 64<<10))
	if err := d.Decode(&creds); err != nil {
		return fmt.Errorf("stdin must contain one JSON credentials object")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	c := accoding.New()
	defer c.Logout()
	u, err := c.Login(ctx, creds.Username, creds.Password)
	creds.Password = ""
	if err != nil {
		return err
	}
	target := accoding.Target{Kind: "problemset", ProblemID: *problem, Service: *service}
	if *contest != "" {
		target.Kind = "contest"
		target.ContestID = *contest
	}
	var out any
	switch *op {
	case "account":
		out = u
	case "contest":
		out, err = c.GetContest(ctx, *contest)
	case "import":
		out, err = c.ImportContest(ctx, *contest)
	case "problem":
		out, err = c.GetProblem(ctx, target)
	case "history":
		out, err = c.ListSubmissions(ctx, target)
	case "result":
		out, err = c.GetSubmission(ctx, target, *submission)
	case "prepare":
		out, err = c.Prepare(ctx, target)
	default:
		return fmt.Errorf("unsupported operation")
	}
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
