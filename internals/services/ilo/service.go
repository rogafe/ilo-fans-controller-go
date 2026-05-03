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
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"golang.org/x/crypto/ssh"

	"ilo-fans-controller-go/internals/config"
	"ilo-fans-controller-go/internals/console"
	"ilo-fans-controller-go/internals/models"
)

type Service interface {
	GetFans(context.Context) ([]models.Fan, error)
	GetTemperatures(context.Context) ([]models.Temperature, error)
	SetFans(context.Context, models.SetFansRequest) ([]models.Fan, error)
	ApplyAdvancedProfile(context.Context, models.ApplyAdvancedProfileRequest, models.AdvancedProfile) error
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
		MemberID       string `json:"MemberId"`
		CurrentReading int    `json:"CurrentReading"`
		Status         struct {
			State string `json:"State"`
		} `json:"Status"`
	} `json:"Fans"`
	Temperatures []struct {
		Name                string `json:"Name"`
		Number              int    `json:"Number"`
		ReadingCelsius      int    `json:"ReadingCelsius"`
		PhysicalContext     string `json:"PhysicalContext"`
		UpperThresholdWarn  int    `json:"UpperThresholdNonCritical"`
		UpperThresholdCrit  int    `json:"UpperThresholdCritical"`
		UpperThresholdFatal int    `json:"UpperThresholdFatal"`
		Status              struct {
			Health string `json:"Health"`
			State  string `json:"State"`
		} `json:"Status"`
		Oem struct {
			Hp struct {
				LocationXmm int `json:"LocationXmm"`
				LocationYmm int `json:"LocationYmm"`
			} `json:"Hp"`
		} `json:"Oem"`
	} `json:"Temperatures"`
}

// interFanSSHSettleDelay is applied between each fan's command pair in SetFans so iLO can settle.
const interFanSSHSettleDelay = 75 * time.Millisecond

const (
	advancedProfileConfirmation = "APPLY ADVANCED PROFILE"
	sshFanIndexBase             = 0
	fanApplyMaxRetryPasses      = 2
	temperatureChassisOID       = ".1.3.6.1.4.1.232.6.2.6.8.1.1"
	temperatureIndexOID         = ".1.3.6.1.4.1.232.6.2.6.8.1.2"
	temperatureLocaleOID        = ".1.3.6.1.4.1.232.6.2.6.8.1.3"
	temperatureValueOID         = ".1.3.6.1.4.1.232.6.2.6.8.1.4"
	temperatureThresholdOID     = ".1.3.6.1.4.1.232.6.2.6.8.1.5"
	temperatureConditionOID     = ".1.3.6.1.4.1.232.6.2.6.8.1.6"
	temperatureThresholdTypeOID = ".1.3.6.1.4.1.232.6.2.6.8.1.7"
)

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
			User:              cfg.ILOUsername,
			Auth:              []ssh.AuthMethod{ssh.Password(cfg.ILOPassword)},
			HostKeyCallback:   ssh.InsecureIgnoreHostKey(),
			HostKeyAlgorithms: cfg.ILOSSHHostKeyAlgos,
			Config: ssh.Config{
				KeyExchanges: cfg.ILOSSHKexAlgos,
				Ciphers:      cfg.ILOSSHCiphers,
				MACs:         cfg.ILOSSHMACs,
			},
			Timeout: 10 * time.Second,
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

	return fansFromThermalResponse(thermal), nil
}

// fanSlotPresent reports whether a Redfish Thermal fan entry represents an installed
// slot we should expose and drive over SSH. Extend the negative list per iLO generation if needed.
func fanSlotPresent(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "absent", "unavailableoffline", "disabled":
		return false
	default:
		return true
	}
}

// fansFromThermalResponse maps Redfish Fans[] to models in JSON array order. CommandNumber keeps the
// full slot ordinal (with sshFanIndexBase applied), so hidden empty bays do not renumber later fans.
func fansFromThermalResponse(thermal thermalResponse) []models.Fan {
	fans := make([]models.Fan, 0, len(thermal.Fans))
	for index, fan := range thermal.Fans {
		if !fanSlotPresent(fan.Status.State) {
			continue
		}

		fans = append(fans, models.Fan{
			Name:          fan.FanName,
			Speed:         fan.CurrentReading,
			CommandNumber: index + sshFanIndexBase,
		})
	}

	sort.Slice(fans, func(i, j int) bool {
		return fans[i].CommandNumber < fans[j].CommandNumber
	})

	return fans
}

func (s *service) GetTemperatures(ctx context.Context) ([]models.Temperature, error) {
	if !s.cfg.HasILOSNMPConfig() {
		return nil, fmt.Errorf("iLO SNMP is not configured")
	}

	redfishTemperatures, redfishErr := s.getRedfishTemperatures(ctx)

	client := gosnmp.GoSNMP{
		Target:    s.cfg.ILOSNMPHost,
		Port:      s.cfg.ILOSNMPPort,
		Community: s.cfg.ILOSNMPCommunity,
		Version:   snmpVersion(s.cfg.ILOSNMPVersion),
		Timeout:   time.Duration(s.cfg.ILOSNMPTimeoutSeconds) * time.Second,
		Retries:   s.cfg.ILOSNMPRetries,
		Context:   ctx,
	}

	if err := client.Connect(); err != nil {
		return nil, err
	}
	defer client.Conn.Close()

	temperaturesByIndex := make(map[int]*models.Temperature)
	for index, temperature := range redfishTemperatures {
		copy := temperature
		temperaturesByIndex[index] = &copy
	}

	for _, rootOID := range []string{
		temperatureChassisOID,
		temperatureIndexOID,
		temperatureLocaleOID,
		temperatureValueOID,
		temperatureThresholdOID,
		temperatureConditionOID,
		temperatureThresholdTypeOID,
	} {
		packets, err := client.WalkAll(rootOID)
		if err != nil {
			return nil, err
		}

		for _, packet := range packets {
			index, ok := snmpIndex(packet.Name)
			if !ok {
				continue
			}

			temperature := temperaturesByIndex[index]
			if temperature == nil {
				temperature = &models.Temperature{Index: index}
				temperaturesByIndex[index] = temperature
			}

			switch rootOID {
			case temperatureLocaleOID:
				temperature.Locale = int(gosnmp.ToBigInt(packet.Value).Int64())
				temperature.LocaleLabel = temperatureLocaleLabel(temperature.Locale)
			case temperatureValueOID:
				temperature.Temperature = int(gosnmp.ToBigInt(packet.Value).Int64())
			case temperatureChassisOID:
				temperature.Chassis = int(gosnmp.ToBigInt(packet.Value).Int64())
			case temperatureIndexOID:
				temperature.Index = int(gosnmp.ToBigInt(packet.Value).Int64())
			case temperatureThresholdOID:
				temperature.Threshold = int(gosnmp.ToBigInt(packet.Value).Int64())
			case temperatureConditionOID:
				temperature.Condition = int(gosnmp.ToBigInt(packet.Value).Int64())
				temperature.ConditionLabel = temperatureConditionLabel(temperature.Condition)
			case temperatureThresholdTypeOID:
				temperature.ThresholdType = int(gosnmp.ToBigInt(packet.Value).Int64())
				temperature.ThresholdTypeLabel = temperatureThresholdTypeLabel(temperature.ThresholdType)
			}
		}
	}

	temperatures := make([]models.Temperature, 0, len(temperaturesByIndex))
	for _, temperature := range temperaturesByIndex {
		if temperature.LocaleLabel == "" {
			temperature.LocaleLabel = temperatureLocaleLabel(temperature.Locale)
		}
		if temperature.ConditionLabel == "" {
			temperature.ConditionLabel = temperatureConditionLabel(temperature.Condition)
		}
		if temperature.ThresholdTypeLabel == "" {
			temperature.ThresholdTypeLabel = temperatureThresholdTypeLabel(temperature.ThresholdType)
		}
		if temperature.Label == "" {
			temperature.Label = fmt.Sprintf("Sensor %02d", temperature.Index)
		}
		if !temperature.Present {
			temperature.Present = !strings.EqualFold(strings.TrimSpace(temperature.State), "Absent")
		}
		if !temperature.Present {
			continue
		}
		if temperature.Temperature == 0 && temperature.Condition == 0 && redfishErr != nil {
			continue
		}

		temperatures = append(temperatures, *temperature)
	}

	sort.Slice(temperatures, func(i, j int) bool {
		return temperatures[i].Index < temperatures[j].Index
	})

	return temperatures, nil
}

func (s *service) getRedfishTemperatures(ctx context.Context) (map[int]models.Temperature, error) {
	if !s.cfg.HasILOConfig() {
		return map[int]models.Temperature{}, nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s/redfish/v1/chassis/1/Thermal/", s.cfg.ILOHost), nil)
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

	result := make(map[int]models.Temperature, len(thermal.Temperatures))
	for _, temperature := range thermal.Temperatures {
		name := strings.TrimSpace(temperature.Name)
		if name == "" {
			name = fmt.Sprintf("Sensor %02d", temperature.Number)
		}

		result[temperature.Number] = models.Temperature{
			Index:             temperature.Number,
			Label:             name,
			PhysicalContext:   strings.TrimSpace(temperature.PhysicalContext),
			Temperature:       temperature.ReadingCelsius,
			Health:            strings.TrimSpace(temperature.Status.Health),
			State:             strings.TrimSpace(temperature.Status.State),
			CautionThreshold:  maxInt(temperature.UpperThresholdWarn, temperature.UpperThresholdCrit),
			CriticalThreshold: temperature.UpperThresholdFatal,
			LocationX:         temperature.Oem.Hp.LocationXmm,
			LocationY:         temperature.Oem.Hp.LocationYmm,
			Present:           !strings.EqualFold(strings.TrimSpace(temperature.Status.State), "Absent"),
		}
	}

	return result, nil
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

	if err := s.runFanCommandPairs(ctx, client, commands, request.ClientID); err != nil {
		s.emit(request.ClientID, "error", err.Error())
		return nil, err
	}

	var updatedFans []models.Fan
	for attempt := 0; ; attempt++ {
		s.emit(request.ClientID, "info", "Polling iLO for applied fan speeds")
		updatedFans, err = s.pollUntilApplied(ctx, targetSpeeds)
		if err == nil {
			break
		}

		fansAfter, errFresh := s.GetFans(ctx)
		if errFresh != nil {
			s.emit(request.ClientID, "error", errFresh.Error())
			return nil, errFresh
		}

		retryCommands := buildCommandsForOutOfTolerance(fansAfter, targetSpeeds, s.cfg.FanApplyTolerance)
		if len(retryCommands) == 0 {
			s.emit(request.ClientID, "error", err.Error())
			return nil, err
		}

		if attempt >= fanApplyMaxRetryPasses {
			s.emit(request.ClientID, "error", err.Error())
			return nil, err
		}

		s.emit(request.ClientID, "info", fmt.Sprintf("Re-applying fan SSH for fans still out of tolerance (pass %d/%d)", attempt+1, fanApplyMaxRetryPasses))
		if err := s.runFanCommandPairs(ctx, client, retryCommands, request.ClientID); err != nil {
			s.emit(request.ClientID, "error", err.Error())
			return nil, err
		}
	}

	s.emit(request.ClientID, "success", "Fan speeds applied successfully")
	return updatedFans, nil
}

func (s *service) ApplyAdvancedProfile(ctx context.Context, request models.ApplyAdvancedProfileRequest, profile models.AdvancedProfile) error {
	if strings.TrimSpace(request.Confirmation) != advancedProfileConfirmation {
		return fmt.Errorf("confirmation phrase must be exactly %q", advancedProfileConfirmation)
	}

	if !s.cfg.HasILOConfig() {
		return fmt.Errorf("iLO credentials are not configured")
	}

	if err := validateAdvancedProfile(profile); err != nil {
		return err
	}

	commands := buildAdvancedCommands(profile)
	if len(commands) == 0 {
		return fmt.Errorf("advanced profile %q has no commands to apply", profile.Name)
	}

	s.emit(request.ClientID, "warning", fmt.Sprintf("Applying advanced profile %q", profile.Name))
	s.emit(request.ClientID, "warning", profile.Warning)
	s.emit(request.ClientID, "info", fmt.Sprintf("Opening SSH connection to %s", s.cfg.ILOHost))

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", s.cfg.ILOHost), s.sshConfig)
	if err != nil {
		s.emit(request.ClientID, "error", err.Error())
		return err
	}
	defer client.Close()

	for _, command := range commands {
		select {
		case <-ctx.Done():
			s.emit(request.ClientID, "error", ctx.Err().Error())
			return ctx.Err()
		default:
		}

		s.emit(request.ClientID, "command", command)
		if err := s.runSSHCommand(client, command); err != nil {
			s.emit(request.ClientID, "error", err.Error())
			return err
		}
	}

	s.emit(request.ClientID, "success", fmt.Sprintf("Advanced profile %q applied successfully", profile.Name))
	return nil
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

func (s *service) runFanCommandPairs(ctx context.Context, client *ssh.Client, commands []string, clientID string) error {
	if len(commands)%2 != 0 {
		return fmt.Errorf("internal error: fan SSH commands must come in max/min pairs")
	}

	for i := 0; i < len(commands); i += 2 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		for j := i; j < i+2; j++ {
			s.emit(clientID, "command", commands[j])
			if err := s.runSSHCommand(client, commands[j]); err != nil {
				return err
			}
		}

		if i+2 < len(commands) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interFanSSHSettleDelay):
			}
		}
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
	timeout := time.Duration(s.cfg.FanApplyTimeoutSeconds) * time.Second
	deadline := time.Now().Add(timeout)
	for {
		fans, err := s.GetFans(ctx)
		if err != nil {
			return nil, err
		}

		if fansMatchTargets(fans, targetSpeeds, s.cfg.FanApplyTolerance) {
			return fans, nil
		}

		if time.Now().After(deadline) {
			details := formatFanMismatchDetails(fans, targetSpeeds)
			if details != "" {
				return nil, fmt.Errorf("timed out waiting for fans to reach requested speeds (tolerance=%d, timeout=%s): %s", s.cfg.FanApplyTolerance, timeout, details)
			}
			return nil, fmt.Errorf("timed out waiting for fans to reach requested speeds (tolerance=%d, timeout=%s)", s.cfg.FanApplyTolerance, timeout)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func fansMatchTargets(fans []models.Fan, targetSpeeds map[string]int, tolerance int) bool {
	for _, fan := range fans {
		targetSpeed, ok := targetSpeeds[fan.Name]
		if !ok {
			continue
		}

		delta := fan.Speed - targetSpeed
		if delta < 0 {
			delta = -delta
		}
		if delta > tolerance {
			return false
		}
	}

	return true
}

type fanMismatch struct {
	name   string
	actual int
	target int
	delta  int
}

func formatFanMismatchDetails(fans []models.Fan, targetSpeeds map[string]int) string {
	mismatches := make([]fanMismatch, 0, len(targetSpeeds))
	for _, fan := range fans {
		target, ok := targetSpeeds[fan.Name]
		if !ok {
			continue
		}

		delta := fan.Speed - target
		if delta < 0 {
			delta = -delta
		}
		mismatches = append(mismatches, fanMismatch{
			name:   fan.Name,
			actual: fan.Speed,
			target: target,
			delta:  delta,
		})
	}

	if len(mismatches) == 0 {
		return ""
	}

	sort.Slice(mismatches, func(i, j int) bool {
		return mismatches[i].delta > mismatches[j].delta
	})

	limit := 5
	if len(mismatches) < limit {
		limit = len(mismatches)
	}

	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		m := mismatches[i]
		parts = append(parts, fmt.Sprintf("%s=%d (target %d)", m.name, m.actual, m.target))
	}

	return strings.Join(parts, ", ")
}

func percentageToILOValue(speed int) int {
	return int(math.Ceil(float64(speed) / 100 * 255))
}

func buildCommands(fans []models.Fan, targetSpeeds map[string]int) []string {
	sorted := append([]models.Fan(nil), fans...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CommandNumber > sorted[j].CommandNumber
	})

	commands := make([]string, 0, len(targetSpeeds)*2)
	for _, fan := range sorted {
		targetSpeed, ok := targetSpeeds[fan.Name]
		if !ok || targetSpeed == fan.Speed {
			continue
		}

		pwm := percentageToILOValue(targetSpeed)
		commands = append(commands,
			fmt.Sprintf("fan p %d max %d", fan.CommandNumber, pwm),
			fmt.Sprintf("fan p %d min 255", fan.CommandNumber),
		)
	}

	return commands
}

func buildCommandsForOutOfTolerance(fans []models.Fan, targetSpeeds map[string]int, tolerance int) []string {
	sorted := append([]models.Fan(nil), fans...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CommandNumber > sorted[j].CommandNumber
	})

	commands := make([]string, 0, len(targetSpeeds)*2)
	for _, fan := range sorted {
		targetSpeed, ok := targetSpeeds[fan.Name]
		if !ok {
			continue
		}

		delta := fan.Speed - targetSpeed
		if delta < 0 {
			delta = -delta
		}
		if delta <= tolerance {
			continue
		}

		pwm := percentageToILOValue(targetSpeed)
		commands = append(commands,
			fmt.Sprintf("fan p %d max %d", fan.CommandNumber, pwm),
			fmt.Sprintf("fan p %d min 255", fan.CommandNumber),
		)
	}

	return commands
}

func buildAdvancedCommands(profile models.AdvancedProfile) []string {
	commands := make([]string, 0, len(profile.CommandBundle.FanMinimums)+len(profile.CommandBundle.FanMaximums)+len(profile.CommandBundle.PIDLows)+len(profile.CommandBundle.PIDHighs)+len(profile.CommandBundle.OCSD)+len(profile.CommandBundle.DisabledSensors))

	for _, command := range profile.CommandBundle.FanMinimums {
		commands = append(commands, fmt.Sprintf("fan p %d min %d", command.Fan, command.Value))
	}

	for _, command := range profile.CommandBundle.FanMaximums {
		commands = append(commands, fmt.Sprintf("fan p %d max %d", command.Fan, command.Value))
	}

	for _, command := range profile.CommandBundle.PIDLows {
		commands = append(commands, fmt.Sprintf("fan pid %s lo %d", formatSensorSet(command.Sensors), command.Value))
	}

	for _, command := range profile.CommandBundle.PIDHighs {
		commands = append(commands, fmt.Sprintf("fan pid %s hi %d", formatSensorSet(command.Sensors), command.Value))
	}

	for _, command := range profile.CommandBundle.OCSD {
		commands = append(commands, fmt.Sprintf("ocsd setts %s %d", formatSensorSet(command.Sensors), command.Value))
	}

	for _, sensor := range profile.CommandBundle.DisabledSensors {
		commands = append(commands, fmt.Sprintf("fan t %d off", sensor))
	}

	return commands
}

func formatSensorSet(sensors []int) string {
	values := make([]string, 0, len(sensors))
	for _, sensor := range sensors {
		values = append(values, strconv.Itoa(sensor))
	}

	return "{" + strings.Join(values, ",") + "}"
}

func validateAdvancedProfile(profile models.AdvancedProfile) error {
	if strings.TrimSpace(profile.Name) == "" {
		return fmt.Errorf("profile name is required")
	}

	for _, command := range profile.CommandBundle.FanMinimums {
		if err := validateFanBoundCommand(command); err != nil {
			return fmt.Errorf("fan minimum: %w", err)
		}
	}

	for _, command := range profile.CommandBundle.FanMaximums {
		if err := validateFanBoundCommand(command); err != nil {
			return fmt.Errorf("fan maximum: %w", err)
		}
	}

	for _, command := range profile.CommandBundle.PIDLows {
		if err := validateSensorCommand(command.Sensors, command.Value); err != nil {
			return fmt.Errorf("pid low: %w", err)
		}
	}

	for _, command := range profile.CommandBundle.PIDHighs {
		if err := validateSensorCommand(command.Sensors, command.Value); err != nil {
			return fmt.Errorf("pid high: %w", err)
		}
	}

	for _, command := range profile.CommandBundle.OCSD {
		if err := validateSensorCommand(command.Sensors, command.Value); err != nil {
			return fmt.Errorf("ocsd: %w", err)
		}
	}

	for _, sensor := range profile.CommandBundle.DisabledSensors {
		if sensor < 0 {
			return fmt.Errorf("disabled sensor ids must be >= 0")
		}
	}

	return nil
}

func ValidateAdvancedProfile(profile models.AdvancedProfile) error {
	return validateAdvancedProfile(profile)
}

func validateFanBoundCommand(command models.FanBoundCommand) error {
	if command.Fan < 1 {
		return fmt.Errorf("fan ids must be >= 1")
	}
	if command.Value < 0 || command.Value > 100 {
		return fmt.Errorf("fan values must be between 0 and 100")
	}

	return nil
}

func validateSensorCommand(sensors []int, value int) error {
	if len(sensors) == 0 {
		return fmt.Errorf("sensor list cannot be empty")
	}

	for _, sensor := range sensors {
		if sensor < 0 {
			return fmt.Errorf("sensor ids must be >= 0")
		}
	}

	if value < 0 {
		return fmt.Errorf("value must be >= 0")
	}

	return nil
}

func (s *service) emit(clientID, eventType, message string) {
	if s.hub == nil {
		return
	}

	s.hub.Send(clientID, eventType, message)
}

func snmpVersion(version string) gosnmp.SnmpVersion {
	if strings.EqualFold(strings.TrimSpace(version), "1") || strings.EqualFold(strings.TrimSpace(version), "v1") {
		return gosnmp.Version1
	}

	return gosnmp.Version2c
}

func snmpIndex(oid string) (int, bool) {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(oid), "."), ".")
	if len(parts) == 0 {
		return 0, false
	}

	index, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0, false
	}

	return index, true
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(value)
	}
}

func temperatureLocaleLabel(locale int) string {
	switch locale {
	case 1:
		return "Other"
	case 2:
		return "Unknown"
	case 3:
		return "System"
	case 4:
		return "System Board"
	case 5:
		return "I/O Board"
	case 6:
		return "CPU"
	case 7:
		return "Memory"
	case 8:
		return "Storage"
	case 9:
		return "Removable Media"
	case 10:
		return "Power Supply"
	case 11:
		return "Ambient"
	case 12:
		return "Chassis"
	case 13:
		return "Bridge Card"
	default:
		return ""
	}
}

func temperatureConditionLabel(condition int) string {
	switch condition {
	case 1:
		return "Other"
	case 2:
		return "OK"
	case 3:
		return "Degraded"
	case 4:
		return "Failed"
	default:
		return ""
	}
}

func temperatureThresholdTypeLabel(thresholdType int) string {
	switch thresholdType {
	case 1:
		return "Other"
	case 5:
		return "Blowout"
	case 9:
		return "Caution"
	case 15:
		return "Critical"
	default:
		return ""
	}
}

func maxInt(values ...int) int {
	max := 0
	for _, value := range values {
		if value > max {
			max = value
		}
	}

	return max
}
