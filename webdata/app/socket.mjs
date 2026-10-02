/*
 *    Copyright (c) 2018-2019, 2023 Unrud <unrud@outlook.com>
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

import jsSHA from "./sha256.mjs"

const challengeResponse = (message, secret) => {
    const shaObj = new jsSHA("SHA-256", "TEXT");
    shaObj.setHMACKey(message, "TEXT");
    shaObj.update(secret);
    return btoa(shaObj.getHMAC("BYTES"));
};

const COMMAND_RTC_OFFER = "r";
const ICE_GATHER_TIMEOUT = 2000;
// Drop datagrams instead of queueing them when the link is congested.
const MAX_BUFFERED_BYTES = 2048;

export default class Socket extends EventTarget {
    #secret;
    #authenticated;
    #ws;
    #pc = null;
    #dc = null;

    constructor(url, secret) {
        super();
        this.#secret = secret;
        this.#authenticated = false;
        this.#ws = new WebSocket(url);
        this.#ws.addEventListener("message", this.#handle_ws_message.bind(this));
        this.#ws.addEventListener("close", this.#handle_ws_close.bind(this));
    }

    #handle_ws_message(event) {
        if (!this.#authenticated) {
            this.#ws.send(challengeResponse(event.data, this.#secret));
            this.#authenticated = true;
            return;
        }
        let message;
        try {
            message = JSON.parse(event.data);
        } catch (e) {
            this.#ws.close();
            throw (e);
        }
        if (message && typeof message == "object" && "voiceFeedback" in message) {
            this.dispatchEvent(new CustomEvent("voice-feedback", {detail: message.voiceFeedback}));
            return;
        }
        if (message && typeof message == "object" && "rtcAnswer" in message) {
            if (this.#pc) {
                this.#pc.setRemoteDescription({type: "answer", sdp: message.rtcAnswer}).catch(console.warn);
            }
            return;
        }
        this.dispatchEvent(new CustomEvent("config", {detail: message}));
        if (!this.#pc) {
            this.#setupUnreliable().catch(console.warn);
        }
    }

    #handle_ws_close() {
        this.#dc = null;
        if (this.#pc) {
            this.#pc.close();
        }
        this.dispatchEvent(new CustomEvent("close"));
    }

    // Optional low-latency path (UDP): on any failure everything keeps working over the WebSocket.
    async #setupUnreliable() {
        if (typeof RTCPeerConnection === "undefined") {
            return;
        }
        const pc = new RTCPeerConnection();
        this.#pc = pc;
        const dc = pc.createDataChannel("move", {ordered: false, maxRetransmits: 0});
        dc.addEventListener("open", () => {
            this.#dc = dc;
        });
        dc.addEventListener("close", () => {
            if (this.#dc === dc) {
                this.#dc = null;
            }
        });
        await pc.setLocalDescription(await pc.createOffer());
        await new Promise((resolve) => {
            if (pc.iceGatheringState == "complete") {
                resolve();
                return;
            }
            const timeout = setTimeout(resolve, ICE_GATHER_TIMEOUT);
            pc.addEventListener("icegatheringstatechange", () => {
                if (pc.iceGatheringState == "complete") {
                    clearTimeout(timeout);
                    resolve();
                }
            });
        });
        this.#ws.send(COMMAND_RTC_OFFER + pc.localDescription.sdp);
    }

    send(message) {
        this.#ws.send(message);
    }

    get unreliableOpen() {
        return this.#dc !== null && this.#dc.readyState == "open";
    }

    sendUnreliable(message) {
        if (this.unreliableOpen && this.#dc.bufferedAmount <= MAX_BUFFERED_BYTES) {
            this.#dc.send(message);
        }
    }
}
