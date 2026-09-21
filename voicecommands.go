/*
 *    Copyright (c) 2026 Unrud <unrud@outlook.com>
 *
 *    This file is part of Remote-Touchpad.
 *
 *    Remote-Touchpad is free software: you can redistribute it and/or modify
 *    it under the terms of the GNU General Public License as published by
 *    the Free Software Foundation, either version 3 of the License, or
 *    (at your option) any later version.
 *
 *    Remote-Touchpad is distributed in the hope that it will be useful,
 *    but WITHOUT ANY WARRANTY; without even the implied warranty of
 *    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *    GNU General Public License for more details.
 *
 *    You should have received a copy of the GNU General Public License
 *    along with Remote-Touchpad.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unrud/remote-touchpad/inputcontrol"
	"go.bug.st/serial"
)

const defaultVoiceCommandBaudRate int = 9600

// voiceCommand maps a set of accepted speech phrases to an action, loaded from
// the voice commands JSON file (see -commands-file).
type voiceCommand struct {
	Phrases  []string `json:"phrases"`
	Action   string   `json:"action"` // "print" or "uart"
	Text     string   `json:"text"`
	Port     string   `json:"port"`     // uart action only, e.g. "COM11" or "/dev/ttyACM0"
	BaudRate int      `json:"baudRate"` // uart action only, defaults to 9600
}

type voiceCommands []voiceCommand

func voiceCommandsFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "remote-touchpad", "commands.json"), nil
}

// loadVoiceCommands returns an empty list without error if path doesn't exist,
// so the voice feature is simply inactive until a commands file is created.
func loadVoiceCommands(path string) (voiceCommands, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var commands voiceCommands
	if err := json.Unmarshal(data, &commands); err != nil {
		return nil, err
	}
	for i, cmd := range commands {
		if cmd.Action != "print" && cmd.Action != "uart" {
			return nil, fmt.Errorf("command %d: unsupported action %q", i, cmd.Action)
		}
		if len(cmd.Phrases) == 0 {
			return nil, fmt.Errorf("command %d: no phrases", i)
		}
		if cmd.Action == "uart" && cmd.Port == "" {
			return nil, fmt.Errorf("command %d: uart action requires a port", i)
		}
	}
	return commands, nil
}

func normalizeVoicePhrase(s string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(s)), ".!? ")
}

// match returns the first configured command whose phrase occurs in text.
func (commands voiceCommands) match(text string) (*voiceCommand, bool) {
	normalized := normalizeVoicePhrase(text)
	if normalized == "" {
		return nil, false
	}
	for i, cmd := range commands {
		for _, phrase := range cmd.Phrases {
			if p := normalizeVoicePhrase(phrase); p != "" && strings.Contains(normalized, p) {
				return &commands[i], true
			}
		}
	}
	return nil, false
}

// run executes the command's action and returns a short human-readable result
// that is sent back to the web client as feedback.
func (cmd *voiceCommand) run(controller inputcontrol.Controller) (string, error) {
	switch cmd.Action {
	case "print":
		if err := controller.KeyboardText(cmd.Text); err != nil {
			return "", err
		}
		return fmt.Sprintf("Printed: %s", cmd.Text), nil
	case "uart":
		if err := sendUART(cmd.Port, cmd.BaudRate, cmd.Text); err != nil {
			return "", err
		}
		return fmt.Sprintf("Sent to %s: %s", cmd.Port, cmd.Text), nil
	default:
		return "", fmt.Errorf("unsupported action %q", cmd.Action)
	}
}

func sendUART(port string, baudRate int, text string) error {
	if baudRate <= 0 {
		baudRate = defaultVoiceCommandBaudRate
	}
	p, err := serial.Open(port, &serial.Mode{BaudRate: baudRate})
	if err != nil {
		return fmt.Errorf("open %s: %w", port, err)
	}
	defer p.Close()
	if _, err := p.Write([]byte(text)); err != nil {
		return fmt.Errorf("write to %s: %w", port, err)
	}
	return nil
}
