package ilo

import (
	"context"
	"strings"
	"testing"

	"ilo-fans-controller-go/internals/config"
	"ilo-fans-controller-go/internals/models"
)

func TestBuildAdvancedCommandsConservative(t *testing.T) {
	t.Parallel()

	commands := buildAdvancedCommands(conservativeProfileFixture())

	if len(commands) != 24 {
		t.Fatalf("len(commands) = %d, want 24", len(commands))
	}

	wantPrefix := []string{
		"fan p 1 min 12",
		"fan p 2 min 12",
		"fan p 3 min 12",
		"fan p 4 min 12",
		"fan p 5 min 12",
		"fan p 6 min 12",
		"fan pid {33,34,35,36,37,38,42,47,52,53,54,55,56,57,58,59,60,61,62,63} lo 3100",
		"fan pid {53,55,57,61,63} hi 3100",
		"ocsd setts {24,26,27,28,29,30,31,32,44} 2",
	}

	for i, want := range wantPrefix {
		if commands[i] != want {
			t.Fatalf("commands[%d] = %q, want %q", i, commands[i], want)
		}
	}

	if commands[len(commands)-1] != "fan t 27 off" {
		t.Fatalf("commands[last] = %q, want %q", commands[len(commands)-1], "fan t 27 off")
	}
}

func TestBuildAdvancedCommandsAggressive(t *testing.T) {
	t.Parallel()

	commands := buildAdvancedCommands(aggressiveProfileFixture())

	if got, want := len(commands), 96; got != want {
		t.Fatalf("len(commands) = %d, want %d", got, want)
	}

	wantPrefix := []string{
		"fan p 1 min 8",
		"fan p 2 min 8",
		"fan p 3 min 8",
		"fan p 4 min 8",
		"fan p 5 min 8",
		"fan p 6 min 8",
		"fan p 1 max 50",
		"fan p 2 max 50",
		"fan p 3 max 50",
		"fan p 4 max 50",
		"fan p 5 max 50",
		"fan p 6 max 50",
		"fan pid {33,34,35,36,37,38,42,47,52,53,54,55,56,57,58,59,60,61,62,63} lo 2500",
		"fan pid {53,55,57,61,63} hi 2500",
		"ocsd setts {0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39,40,41,42,43,44,45} 2",
	}

	for i, want := range wantPrefix {
		if commands[i] != want {
			t.Fatalf("commands[%d] = %q, want %q", i, commands[i], want)
		}
	}

	if commands[len(commands)-1] != "fan t 80 off" {
		t.Fatalf("commands[last] = %q, want %q", commands[len(commands)-1], "fan t 80 off")
	}
}

func TestFormatSensorSet(t *testing.T) {
	t.Parallel()

	if got, want := formatSensorSet([]int{0, 1, 2, 5, 9}), "{0,1,2,5,9}"; got != want {
		t.Fatalf("formatSensorSet() = %q, want %q", got, want)
	}
}

func TestValidateAdvancedProfileRejectsInvalidProfiles(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		profile models.AdvancedProfile
		want    string
	}{
		{
			name:    "empty name",
			profile: models.AdvancedProfile{},
			want:    "profile name is required",
		},
		{
			name: "empty pid sensors",
			profile: models.AdvancedProfile{
				Name: "broken",
				CommandBundle: models.AdvancedCommandBundle{
					PIDLows: []models.PIDCommand{{Sensors: nil, Value: 1}},
				},
			},
			want: "sensor list cannot be empty",
		},
		{
			name: "negative disabled sensor",
			profile: models.AdvancedProfile{
				Name: "broken",
				CommandBundle: models.AdvancedCommandBundle{
					DisabledSensors: []int{-1},
				},
			},
			want: "disabled sensor ids must be >= 0",
		},
		{
			name: "invalid fan value",
			profile: models.AdvancedProfile{
				Name: "broken",
				CommandBundle: models.AdvancedCommandBundle{
					FanMinimums: []models.FanBoundCommand{{Fan: 1, Value: 101}},
				},
			},
			want: "fan values must be between 0 and 100",
		},
		{
			name: "invalid fan id",
			profile: models.AdvancedProfile{
				Name: "broken",
				CommandBundle: models.AdvancedCommandBundle{
					FanMinimums: []models.FanBoundCommand{{Fan: 0, Value: 10}},
				},
			},
			want: "fan ids must be >= 1",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateAdvancedProfile(tc.profile)
			if err == nil {
				t.Fatal("validateAdvancedProfile returned nil error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestApplyAdvancedProfileRejectsWrongConfirmation(t *testing.T) {
	t.Parallel()

	service := New(zeroConfig(), nil)

	err := service.ApplyAdvancedProfile(context.Background(), models.ApplyAdvancedProfileRequest{
		Confirmation: "WRONG",
	}, aggressiveProfileFixture())
	if err == nil {
		t.Fatal("ApplyAdvancedProfile returned nil error")
	}

	if !strings.Contains(err.Error(), advancedProfileConfirmation) {
		t.Fatalf("error %q does not contain confirmation phrase", err.Error())
	}
}

func conservativeProfileFixture() models.AdvancedProfile {
	return models.AdvancedProfile{
		Name: "Conservative",
		CommandBundle: models.AdvancedCommandBundle{
			FanMinimums: []models.FanBoundCommand{
				{Fan: 1, Value: 12}, {Fan: 2, Value: 12}, {Fan: 3, Value: 12},
				{Fan: 4, Value: 12}, {Fan: 5, Value: 12}, {Fan: 6, Value: 12},
			},
			PIDLows: []models.PIDCommand{{
				Sensors: []int{33, 34, 35, 36, 37, 38, 42, 47, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63},
				Value:   3100,
			}},
			PIDHighs: []models.PIDCommand{{
				Sensors: []int{53, 55, 57, 61, 63},
				Value:   3100,
			}},
			OCSD: []models.OCSDCommand{{
				Sensors: []int{24, 26, 27, 28, 29, 30, 31, 32, 44},
				Value:   2,
			}},
			DisabledSensors: []int{32, 45, 31, 41, 37, 38, 29, 34, 35, 30, 40, 36, 28, 33, 27},
		},
	}
}

func aggressiveProfileFixture() models.AdvancedProfile {
	profile := conservativeProfileFixture()
	profile.Name = "Aggressive"
	profile.CommandBundle.FanMinimums = []models.FanBoundCommand{
		{Fan: 1, Value: 8}, {Fan: 2, Value: 8}, {Fan: 3, Value: 8},
		{Fan: 4, Value: 8}, {Fan: 5, Value: 8}, {Fan: 6, Value: 8},
	}
	profile.CommandBundle.FanMaximums = []models.FanBoundCommand{
		{Fan: 1, Value: 50}, {Fan: 2, Value: 50}, {Fan: 3, Value: 50},
		{Fan: 4, Value: 50}, {Fan: 5, Value: 50}, {Fan: 6, Value: 50},
	}
	profile.CommandBundle.PIDLows[0].Value = 2500
	profile.CommandBundle.PIDHighs[0].Value = 2500

	profile.CommandBundle.OCSD = []models.OCSDCommand{{
		Sensors: func() []int {
			values := make([]int, 0, 46)
			for i := 0; i <= 45; i++ {
				values = append(values, i)
			}
			return values
		}(),
		Value: 2,
	}}
	profile.CommandBundle.DisabledSensors = func() []int {
		values := make([]int, 0, 81)
		for i := 0; i <= 80; i++ {
			values = append(values, i)
		}
		return values
	}()

	return profile
}

func TestBuildCommandsPinsMinAndMaxToSamePWM(t *testing.T) {
	t.Parallel()

	fans := []models.Fan{{Name: "Fan 1", Speed: 50, CommandNumber: 1}}
	targets := map[string]int{"Fan 1": 15}

	commands := buildCommands(fans, targets)
	want := []string{"fan p 1 min 39", "fan p 1 max 39"}
	if len(commands) != len(want) {
		t.Fatalf("len(commands) = %d, want %d: %v", len(commands), len(want), commands)
	}
	for i := range want {
		if commands[i] != want[i] {
			t.Fatalf("commands[%d] = %q, want %q", i, commands[i], want[i])
		}
	}
}

func TestFansMatchTargetsWithinTolerance(t *testing.T) {
	t.Parallel()

	fans := []models.Fan{{Name: "Fan 1", Speed: 37}}
	targets := map[string]int{"Fan 1": 15}
	if fansMatchTargets(fans, targets, 2) {
		t.Fatal("fansMatchTargets(37 vs 15, tol=2) = true, want false")
	}

	for _, speed := range []int{13, 14, 15, 16, 17} {
		f := []models.Fan{{Name: "Fan 1", Speed: speed}}
		if !fansMatchTargets(f, targets, 2) {
			t.Fatalf("fansMatchTargets(speed=%d vs 15, tol=2) = false, want true", speed)
		}
	}
}

func zeroConfig() config.Config {
	return config.Config{}
}
