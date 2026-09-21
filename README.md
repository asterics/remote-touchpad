# Remote Touchpad

Control mouse and keyboard from the webbrowser of a smartphone
(or any other device with a touchscreen).
To take control open the displayed URL or scan the QR code.

Supports Flatpak's RemoteDesktop portal (for Wayland), X11, macOS and Windows.

## About this fork

This fork adds a settings panel to the web GUI with:

* A mouse control mode switch between the original **trackpad** behavior
  (relative dragging) and a new **joystick** mode: touching the screen sets a
  zero point, and moving the finger away from it drives a proportional,
  joystick-like cursor movement (computed server-side) until the finger is
  lifted. Clicking and dragging behave exactly as before.
* An adjustable **mouse speed** (gain, 10%-1000%, via slider).
* An adjustable **deadzone** (0-20 pixels) around the joystick zero point.
* An adjustable **acceleration factor**, so the cursor moves faster the
  longer the finger is held away from the zero point.

Settings changed in the web GUI are saved on the server and are still in
effect after restarting it.

A new **trusted mode** (`-trusted` flag) disables the per-run secret, so the
server can be reached under a fixed, bookmarkable URL (combine with a fixed
port, e.g. `-bind :8080`). Only use this on networks you trust, since anyone
who can reach the address gets full control without authentication.

## Installation

* [Flatpak](https://flathub.org/apps/details/com.github.unrud.RemoteTouchpad)
* [Snap](https://snapcraft.io/remote-touchpad)
* [macOS & Windows](https://github.com/Unrud/remote-touchpad/releases/latest)
* Golang:
  * Portal & uinput & X11:

    ```sh
    go install -tags portal,uinput,x11 github.com/unrud/remote-touchpad@latest
    ```
  * Windows:

    ```sh
    go install github.com/unrud/remote-touchpad@latest
    ```

## Screenshots

![screenshot 1](https://raw.githubusercontent.com/Unrud/remote-touchpad/master/screenshots/1.png)

![screenshot 2](https://raw.githubusercontent.com/Unrud/remote-touchpad/master/screenshots/2.png)

![screenshot 3](https://raw.githubusercontent.com/Unrud/remote-touchpad/master/screenshots/3.png)

![screenshot 4](https://raw.githubusercontent.com/Unrud/remote-touchpad/master/screenshots/4.png)
