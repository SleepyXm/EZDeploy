package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"EZDeploy/core"
)

func releasesCommand(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ezdeploy releases <project-or-repository>")
	}
	name, project, err := core.ResolveProject(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Releases for %s:\n", name)
	if len(project.Releases) == 0 {
		fmt.Println("  No successful releases recorded yet; the current revision will be preserved on the next successful deployment.")
		return nil
	}
	for index := len(project.Releases) - 1; index >= 0; index-- {
		release := project.Releases[index]
		current := ""
		if release.Revision == project.Revision {
			current = " (current)"
		}
		fmt.Printf("  %-31s %-8s %s %.12s%s\n", release.ID, release.Operation, release.DeployedAt.Local().Format("2006-01-02 15:04:05"), release.Revision, current)
	}
	return nil
}

func rollbackCommand(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: sudo ezdeploy rollback <project-or-repository> --release <release-id>")
	}
	flags := flag.NewFlagSet("rollback", flag.ContinueOnError)
	releaseID, confirmed := flags.String("release", "", "saved release ID"), flags.Bool("yes", false, "skip the interactive confirmation")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() > 0 || *releaseID == "" {
		return fmt.Errorf("usage: sudo ezdeploy rollback <project-or-repository> --release <release-id>")
	}
	if err := requireSudo("rollback"); err != nil {
		return err
	}
	name, project, err := core.ResolveProject(args[0])
	if err != nil {
		return err
	}
	var selected *core.Release
	for index := range project.Releases {
		if project.Releases[index].ID == *releaseID {
			selected = &project.Releases[index]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("release %q is not recorded for %s", *releaseID, name)
	}
	fmt.Printf("Rollback %s to %.12s: code and server configuration will be restored, but database schema and data will NOT be rolled back; that remains the project owner's responsibility.\n", name, selected.Revision)
	if !*confirmed {
		fmt.Print("Continue? [y/N]: ")
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if answer = strings.ToLower(strings.TrimSpace(answer)); answer != "y" && answer != "yes" {
			return fmt.Errorf("rollback cancelled")
		}
	}
	core.LogOperation(name, string(operationRollback), "started")
	rollback, err := core.BeginDeploymentRollback(name, project.Path)
	if err != nil {
		core.LogOperation(name, string(operationRollback), "failed")
		return err
	}
	result, err := deployOne([]string{project.RepoURL}, rollback, operationRollback, selected)
	if err != nil {
		return failDeployment(name, operationRollback, err, []*core.DeploymentRollback{rollback})
	}
	return finishDeployments([]deploymentResult{result}, []*core.DeploymentRollback{rollback}, operationRollback)
}

func logsCommand(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: sudo ezdeploy logs <project-or-repository> --source runtime|deployment [options]")
	}
	flags := flag.NewFlagSet("logs", flag.ContinueOnError)
	source, service := flags.String("source", "", "runtime or deployment"), flags.String("service", "", "service name")
	lines, follow := flags.Int("lines", 100, "line count"), flags.Bool("follow", false, "follow log output")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if err := requireSudo("logs"); err != nil {
		return err
	}
	name, project, err := core.ResolveProject(args[0])
	if err != nil {
		return err
	}
	command, err := core.LogCommand(name, project, core.LogOptions{Source: *source, Service: *service, Lines: *lines, Follow: *follow})
	if err != nil {
		return err
	}
	return command.Run()
}

func networkCommand(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ezdeploy network <project-or-repository>")
	}
	name, project, err := core.ResolveProject(args[0])
	if err != nil {
		return err
	}
	report := core.NetworkDiagnostics(project)
	fmt.Printf("Network and DNS for %s (%s):\n", name, report.Hostname)
	if !report.MetadataAvailable {
		fmt.Println("  EC2 IMDSv2 metadata is unavailable, so automatic public-IP detection cannot run. No third-party IP service was contacted.")
	} else if report.Match {
		fmt.Printf("  Healthy: DNS resolves to this instance's public IPv4 address, %s.\n", report.PublicIPv4)
	} else {
		current := "no IPv4 A record"
		if len(report.Addresses) > 0 {
			current = strings.Join(report.Addresses, ", ")
		}
		fmt.Printf("  DNS mismatch: %s currently resolves to %s. Update %s A to %s; no redeploy is required.\n", report.Hostname, current, report.Record, report.PublicIPv4)
	}
	fmt.Println("  For a stable address, associate an Elastic IP. EC2 reboot retains a public IP; stop/start normally changes it.")
	return nil
}
