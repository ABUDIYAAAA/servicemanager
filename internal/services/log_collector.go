package services

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"

	"servicemanager/internal/models"
)

type LogCollector struct {
	cli         *client.Client
	ctx         context.Context
	cancel      context.CancelFunc
	activeTails map[string]context.CancelFunc
	mu          sync.Mutex
}

func NewLogCollector() (*LogCollector, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &LogCollector{
		cli:         cli,
		ctx:         ctx,
		cancel:      cancel,
		activeTails: make(map[string]context.CancelFunc),
	}, nil
}

func (lc *LogCollector) Start() {
	slog.Info("Starting native Docker Log Collector...")

	// 1. Discover existing managed containers
	containers, err := lc.cli.ContainerList(lc.ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("label", "seed.managed=true")),
	})
	if err == nil {
		for _, c := range containers {
			lc.startTailing(c.ID, c.Labels)
		}
	} else {
		slog.Error("Failed to list existing containers", slog.Any("error", err))
	}

	// 2. Listen to Docker events for start/die
	eventFilter := filters.NewArgs(
		filters.Arg("type", "container"),
		filters.Arg("event", "start"),
		filters.Arg("event", "die"),
		filters.Arg("label", "seed.managed=true"),
	)
	msgCh, errCh := lc.cli.Events(lc.ctx, events.ListOptions{Filters: eventFilter})

	go func() {
		for {
			select {
			case <-lc.ctx.Done():
				slog.Info("Docker Log Collector stopping...")
				return
			case err := <-errCh:
				if err != nil && err != context.Canceled {
					slog.Error("Docker event stream error", slog.Any("error", err))
					// In production, we'd add reconnect logic here.
					time.Sleep(2 * time.Second)
					msgCh, errCh = lc.cli.Events(lc.ctx, events.ListOptions{Filters: eventFilter})
				}
			case msg := <-msgCh:
				if msg.Action == "start" {
					containerID := msg.Actor.ID
					labels := msg.Actor.Attributes
					lc.startTailing(containerID, labels)
				} else if msg.Action == "die" {
					lc.stopTailing(msg.Actor.ID)
				}
			}
		}
	}()
}

func (lc *LogCollector) Stop() {
	lc.cancel()
}

func (lc *LogCollector) startTailing(containerID string, labels map[string]string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if _, exists := lc.activeTails[containerID]; exists {
		return // Already tailing
	}

	deploymentIDStr := labels["seed.deployment_id"]
	serviceIDStr := labels["seed.service_id"]
	deploymentID, _ := strconv.Atoi(deploymentIDStr)
	serviceID, _ := strconv.Atoi(serviceIDStr)

	if deploymentID == 0 {
		return // Not a valid managed deployment
	}

	ctx, cancel := context.WithCancel(lc.ctx)
	lc.activeTails[containerID] = cancel

	go func() {
		slog.Info("Tailing logs for container", slog.String("container", containerID), slog.Int("deployment_id", deploymentID))
		defer lc.stopTailing(containerID)

		reader, err := lc.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
			Tail:       "0", // Only get new logs, or use "all" to catch up since start. Let's use "0" since we catch starts live.
		})
		if err != nil {
			slog.Error("Failed to get container logs", slog.Any("error", err), slog.String("container", containerID))
			return
		}
		defer reader.Close()

		// Docker multiplexed stream parser
		// 8 byte header: [STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4]
		hdr := make([]byte, 8)
		for {
			_, err := io.ReadFull(reader, hdr)
			if err != nil {
				if err != io.EOF && err != context.Canceled {
					slog.Debug("Container log stream ended with error", slog.Any("error", err))
				}
				return
			}

			streamType := hdr[0]
			size := binary.BigEndian.Uint32(hdr[4:8])

			payload := make([]byte, size)
			_, err = io.ReadFull(reader, payload)
			if err != nil {
				return
			}

			streamName := "stdout"
			if streamType == 2 {
				streamName = "stderr"
			}

			msg := string(payload)
			// Remove trailing newlines
			msg = strings.TrimSuffix(msg, "\n")
			msg = strings.TrimSuffix(msg, "\r")

			LogPipeline <- models.LogEvent{
				ServiceID:    serviceID,
				DeploymentID: deploymentID,
				ContainerID:  containerID,
				Timestamp:    time.Now().UTC(),
				Stream:       streamName,
				Message:      msg,
			}
		}
	}()
}

func (lc *LogCollector) stopTailing(containerID string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if cancel, exists := lc.activeTails[containerID]; exists {
		cancel()
		delete(lc.activeTails, containerID)
		slog.Info("Stopped tailing container logs", slog.String("container", containerID))
	}
}
