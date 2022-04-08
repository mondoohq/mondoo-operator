package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"go.uber.org/zap"
)

// CommandArgs is a warpper for cmd args
type CommandArgs struct {
	Command             string
	CmdArgs             []string
	PipeToStdIn         string
	EnvironmentVariable []string
}

// CommandOut is a wrapper for cmd out returned after executing command args
type CommandOut struct {
	StdOut   string
	StdErr   string
	ExitCode int
	Err      error
}

// ExecuteCommand executes a os command with stdin and returns output
func ExecuteCommand(cmdStruct CommandArgs) CommandOut {
	zap.S().Infof("Running %s %v", cmdStruct.Command, cmdStruct.CmdArgs)

	var outBuffer, errBuffer bytes.Buffer

	cmd := exec.Command(cmdStruct.Command, cmdStruct.CmdArgs...) //nolint:gosec // We safely suppress gosec in tests file

	cmd.Env = append(cmd.Env, cmdStruct.EnvironmentVariable...)

	stdOut, err := cmd.StdoutPipe()

	if err != nil {
		return CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), Err: err}
	}

	stdin, err := cmd.StdinPipe()

	if err != nil {
		return CommandOut{Err: err}
	}

	defer stdOut.Close()

	scanner := bufio.NewScanner(stdOut)
	go func() {
		for scanner.Scan() {
			outBuffer.WriteString(scanner.Text())
			fmt.Println(scanner.Text())
		}
	}()

	stdErr, err := cmd.StderrPipe()

	if err != nil {
		return CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), Err: err}
	}

	defer stdErr.Close()

	stdErrScanner := bufio.NewScanner(stdErr)
	go func() {
		for stdErrScanner.Scan() {

			txt := stdErrScanner.Text()

			if !strings.Contains(txt, "no buildable Go source files in") {
				errBuffer.WriteString(txt)
				fmt.Println(txt)
			}
		}
	}()

	err = cmd.Start()
	if err != nil {
		return CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), Err: err}
	}

	if cmdStruct.PipeToStdIn != "" {
		_, err = stdin.Write([]byte(cmdStruct.PipeToStdIn))
		if err != nil {
			return CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), Err: err}
		}
		stdin.Close()
	}

	err = cmd.Wait()
	out := CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String()}
	if err != nil {
		out.Err = err
		if code, ok := ExitStatus(err); ok {
			out.ExitCode = code
		}
	}

	return out
}

func ExitStatus(err error) (int, bool) {
	exitErr, ok := err.(*exec.ExitError)
	if ok {
		waitStatus, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus)
		if ok {
			return waitStatus.ExitStatus(), true
		}
	}
	return 0, false
}
