package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

func runCountBasedUpdate(
	ctx context.Context,
	opts UpdateOptions,
	checkResult *CheckResult,
	noUpdatesMessage string,
	dryRunMessageFn func(count int) string,
	command string,
	args []string,
	runErrFormat string,
	successMessageFn func(count int) string,
) (*UpdateResult, error) {
	result := &UpdateResult{}

	if checkResult.AvailableUpdates == 0 {
		result.Message = noUpdatesMessage
		return result, nil
	}

	if opts.DryRun {
		result.Message = dryRunMessageFn(checkResult.AvailableUpdates)
		result.Packages = checkResult.Packages

		return result, nil
	}

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		result.Errors = append(result.Errors, err)
		return result, fmt.Errorf(runErrFormat, err)
	}

	result.UpdatedCount = checkResult.AvailableUpdates
	result.Packages = checkResult.Packages
	result.Message = successMessageFn(result.UpdatedCount)

	return result, nil
}
