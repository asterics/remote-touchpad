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

const clamp = (value, min, max) => Math.min(Math.max(value, min), max);

const modeSelect = document.getElementById("settings-mouse-mode");
const speedRange = document.getElementById("settings-mouse-speed");
const speedValue = document.getElementById("settings-mouse-speed-value");
const deadzoneRange = document.getElementById("settings-deadzone");
const deadzoneValue = document.getElementById("settings-deadzone-value");
const accelerationRange = document.getElementById("settings-acceleration");
const accelerationValue = document.getElementById("settings-acceleration-value");
const joystickOnlyElements = document.querySelectorAll(".joystick-only");

// The server persists settings across restarts, so it's the single source of truth;
// this class only mirrors the current values into the form and reports user edits.
export default class Settings {
    #values = {
        mouseMode: "trackpad",
        moveSpeed: 1,
        joystickDeadzone: 4,
        joystickAcceleration: 1,
    };
    #onChange;

    constructor(onChange) {
        this.#onChange = onChange;
        modeSelect.addEventListener("change", this.#handleChange.bind(this));
        speedRange.addEventListener("input", this.#handleChange.bind(this));
        deadzoneRange.addEventListener("input", this.#handleChange.bind(this));
        accelerationRange.addEventListener("input", this.#handleChange.bind(this));
        this.#updateInputs();
        this.#updateVisibility();
    }

    get values() {
        return this.#values;
    }

    applyServerDefaults(config) {
        this.#values.mouseMode = config.mouseMode;
        this.#values.moveSpeed = config.moveSpeed;
        this.#values.joystickDeadzone = config.joystickDeadzone;
        this.#values.joystickAcceleration = config.joystickAcceleration;
        this.#updateInputs();
        this.#updateVisibility();
    }

    #updateInputs() {
        modeSelect.value = this.#values.mouseMode;
        speedRange.value = Math.round(this.#values.moveSpeed * 100);
        speedValue.textContent = `${speedRange.value}%`;
        deadzoneRange.value = this.#values.joystickDeadzone;
        deadzoneValue.textContent = `${this.#values.joystickDeadzone}px`;
        accelerationRange.value = this.#values.joystickAcceleration;
        accelerationValue.textContent = Number(this.#values.joystickAcceleration).toFixed(1);
    }

    #updateVisibility() {
        const joystick = this.#values.mouseMode == "joystick";
        for (const element of joystickOnlyElements) {
            element.classList.toggle("hidden", !joystick);
        }
    }

    #handleChange() {
        this.#values.mouseMode = modeSelect.value;
        this.#values.moveSpeed = clamp(parseInt(speedRange.value, 10), 10, 1000) / 100;
        this.#values.joystickDeadzone = clamp(parseInt(deadzoneRange.value, 10), 0, 20);
        this.#values.joystickAcceleration = clamp(parseFloat(accelerationRange.value), 0, 10);
        this.#updateInputs();
        this.#updateVisibility();
        this.#onChange();
    }
}
