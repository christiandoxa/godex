package quota

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type redeemRunner interface {
	Prepare(context.Context, quotamodel.RedeemInput) (quotamodel.RedeemPlan, error)
	Execute(context.Context, quotamodel.RedeemPlan) (quotamodel.RedeemResult, error)
}

type redeemOptions struct {
	profile string
	yes     bool
	baseURL string
	noProxy bool
}

func Redeem(ctx context.Context, runner redeemRunner, out io.Writer, arguments []string) error {
	return redeemWithIO(ctx, runner, out, os.Stdin, os.Stderr, terminalFile(os.Stdin) && terminalFile(os.Stderr), arguments)
}

func redeemWithIO(
	ctx context.Context,
	runner redeemRunner,
	out io.Writer,
	in io.Reader,
	errOut io.Writer,
	interactive bool,
	arguments []string,
) error {
	if runner == nil {
		return errors.New("redeem support is not configured")
	}
	options, err := parseRedeemArguments(arguments)
	if err != nil {
		return err
	}
	plan, err := runner.Prepare(ctx, quotamodel.RedeemInput{Profile: options.profile, BaseURL: options.baseURL, NoProxy: options.noProxy})
	if err != nil {
		return err
	}
	if plan.NearReset != nil && !options.yes {
		confirmed, err := confirmRedeem(ctx, in, errOut, interactive, plan.Profile, *plan.NearReset)
		if err != nil {
			return err
		}
		if !confirmed {
			return errors.New("redeem cancelled")
		}
	}
	result, err := runner.Execute(ctx, plan)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "profile=%s outcome=%s request_id=%s\n", result.Profile, result.Outcome, result.RequestID)
	return err
}

func parseRedeemArguments(arguments []string) (redeemOptions, error) {
	options := redeemOptions{}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "-y" || argument == "--yes":
			options.yes = true
		case argument == "--no-proxy":
			options.noProxy = true
		case argument == "--base-url" || strings.HasPrefix(argument, "--base-url="):
			value, next, err := quotaOptionValue(arguments, index, argument, "--base-url")
			if err != nil {
				return redeemOptions{}, err
			}
			options.baseURL, index = value, next
		case argument == "--help" || argument == "-h":
			return redeemOptions{}, errors.New("usage: godex redeem PROFILE [-y|--yes] [--base-url URL] [--no-proxy]")
		case strings.HasPrefix(argument, "-"):
			return redeemOptions{}, fmt.Errorf("unknown redeem option %q", argument)
		case options.profile == "":
			options.profile = argument
		default:
			return redeemOptions{}, errors.New("redeem accepts exactly one profile name")
		}
	}
	if options.profile == "" {
		return redeemOptions{}, errors.New("usage: godex redeem PROFILE [-y|--yes] [--base-url URL] [--no-proxy]")
	}
	return options, nil
}

func confirmRedeem(ctx context.Context, in io.Reader, errOut io.Writer, interactive bool, profile string, near quotamodel.NearReset) (bool, error) {
	resetTime := time.Unix(near.ResetAt, 0).UTC().Format(time.RFC3339)
	if !interactive {
		return false, fmt.Errorf("profile %q has a %s reset near at %s; rerun in a terminal to confirm or pass --yes", profile, near.Label, resetTime)
	}
	if input, output, ok := redeemTerminalIO(in, errOut); ok {
		return runRedeemPromptTUI(ctx, input, output, profile, near.Label, resetTime)
	}
	if _, err := fmt.Fprintf(errOut, "Profile %q has a %s reset near at %s.\n", profile, near.Label, resetTime); err != nil {
		return false, err
	}
	reader := bufio.NewReader(in)
	for {
		if _, err := fmt.Fprint(errOut, "Redeem one reset credit anyway? [y/N]: "); err != nil {
			return false, err
		}
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, errors.New("failed to read redeem confirmation")
		}
		confirmed, valid := parseRedeemConfirmation(line)
		if valid {
			return confirmed, nil
		}
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if _, err := fmt.Fprintln(errOut, "Please answer yes or no."); err != nil {
			return false, err
		}
	}
}

func parseRedeemConfirmation(input string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "", "n", "no":
		return false, true
	case "y", "yes":
		return true, true
	default:
		return false, false
	}
}

func terminalFile(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
