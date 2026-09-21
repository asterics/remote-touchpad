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

// Only available (often webkit-prefixed) in Chromium-based browsers, and only
// in a secure context (HTTPS or localhost) with microphone permission granted.
const SpeechRecognitionImpl = window.SpeechRecognition || window.webkitSpeechRecognition;

const FEEDBACK_TIMEOUT = 4000; // milliseconds
const PTT_KEY_STORAGE_KEY = "pttKey";

const button = document.getElementById("speak-button");
const status = document.getElementById("voice-status");
const pttKeyButton = document.getElementById("ptt-key-button");

const isTypingTarget = (element) =>
    element && (element.tagName == "INPUT" || element.tagName == "TEXTAREA" || element.tagName == "SELECT");

const formatKeyCode = (code) => {
    if (!code) {
        return "Not set";
    }
    if (code.startsWith("Key")) {
        return code.slice(3);
    }
    if (code.startsWith("Digit")) {
        return code.slice(5);
    }
    return code;
};

export default class Speech {
    #inputController;
    #recognition = null;
    #active = false;
    #statusTimeout = null;
    #pttCode = localStorage.getItem(PTT_KEY_STORAGE_KEY) || null;
    #binding = false;

    constructor(inputController) {
        this.#inputController = inputController;
        if (!SpeechRecognitionImpl) {
            button.classList.add("hidden");
            return;
        }
        button.addEventListener("touchstart", this.#handleStart.bind(this));
        button.addEventListener("mousedown", this.#handleStart.bind(this));
        button.addEventListener("touchend", this.#handleStop.bind(this));
        button.addEventListener("touchcancel", this.#handleStop.bind(this));
        button.addEventListener("mouseup", this.#handleStop.bind(this));
        button.addEventListener("mouseleave", this.#handleStop.bind(this));
        button.addEventListener("contextmenu", (event) => event.preventDefault());
        // Capture phase, so a bound key is consumed here before Keyboard's
        // bubble-phase listener could also type it on the remote computer.
        document.addEventListener("keydown", this.#handlePttKeydown.bind(this), {capture: true});
        document.addEventListener("keyup", this.#handlePttKeyup.bind(this), {capture: true});
        pttKeyButton.addEventListener("click", this.#handlePttKeyButtonClick.bind(this));
        this.#updatePttKeyButton();
    }

    showFeedback(text) {
        status.textContent = text;
        status.classList.remove("hidden");
        clearTimeout(this.#statusTimeout);
        this.#statusTimeout = setTimeout(() => status.classList.add("hidden"), FEEDBACK_TIMEOUT);
    }

    #handleStart(event) {
        event.preventDefault();
        if (this.#active) {
            return;
        }
        this.#active = true;
        button.classList.add("listening");
        this.#recognition = new SpeechRecognitionImpl();
        this.#recognition.lang = navigator.language || "en-US";
        this.#recognition.interimResults = true;
        this.#recognition.continuous = true;
        this.#recognition.addEventListener("result", this.#handleResult.bind(this));
        this.#recognition.addEventListener("error", (event) => {
            this.showFeedback(`Speech error: ${event.error}`);
        });
        this.#recognition.addEventListener("end", () => {
            // Some browsers stop recognition after a short pause; restart while held.
            if (this.#active) {
                try {
                    this.#recognition.start();
                } catch {
                    // ignore, e.g. already starting
                }
            }
        });
        try {
            this.#recognition.start();
            this.showFeedback("Listening…");
        } catch {
            this.showFeedback("Could not start microphone");
        }
    }

    #handleResult(event) {
        const result = event.results[event.results.length - 1];
        const transcript = result[0].transcript.trim();
        if (!result.isFinal) {
            this.showFeedback(transcript);
            return;
        }
        if (transcript) {
            this.#inputController.voiceCommand(transcript);
            this.showFeedback(`Heard: "${transcript}"`);
        }
    }

    #handleStop() {
        if (!this.#active) {
            return;
        }
        this.#active = false;
        button.classList.remove("listening");
        if (this.#recognition) {
            this.#recognition.stop();
            this.#recognition = null;
        }
    }

    #updatePttKeyButton() {
        pttKeyButton.textContent = this.#binding ? "Press a key… (Esc to cancel)" : formatKeyCode(this.#pttCode);
    }

    #handlePttKeyButtonClick() {
        if (this.#binding) {
            return;
        }
        this.#binding = true;
        this.#updatePttKeyButton();
        const captureNextKey = (event) => {
            event.preventDefault();
            event.stopPropagation();
            this.#binding = false;
            if (event.code != "Escape") {
                this.#pttCode = event.code;
                localStorage.setItem(PTT_KEY_STORAGE_KEY, this.#pttCode);
            }
            this.#updatePttKeyButton();
        };
        document.addEventListener("keydown", captureNextKey, {capture: true, once: true});
    }

    #handlePttKeydown(event) {
        if (this.#binding || !this.#pttCode || event.code != this.#pttCode ||
            isTypingTarget(document.activeElement)) {
            return;
        }
        event.preventDefault();
        event.stopPropagation();
        if (!event.repeat) {
            this.#handleStart(event);
        }
    }

    #handlePttKeyup(event) {
        if (this.#binding || !this.#pttCode || event.code != this.#pttCode ||
            isTypingTarget(document.activeElement)) {
            return;
        }
        event.preventDefault();
        event.stopPropagation();
        this.#handleStop(event);
    }
}
