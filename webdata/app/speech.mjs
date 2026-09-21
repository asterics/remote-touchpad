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
const VOICE_LANG_STORAGE_KEY = "voiceLang";
const PTT_MIN_DURATION_STORAGE_KEY = "pttMinDuration";

const button = document.getElementById("speak-button");
const status = document.getElementById("voice-status");
const pttKeyButton = document.getElementById("ptt-key-button");
const voiceLangSetting = document.getElementById("voice-lang-setting");
const voiceLangSelect = document.getElementById("settings-voice-lang");
const minDurationSetting = document.getElementById("ptt-min-duration-setting");
const minDurationRange = document.getElementById("settings-ptt-min-duration");
const minDurationValue = document.getElementById("settings-ptt-min-duration-value");

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
    #pressStartTime = 0;
    #held = false;
    #minHoldTimeout = null;

    constructor(inputController) {
        this.#inputController = inputController;
        if (!SpeechRecognitionImpl) {
            button.classList.add("hidden");
            voiceLangSetting.classList.add("hidden");
            minDurationSetting.classList.add("hidden");
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
        voiceLangSelect.value = localStorage.getItem(VOICE_LANG_STORAGE_KEY) || "";
        voiceLangSelect.addEventListener("change", () => {
            localStorage.setItem(VOICE_LANG_STORAGE_KEY, voiceLangSelect.value);
        });
        const storedMinDuration = parseInt(localStorage.getItem(PTT_MIN_DURATION_STORAGE_KEY), 10);
        if (Number.isFinite(storedMinDuration)) {
            minDurationRange.value = storedMinDuration;
        }
        this.#updateMinDurationLabel();
        minDurationRange.addEventListener("input", () => {
            localStorage.setItem(PTT_MIN_DURATION_STORAGE_KEY, minDurationRange.value);
            this.#updateMinDurationLabel();
        });
    }

    showFeedback(text) {
        status.textContent = text;
        status.classList.remove("hidden");
        clearTimeout(this.#statusTimeout);
        this.#statusTimeout = setTimeout(() => status.classList.add("hidden"), FEEDBACK_TIMEOUT);
    }

    #handleStart(event) {
        event.preventDefault();
        this.#held = true;
        if (this.#active) {
            return;
        }
        this.#active = true;
        this.#pressStartTime = Date.now();
        button.classList.add("listening");
        this.#recognition = new SpeechRecognitionImpl();
        this.#recognition.lang = voiceLangSelect.value || navigator.language || "en-US";
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

    // Keeps listening past the release until the configured minimum hold time
    // has elapsed, instead of cutting off a command that was spoken quickly.
    #handleStop() {
        this.#held = false;
        if (!this.#active) {
            return;
        }
        const minDuration = parseInt(minDurationRange.value, 10) || 0;
        const remaining = minDuration - (Date.now() - this.#pressStartTime);
        clearTimeout(this.#minHoldTimeout);
        if (remaining > 0) {
            this.#minHoldTimeout = setTimeout(this.#finishListening.bind(this), remaining);
            return;
        }
        this.#finishListening();
    }

    #finishListening() {
        if (this.#held) {
            return; // pressed again before the minimum hold time elapsed
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

    #updateMinDurationLabel() {
        minDurationValue.textContent = `${(parseInt(minDurationRange.value, 10) / 1000).toFixed(1)}s`;
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
