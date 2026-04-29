package ilo

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"ilo-fans-controller-go/internals/config"
	"ilo-fans-controller-go/internals/console"
	"ilo-fans-controller-go/internals/models"
)

type Service interface {
	GetFans(context.Context) ([]models.Fan, error)
	SetFans(context.Context, models.SetFansRequest) ([]models.Fan, error)
}

type service struct {
	cfg        config.Config
	hub        *console.Hub
	httpClient *http.Client
	sshConfig  *ssh.ClientConfig
}

type thermalResponse struct {
	Fans []struct {
		FanName        string `json:"FanName"`
		CurrentReading int    `json:"CurrentReading"`
	} `json:"Fans"`
}

func New(cfg config.Config, hub *console.Hub) Service {
	return &service{
		cfg: cfg,
		hub: hub,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.AllowInsecureTLS},
			},
		},
		sshConfig: &ssh.ClientConfig{
			User:            cfg.ILOUsername,
			Auth:            []ssh.AuthMethod{ssh.Password(cfg.ILOPassword)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         10 * time.Second,
		},
	}
}

func (s *service) GetFans(ctx context.Context) ([]models.Fan, error) {
	if !s.cfg.HasILOConfig() {
		return nil, fmt.Errorf("iLO credentials are not configured")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s/redfish/v1/chassis/1/Thermal", s.cfg.ILOHost), nil)
	if err != nil {
		return nil, err
	}
	request.SetBasicAuth(s.cfg.ILOUsername, s.cfg.ILOPassword)

	response, err := s.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return nil, fmt.Errorf("iLO Redfish request failed with status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var thermal thermalResponse
	if err := json.NewDecoder(response.Body).Decode(&thermal); err != nil {
		return nil, err
	}

	fans := make([]models.Fan, 0, len(thermal.Fans))
	for index, fan := range thermal.Fans {
		fans = append(fans, models.Fan{
			Name:  fan.FanName,
			Speed: fan.CurrentReading,
			Index: index,
		})
	}

	sort.Slice(fans, func(i, j int) bool {
		return fans[i].Index < fans[j].Index
	})

	return fans, nil
}

func (s *service) SetFans(ctx context.Context, request models.SetFansRequest) ([]models.Fan, error) {
	s.emit(request.ClientID, "info", "Loading current fan state from iLO")
	fans, err := s.GetFans(ctx)
	if err != nil {
		s.emit(request.ClientID, "error", err.Error())
		return nil, err
	}

	targetSpeeds, err := s.normalizeTargetSpeeds(request, fans)
	if err != nil {
		s.emit(request.ClientID, "error", err.Error())
		return nil, err
	}

	commands := buildCommands(fans, targetSpeeds)
	if len(commands) == 0 {
		s.emit(request.ClientID, "info", "No changes detected. No commands need to be sent.")
		return fans, nil
	}

	s.emit(request.ClientID, "info", fmt.Sprintf("Opening SSH connection to %s", s.cfg.ILOHost))
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", s.cfg.ILOHost), s.sshConfig)
	if err != nil {
		s.emit(request.ClientID, "error", err.Error())
		return nil, err
	}
	defer client.Close()

	for _, command := range commands {
		s.emit(request.ClientID, "command", command)
		if err := s.runSSHCommand(client, command); err != nil {
			s.emit(request.ClientID, "error", err.Error())
			return nil, err
		}
	}

	s.emit(request.ClientID, "info", "Polling iLO for applied fan speeds")
	updatedFans, err := s.pollUntilApplied(ctx, targetSpeeds)
	if err != nil {
		s.emit(request.ClientID, "error", err.Error())
		return nil, err
	}

	s.emit(request.ClientID, "success", "Fan speeds applied successfully")
	return updatedFans, nil
}

func (s *service) normalizeTargetSpeeds(request models.SetFansRequest, fans []models.Fan) (map[string]int, error) {
	if request.Speed == nil && len(request.Fans) == 0 {
		return nil, fmt.Errorf("request must include either speed or fans")
	}

	knownFans := make(map[string]struct{}, len(fans))
	for _, fan := range fans {
		knownFans[fan.Name] = struct{}{}
	}

	targetSpeeds := make(map[string]int, len(fans))
	if request.Speed != nil {
		if err := s.validateSpeed(*request.Speed); err != nil {
			return nil, err
		}

		for _, fan := range fans {
			targetSpeeds[fan.Name] = *request.Speed
		}
	}

	for fanName, speed := range request.Fans {
		if _, ok := knownFans[fanName]; !ok {
			return nil, fmt.Errorf("unknown fan name: %s", fanName)
		}

		if err := s.validateSpeed(speed); err != nil {
			return nil, fmt.Errorf("%s: %w", fanName, err)
		}

		targetSpeeds[fanName] = speed
	}

	return targetSpeeds, nil
}

func (s *service) validateSpeed(speed int) error {
	if speed < s.cfg.MinimumFanSpeed || speed > 100 {
		return fmt.Errorf("speed must be between %d and 100", s.cfg.MinimumFanSpeed)
	}

	return nil
}

func (s *service) runSSHCommand(client *ssh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	var stderr bytes.Buffer
	session.Stderr = &stderr

	if err := session.Run(command); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}

		return err
	}

	return nil
}

func (s *service) pollUntilApplied(ctx context.Context, targetSpeeds map[string]int) ([]models.Fan, error) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		fans, err := s.GetFans(ctx)
		if err != nil {
			return nil, err
		}

		if fansMatchTargets(fans, targetSpeeds) {
			return fans, nil
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for fans to reach requested speeds")
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func fansMatchTargets(fans []models.Fan, targetSpeeds map[string]int) bool {
	for _, fan := range fans {
		targetSpeed, ok := targetSpeeds[fan.Name]
		if ok && fan.Speed != targetSpeed {
			return false
		}
	}

	return true
}

func percentageToILOValue(speed int) int {
	return int(math.Ceil(float64(speed) / 100 * 255))
}

func buildCommands(fans []models.Fan, targetSpeeds map[string]int) []string {
	commands := make([]string, 0, len(targetSpeeds)*2)
	for _, fan := range fans {
		targetSpeed, ok := targetSpeeds[fan.Name]
		if !ok || targetSpeed == fan.Speed {
			continue
		}

		commands = append(commands,
			fmt.Sprintf("fan p %d max %d", fan.Index, percentageToILOValue(targetSpeed)),
			fmt.Sprintf("fan p %d min 255", fan.Index),
		)
	}

	return commands
}

func (s *service) emit(clientID, eventType, message string) {
	if s.hub == nil {
		return
	}

	s.hub.Send(clientID, eventType, message)
}
