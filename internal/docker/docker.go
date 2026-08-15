package docker

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// FreePort finds a free TCP port above the given minimum value.
func FreePort(above int) (int, error) {
	for port := above + 1; port < 65535; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			continue
		}
		ln.Close()
		return port, nil
	}
	return 0, fmt.Errorf("no free port found above %d", above)
}

// BuildImage runs `docker build` for the given context directory, streaming output to logFn.
func BuildImage(ctx context.Context, imageTag, contextDir string, logFn func(string)) error {
	cmd := exec.CommandContext(ctx, "docker", "build", "-t", imageTag, contextDir)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start docker build: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		logFn(line)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}

	if err = cmd.Wait(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	return nil
}

// StopAndRemoveContainer stops and removes a Docker container by name (idempotent).
func StopAndRemoveContainer(name string) error {
	// Try stop — ignore error if container doesn't exist / isn't running
	stopCmd := exec.Command("docker", "stop", name)
	_ = stopCmd.Run()

	// Try remove — ignore error if already removed
	rmCmd := exec.Command("docker", "rm", "-f", name)
	_ = rmCmd.Run()

	return nil
}

// RunContainer starts a detached Docker container with port mapping and environment variables, and optional labels.
func RunContainer(imageTag, containerName string, hostPort, containerPort int, envVars map[string]string, labels map[string]string) error {
	args := []string{
		"run", "-d",
		"--name", containerName,
		"--restart", "unless-stopped",
		"-p", fmt.Sprintf("%d:%d", hostPort, containerPort),
	}

	for k, v := range envVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	for k, v := range labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, imageTag)

	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run failed: %w — output: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ContainerExists checks whether a Docker container with the given name exists.
func ContainerExists(name string) (bool, error) {
	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", name)
	out, err := cmd.Output()
	if err != nil {
		return false, nil // container doesn't exist
	}
	return strings.TrimSpace(string(out)) != "", nil
}
