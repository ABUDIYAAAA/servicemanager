package docker

import "fmt"

// GenerateDockerfile returns a Dockerfile string tuned for the given framework.
// buildCmd and runCmd are the user-provided commands. rootDir is the app subdirectory (relative).
// port is the container port the app listens on.
func GenerateDockerfile(framework, buildCmd, runCmd, rootDir string, port int) string {
	if rootDir == "" || rootDir == "." {
		rootDir = "."
	}

	switch framework {
	case "Node.js":
		return generateNodeDockerfile(buildCmd, runCmd, rootDir, port)
	case "Go":
		return generateGoDockerfile(buildCmd, runCmd, rootDir, port)
	case "Python":
		return generatePythonDockerfile(buildCmd, runCmd, rootDir, port)
	default:
		return generateGenericDockerfile(buildCmd, runCmd, rootDir, port)
	}
}

func generateNodeDockerfile(buildCmd, runCmd, rootDir string, port int) string {
	copyCmd := "COPY . ."
	cdCmd := ""
	if rootDir != "." {
		cdCmd = fmt.Sprintf("WORKDIR /app/%s\n", rootDir)
		copyCmd = fmt.Sprintf("COPY . /app/\nWORKDIR /app/%s", rootDir)
	}

	build := ""
	if buildCmd != "" {
		build = fmt.Sprintf("RUN %s\n", buildCmd)
	}

	run := "node index.js"
	if runCmd != "" {
		run = runCmd
	}

	return fmt.Sprintf(`FROM node:20-alpine AS base
WORKDIR /app
%s
COPY package*.json ./
RUN npm ci 2>/dev/null || npm install
%s
%sEXPOSE %d
CMD ["sh", "-c", "%s"]
`, cdCmd, copyCmd, build, port, run)
}

func generateGoDockerfile(buildCmd, runCmd, rootDir string, port int) string {
	workdir := "/app"
	if rootDir != "." {
		workdir = fmt.Sprintf("/app/%s", rootDir)
	}

	compileTo := "/app/bin/service"
	build := fmt.Sprintf("RUN go build -o %s ./...", compileTo)
	if buildCmd != "" {
		build = fmt.Sprintf("RUN %s", buildCmd)
	}

	return fmt.Sprintf(`FROM golang:1.23-alpine AS builder
WORKDIR %s
COPY . /app/
RUN go mod download 2>/dev/null || true
%s

FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder %s /app/service
EXPOSE %d
CMD ["/app/service"]
`, workdir, build, compileTo, port)
}

func generatePythonDockerfile(buildCmd, runCmd, rootDir string, port int) string {
	workdir := "/app"
	if rootDir != "." {
		workdir = fmt.Sprintf("/app/%s", rootDir)
	}

	build := ""
	if buildCmd != "" {
		build = fmt.Sprintf("RUN %s\n", buildCmd)
	}

	run := "python app.py"
	if runCmd != "" {
		run = runCmd
	}

	return fmt.Sprintf(`FROM python:3.12-slim
WORKDIR /app
COPY . /app/
WORKDIR %s
RUN pip install --no-cache-dir -r requirements.txt 2>/dev/null || true
%sEXPOSE %d
CMD ["sh", "-c", "%s"]
`, workdir, build, port, run)
}

func generateGenericDockerfile(buildCmd, runCmd, rootDir string, port int) string {
	workdir := "/app"
	if rootDir != "." {
		workdir = fmt.Sprintf("/app/%s", rootDir)
	}

	build := ""
	if buildCmd != "" {
		build = fmt.Sprintf("RUN %s\n", buildCmd)
	}

	run := "echo 'No run command provided'"
	if runCmd != "" {
		run = runCmd
	}

	return fmt.Sprintf(`FROM ubuntu:22.04
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y curl wget git ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY . /app/
WORKDIR %s
%sEXPOSE %d
CMD ["sh", "-c", "%s"]
`, workdir, build, port, run)
}
