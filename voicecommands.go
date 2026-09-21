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

// voiceMatchThreshold is the minimum average per-word similarity (0-1) a
// phrase needs to be accepted as a match for recognized (and possibly
// misheard) speech.
const voiceMatchThreshold float64 = 0.6

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

func normalizeVoiceWords(s string) []string {
	words := make([]string, 0)
	for _, word := range strings.Fields(strings.ToLower(s)) {
		if word = strings.Trim(word, ".,!?;:\"'()"); word != "" {
			words = append(words, word)
		}
	}
	return words
}

// levenshtein returns the edit distance between two rune slices.
func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

// wordSimilarity returns 1 for identical words, down to 0 for completely
// different ones, based on their edit distance relative to their length.
func wordSimilarity(a, b string) float64 {
	if a == b {
		return 1
	}
	ra, rb := []rune(a), []rune(b)
	maxLen := max(len(ra), len(rb))
	if maxLen == 0 {
		return 1
	}
	return 1 - float64(levenshtein(ra, rb))/float64(maxLen)
}

// phraseScore is the best average per-word similarity of phraseWords against
// any equal-length, consecutive window of inputWords (so extra words spoken
// before/after the phrase don't prevent a match).
func phraseScore(phraseWords, inputWords []string) float64 {
	if len(phraseWords) == 0 || len(inputWords) < len(phraseWords) {
		return 0
	}
	best := 0.0
	for start := 0; start+len(phraseWords) <= len(inputWords); start++ {
		sum := 0.0
		for i, word := range phraseWords {
			sum += wordSimilarity(word, inputWords[start+i])
		}
		if score := sum / float64(len(phraseWords)); score > best {
			best = score
		}
	}
	return best
}

// match fuzzily compares text against all configured phrases (tolerating
// misrecognized words and extra words before/after) and returns the command
// whose closest phrase scores highest, provided it clears voiceMatchThreshold.
func (commands voiceCommands) match(text string) (*voiceCommand, bool) {
	inputWords := normalizeVoiceWords(text)
	if len(inputWords) == 0 {
		return nil, false
	}
	var best *voiceCommand
	bestScore := 0.0
	for i, cmd := range commands {
		for _, phrase := range cmd.Phrases {
			if score := phraseScore(normalizeVoiceWords(phrase), inputWords); score > bestScore {
				bestScore = score
				best = &commands[i]
			}
		}
	}
	if bestScore < voiceMatchThreshold {
		return nil, false
	}
	return best, true
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
