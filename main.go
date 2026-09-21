/*
 *    Copyright (c) 2018-2019 Unrud <unrud@outlook.com>
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
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	mathrand "math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/unrud/remote-touchpad/inputcontrol"
	"github.com/unrud/remote-touchpad/terminal"
	"golang.org/x/net/websocket"
)

const (
	defaultSecretLength     int           = 12
	authenticationRateLimit time.Duration = time.Second / 10
	authenticationRateBurst int           = 10
	challengeLength         int           = 12
	defaultBind             string        = ":0"
	version                 string        = "1.5.5"
	prettyAppName           string        = "Remote Touchpad"
	// pixels/second of cursor movement per pixel of joystick deflection (at gain 1)
	joystickBaseGain float64 = 0.4
	// clamp elapsed time between joystick updates to avoid jumps after pauses (seconds)
	joystickMaxDT float64 = 0.25
	// upper bound for the hold-duration acceleration multiplier
	joystickMaxAccelMult float64 = 8
)

type config struct {
	UpdateRate           uint    `json:"updateRate"`
	ScrollSpeed          float64 `json:"scrollSpeed"`
	MoveSpeed            float64 `json:"moveSpeed"`
	MouseScrollSpeed     float64 `json:"mouseScrollSpeed"`
	MouseMoveSpeed       float64 `json:"mouseMoveSpeed"`
	MouseMode            string  `json:"mouseMode"`
	JoystickDeadzone     float64 `json:"joystickDeadzone"`
	JoystickAcceleration float64 `json:"joystickAcceleration"`
}

// persistentSettings holds the user-adjustable settings that are saved to disk,
// so the next server start resumes with the settings last used in the web GUI.
type persistentSettings struct {
	MouseMode            string  `json:"mouseMode"`
	MoveSpeed            float64 `json:"moveSpeed"`
	JoystickDeadzone     float64 `json:"joystickDeadzone"`
	JoystickAcceleration float64 `json:"joystickAcceleration"`
}

func defaultPersistentSettings() persistentSettings {
	return persistentSettings{
		MouseMode:            "trackpad",
		MoveSpeed:            1,
		JoystickDeadzone:     4,
		JoystickAcceleration: 1,
	}
}

// settingsFilePath returns the location of the persisted settings file, using the
// OS-specific user config directory. Returns an error if it cannot be determined.
func settingsFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "remote-touchpad", "settings.json"), nil
}

func loadPersistentSettings(path string) (persistentSettings, error) {
	settings := defaultPersistentSettings()
	data, err := os.ReadFile(path)
	if err != nil {
		return settings, err
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return defaultPersistentSettings(), err
	}
	return settings, nil
}

func savePersistentSettings(path string, settings persistentSettings) error {
	if path == "" {
		return errors.New("no settings file path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// settingsStore holds the settings currently in effect, shared by all connections,
// and persists changes to disk. Safe for concurrent use.
type settingsStore struct {
	mu      sync.Mutex
	path    string
	current persistentSettings
}

func newSettingsStore(path string, initial persistentSettings) *settingsStore {
	return &settingsStore{path: path, current: initial}
}

func (s *settingsStore) snapshot() persistentSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *settingsStore) update(settings persistentSettings) {
	s.mu.Lock()
	s.current = settings
	s.mu.Unlock()
	if err := savePersistentSettings(s.path, settings); err != nil {
		log.Printf("failed to save settings: %v", err)
	}
}

// per-connection state for joystick mode, only accessed by a single connection's goroutine
type joystickState struct {
	gain         float64
	deadzone     float64
	acceleration float64
	holding      bool
	holdStart    time.Time
	lastUpdate   time.Time
	remainderX   float64
	remainderY   float64
}

// applyJoystickMove converts the current finger offset from the zero point into
// proportional cursor movement, applying deadzone, gain and hold-duration acceleration.
func applyJoystickMove(controller inputcontrol.Controller, js *joystickState, offsetX, offsetY float64) error {
	now := time.Now()
	mag := math.Hypot(offsetX, offsetY)
	if mag <= js.deadzone {
		js.holding = false
		js.remainderX, js.remainderY = 0, 0
		js.lastUpdate = now
		return nil
	}
	if !js.holding {
		js.holding = true
		js.holdStart = now
		js.lastUpdate = now
	}
	dt := now.Sub(js.lastUpdate).Seconds()
	if dt > joystickMaxDT {
		dt = joystickMaxDT
	}
	js.lastUpdate = now
	accelMult := 1 + js.acceleration*now.Sub(js.holdStart).Seconds()
	if accelMult > joystickMaxAccelMult {
		accelMult = joystickMaxAccelMult
	}
	effMag := mag - js.deadzone
	speed := effMag * joystickBaseGain * js.gain * accelMult
	moveX := offsetX/mag*speed*dt + js.remainderX
	moveY := offsetY/mag*speed*dt + js.remainderY
	intX, intY := int(moveX), int(moveY)
	js.remainderX = moveX - float64(intX)
	js.remainderY = moveY - float64(intY)
	if intX == 0 && intY == 0 {
		return nil
	}
	return controller.PointerMove(intX, intY)
}

const (
	commandKeyboardText            byte = 't'
	commandKeyboardKey             byte = 'k'
	commandPointerScrollInProgress byte = 's'
	commandPointerScrollFinished   byte = 'S'
	commandPointerMove             byte = 'm'
	commandPointerButton           byte = 'b'
	commandPointerJoystickMove     byte = 'j'
	commandJoystickConfig          byte = 'g'
)

func processCommand(controller inputcontrol.Controller, js *joystickState, store *settingsStore, commandWithArg string) error {
	if len(commandWithArg) == 0 {
		return errors.New("empty command")
	}
	command := commandWithArg[0]
	arg := commandWithArg[1:]
	parseInts := func(s string, targets ...*int) error {
		parts := strings.Split(s, ";")
		if len(parts) != len(targets) {
			return errors.New("wrong number of arguments")
		}
		for i, part := range parts {
			var err error
			if *targets[i], err = strconv.Atoi(part); err != nil {
				return fmt.Errorf("argument %d: %w", i, err)
			}
		}
		return nil
	}
	parseFloats := func(s string, targets ...*float64) error {
		parts := strings.Split(s, ";")
		if len(parts) != len(targets) {
			return errors.New("wrong number of arguments")
		}
		for i, part := range parts {
			var err error
			if *targets[i], err = strconv.ParseFloat(part, 64); err != nil {
				return fmt.Errorf("argument %d: %w", i, err)
			}
		}
		return nil
	}
	switch command {
	case commandKeyboardText:
		if !utf8.ValidString(arg) {
			return errors.New("invalid utf-8")
		}
		return controller.KeyboardText(arg)
	case commandKeyboardKey:
		var key inputcontrol.Key
		if err := parseInts(arg, (*int)(&key)); err != nil {
			return err
		}
		if key < 0 || key >= inputcontrol.KeyLimit {
			return errors.New("unsupported key")
		}
		return controller.KeyboardKey(key)
	case commandPointerScrollInProgress, commandPointerScrollFinished, commandPointerMove:
		var x, y int
		if len(arg) != 0 {
			if err := parseInts(arg, &x, &y); err != nil {
				return err
			}
		}
		switch command {
		case commandPointerScrollInProgress:
			return controller.PointerScroll(x, y, false)
		case commandPointerScrollFinished:
			return controller.PointerScroll(x, y, true)
		case commandPointerMove:
			return controller.PointerMove(x, y)
		default:
			panic("unreachable")
		}
	case commandPointerButton:
		var button inputcontrol.PointerButton
		var pressed int
		if err := parseInts(arg, (*int)(&button), &pressed); err != nil {
			return err
		}
		if button < 0 || button >= inputcontrol.PointerButtonLimit {
			return errors.New("unsupported pointer button")
		}
		return controller.PointerButton(button, pressed != 0)
	case commandPointerJoystickMove:
		var x, y int
		if len(arg) != 0 {
			if err := parseInts(arg, &x, &y); err != nil {
				return err
			}
		}
		return applyJoystickMove(controller, js, float64(x), float64(y))
	case commandJoystickConfig:
		var modeNum, gain, deadzone, acceleration float64
		if err := parseFloats(arg, &modeNum, &gain, &deadzone, &acceleration); err != nil {
			return err
		}
		if gain <= 0 || deadzone < 0 || acceleration < 0 {
			return errors.New("invalid joystick configuration")
		}
		mode := "trackpad"
		if modeNum != 0 {
			mode = "joystick"
		}
		js.gain, js.deadzone, js.acceleration = gain, deadzone, acceleration
		store.update(persistentSettings{
			MouseMode:            mode,
			MoveSpeed:            gain,
			JoystickDeadzone:     deadzone,
			JoystickAcceleration: acceleration,
		})
		return nil
	default:
		return errors.New("unsupported command")
	}
}

type challenge struct {
	message, expectedResponse string
}

func (c challenge) verify(response string) bool {
	return c.expectedResponse == response
}

func authenticationChallengeGenerator(secret string, challenges chan<- challenge) {
	unsecureSource := mathrand.NewSource(time.Now().UnixNano())
	unsecureRand := mathrand.New(unsecureSource)
	b := make([]byte, challengeLength)
	for {
		if _, err := unsecureRand.Read(b[:]); err != nil {
			log.Fatal(err)
		}
		message := base64.StdEncoding.EncodeToString(b[:])
		mac := hmac.New(sha256.New, []byte(message))
		mac.Write([]byte(secret))
		challenges <- challenge{
			message:          message,
			expectedResponse: base64.StdEncoding.EncodeToString(mac.Sum(nil)),
		}
		time.Sleep(authenticationRateLimit)
	}
}

func secureRandBase64(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b[:]); err != nil {
		log.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b[:])
}

func main() {
	terminal.SetTitle(prettyAppName)
	settingsPath, settingsPathErr := settingsFilePath()
	persisted := defaultPersistentSettings()
	if settingsPathErr != nil {
		log.Printf("settings won't persist across restarts: %v", settingsPathErr)
	} else if loaded, err := loadPersistentSettings(settingsPath); err == nil {
		persisted = loaded
	}
	var bind, certFile, keyFile, secret string
	var showVersion, trustedMode bool
	var config config
	flag.BoolVar(&showVersion, "version", false, "show program's version number and exit")
	flag.StringVar(&bind, "bind", defaultBind, "bind server to [HOSTNAME]:PORT")
	flag.StringVar(&secret, "secret", "", "shared secret for client authentication")
	flag.BoolVar(&trustedMode, "trusted", false, "trusted mode: no secret required, for a fixed, bookmarkable URL (only use on trusted networks)")
	flag.StringVar(&certFile, "cert", "", "file containing TLS certificate")
	flag.StringVar(&keyFile, "key", "", "file containing TLS private key")
	flag.UintVar(&config.UpdateRate, "update-rate", 30, "number of updates per second")
	flag.Float64Var(&config.MoveSpeed, "move-speed", persisted.MoveSpeed, "move speed multiplier")
	flag.Float64Var(&config.ScrollSpeed, "scroll-speed", 1, "scroll speed multiplier")
	flag.Float64Var(&config.MouseMoveSpeed, "mouse-move-speed", 1, "mouse move speed multiplier")
	flag.Float64Var(&config.MouseScrollSpeed, "mouse-scroll-speed", 1, "mouse scroll speed multiplier")
	flag.StringVar(&config.MouseMode, "mouse-mode", persisted.MouseMode, "touch mouse control mode: \"trackpad\" or \"joystick\"")
	flag.Float64Var(&config.JoystickDeadzone, "joystick-deadzone", persisted.JoystickDeadzone, "joystick mode: deadzone radius in pixels (0-20)")
	flag.Float64Var(&config.JoystickAcceleration, "joystick-acceleration", persisted.JoystickAcceleration, "joystick mode: acceleration factor while held (0 disables)")
	flag.Parse()
	if showVersion {
		fmt.Println(version)
		return
	}
	if config.MouseMode != "trackpad" && config.MouseMode != "joystick" {
		log.Fatal(`mouse mode must be "trackpad" or "joystick"`)
	}
	if trustedMode && secret != "" {
		log.Fatal("-secret cannot be combined with -trusted")
	}
	if certFile != "" && keyFile == "" {
		log.Fatal("TLS private key file missing")
	}
	if certFile == "" && keyFile != "" {
		log.Fatal("TLS certificate file missing")
	}
	tls := certFile != "" && keyFile != ""
	if !trustedMode && secret == "" {
		secret = secureRandBase64(defaultSecretLength)
	}
	if len(inputcontrol.Controllers) == 0 {
		log.Fatal("compiled without controller")
	}
	var controller inputcontrol.Controller
	var controllerName string
	var platformErrs []error
	for _, controllerInfo := range inputcontrol.Controllers {
		controllerName = controllerInfo.Name
		var err error
		controller, err = controllerInfo.Init()
		if err == nil {
			break
		} else {
			wrappedErr := fmt.Errorf("%v controller: %w", controllerName, err)
			if _, ok := errors.AsType[*inputcontrol.UnsupportedPlatformError](err); ok {
				platformErrs = append(platformErrs, wrappedErr)
			} else {
				log.Fatal(wrappedErr)
			}
		}
	}
	if controller == nil {
		log.Fatal(fmt.Errorf("unsupported platform:\n%w", errors.Join(platformErrs...)))
	}
	defer controller.Close()
	store := newSettingsStore(settingsPath, persistentSettings{
		MouseMode:            config.MouseMode,
		MoveSpeed:            config.MoveSpeed,
		JoystickDeadzone:     config.JoystickDeadzone,
		JoystickAcceleration: config.JoystickAcceleration,
	})
	authenticationChallenges := make(chan challenge, authenticationRateBurst)
	go authenticationChallengeGenerator(secret, authenticationChallenges)
	listener, err := net.Listen("tcp", bind)
	if err != nil {
		log.Fatal(err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	host := ""
	bindHost, _, err := net.SplitHostPort(bind)
	if err != nil {
		log.Fatal(err)
	}
	for _, b := range addr.IP {
		if b != 0 {
			host = bindHost
			break
		}
	}
	if host == "" {
		host = findDefaultHost()
	}
	port := addr.Port
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(webdataFS)))
	mux.Handle("/ws", websocket.Handler(func(ws *websocket.Conn) {
		var message string
		challenge := <-authenticationChallenges
		websocket.Message.Send(ws, challenge.message)
		if err := websocket.Message.Receive(ws, &message); err != nil {
			return
		}
		if !challenge.verify(message) {
			return
		}
		settings := store.snapshot()
		connConfig := config
		connConfig.MouseMode = settings.MouseMode
		connConfig.MoveSpeed = settings.MoveSpeed
		connConfig.JoystickDeadzone = settings.JoystickDeadzone
		connConfig.JoystickAcceleration = settings.JoystickAcceleration
		websocket.JSON.Send(ws, connConfig)
		js := &joystickState{
			gain:         settings.MoveSpeed,
			deadzone:     settings.JoystickDeadzone,
			acceleration: settings.JoystickAcceleration,
			lastUpdate:   time.Now(),
		}
		for {
			if err := websocket.Message.Receive(ws, &message); err != nil {
				return
			}
			if err := processCommand(controller, js, store, message); err != nil {
				log.Print(fmt.Errorf("%s controller: %w", controllerName, err))
				return
			}
		}
	}))
	domain := host
	if port != 80 && !tls || port != 443 && tls {
		domain = net.JoinHostPort(host, strconv.Itoa(port))
	}
	scheme := "http"
	if tls {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s/", scheme, domain)
	if secret != "" {
		url += "#" + secret
	}
	fmt.Println(url)
	if qrCode, err := terminal.GenerateQRCode(url, terminal.SupportsColor(os.Stdout.Fd())); err == nil {
		fmt.Print(qrCode)
	} else {
		log.Printf("QR code error: %v", err)
	}
	if !tls {
		fmt.Println("▌   WARNING: TLS is not enabled    ▐")
		fmt.Println("▌Don't use in an untrusted network!▐")
	}
	if trustedMode {
		fmt.Println("▌     WARNING: Trusted mode enabled      ▐")
		fmt.Println("▌ No secret required - anyone who can    ▐")
		fmt.Println("▌ reach this address has full control    ▐")
		if _, bindPort, err := net.SplitHostPort(bind); err == nil && bindPort == "0" {
			log.Print("Note: -bind uses an automatically assigned port; " +
				"use e.g. -bind :8080 for a stable URL")
		}
	}
	if tls {
		err = http.ServeTLS(listener, mux, certFile, keyFile)
	} else {
		err = http.Serve(listener, mux)
	}
	log.Fatal(err)
}
