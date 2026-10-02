/*
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
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
	"github.com/unrud/remote-touchpad/inputcontrol"
)

const (
	// Browsers can't send raw UDP; an unordered, zero-retransmit WebRTC data channel
	// is UDP on the wire (SCTP/DTLS) and avoids TCP head-of-line blocking and retransmit bursts.
	unreliableChannelLabel string        = "move"
	commandRTCOffer        byte          = 'r'
	iceGatherTimeout       time.Duration = 3 * time.Second
)

// rtcAnswerMessage is sent to the client in response to a WebRTC offer.
type rtcAnswerMessage struct {
	RTCAnswer string `json:"rtcAnswer"`
}

// unreliableState tracks the datagram stream of one connection. Not safe for concurrent use.
type unreliableState struct {
	lastSeq      int64
	lastX, lastY int
}

// processUnreliable handles a datagram of the form "<m|j><seq>;<a>;<b>". Datagrams may be lost,
// duplicated or reordered. Stale ones are dropped; pointer moves carry running totals, so a lost
// datagram is made up for by the next one.
func processUnreliable(controller inputcontrol.Controller, js *joystickState, us *unreliableState, msg string) error {
	if len(msg) == 0 {
		return errors.New("empty datagram")
	}
	parts := strings.Split(msg[1:], ";")
	if len(parts) != 3 {
		return errors.New("wrong number of arguments")
	}
	seq, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return err
	}
	a, err := strconv.Atoi(parts[1])
	if err != nil {
		return err
	}
	b, err := strconv.Atoi(parts[2])
	if err != nil {
		return err
	}
	if seq <= us.lastSeq {
		return nil
	}
	us.lastSeq = seq
	switch msg[0] {
	case commandPointerMove:
		dx, dy := a-us.lastX, b-us.lastY
		us.lastX, us.lastY = a, b
		if dx == 0 && dy == 0 {
			return nil
		}
		return controller.PointerMove(dx, dy)
	case commandPointerJoystickMove:
		js.setOffset(float64(a), float64(b))
		return nil
	default:
		return errors.New("unsupported command")
	}
}

// answerRTCOffer completes the WebRTC handshake for the browser's offer (signaled over the
// authenticated WebSocket) and passes datagrams from the data channel to onDatagram.
// The returned function closes the peer connection.
func answerRTCOffer(offerSDP string, onDatagram func(string)) (string, func(), error) {
	settings := webrtc.SettingEngine{}
	settings.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4, webrtc.NetworkTypeUDP6})
	// Browsers may hide their address behind an mDNS name; the actual one is learned from the probe packets.
	settings.SetICEMulticastDNSMode(ice.MulticastDNSModeQueryOnly)
	pc, err := webrtc.NewAPI(webrtc.WithSettingEngine(settings)).NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return "", nil, err
	}
	fail := func(err error) (string, func(), error) {
		pc.Close()
		return "", nil, err
	}
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != unreliableChannelLabel {
			return
		}
		dc.OnMessage(func(m webrtc.DataChannelMessage) {
			if m.IsString {
				onDatagram(string(m.Data))
			}
		})
	})
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		if s == webrtc.PeerConnectionStateFailed {
			pc.Close()
		}
	})
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSDP}); err != nil {
		return fail(err)
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return fail(err)
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		return fail(err)
	}
	select {
	case <-gathered:
	case <-time.After(iceGatherTimeout):
	}
	return pc.LocalDescription().SDP, func() { pc.Close() }, nil
}
