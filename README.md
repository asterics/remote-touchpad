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

### Voice commands ("push to talk")

The web GUI has a microphone button (🎤) on the touchpad screen for push-to-talk voice
commands: hold the button while speaking, release when done. Speech-to-text
happens in the browser; the recognized phrase is sent to the server, which
fuzzily matches it against a small set of fixed commands (tolerating extra
words and minor misrecognitions, picking whichever configured phrase is
closest) and runs the associated action. A short result or error message is
shown above the button. Commands are configured in a JSON file, see config/Readme.md for details. 

To avoid accidentally triggering a command from a very short/accidental
press, a "Minimum hold time" slider in the settings panel (0-7s, default
0.3s) keeps listening for that long even if the button/key is released
sooner, instead of cutting the recording short; this value is stored in the
browser only.

A physical key (e.g. a button on a paired Bluetooth keyboard or an
accessibility switch) can trigger push-to-talk too: in the settings panel,
tap "Push-to-talk key", then press the desired key to bind it (Esc cancels).
The binding is stored in the browser only (not on the server). While a key is
bound, it activates push-to-talk everywhere except while typing in a text
field or adjusting a settings control, so it won't interfere with normal use.

The recognition language defaults to the browser's own language, but can be
overridden with the "Voice language" dropdown in the settings panel (e.g.
English or German); this choice is also stored in the browser only.

#### Microphone requirements

* Voice commands rely on the non-standard `webkitSpeechRecognition` /
  `SpeechRecognition` API, currently only available in Chromium-based
  browsers (Chrome, Edge, etc., desktop and Android). The 🎤 button is
  automatically hidden if the browser doesn't support it.
* Like all microphone access, it only works in a
  [secure context](https://developer.mozilla.org/en-US/docs/Web/Security/Secure_Contexts):
  either enable TLS (`-cert`/`-key`) so the page is served over `https://`,
  or connect from `http://localhost`. A plain `http://` connection to the
  server's LAN address (the common case for this app) will **not** be
  allowed to use the microphone by the browser.
* The browser will prompt for microphone permission on first use; it must be
  granted for the current origin (i.e. re-granted if the URL/port changes).
* Speech recognition typically requires an internet connection, since most
  browsers perform the speech-to-text processing in the cloud.

#### Enabling HTTPS with mkcert

TLS is enabled with the existing `-cert`/`-key` flags, which expect a
certificate/key pair in PEM format. [mkcert](https://github.com/FiloSottile/mkcert)
is the easiest way to get one that's actually trusted (no browser warnings),
since it creates a local certificate authority (CA) and installs it into
your system/browser trust stores.

1. **Install mkcert** and create the local CA (once per PC):

   ```sh
   mkcert -install
   ```

   This installs the CA into your OS/browser trust store. On Windows, if it
   also fails to update a Java `cacerts` keystore with a permission error,
   that's harmless and can be ignored unless you specifically need Java
   applications to trust it.

2. **Generate a certificate** for the address your phone will connect to
   (both the LAN IP and hostname, if you use one):

   ```sh
   mkcert 192.168.1.20 my-pc.local localhost 127.0.0.1
   ```

   This creates two files, e.g. `192.168.1.20+3.pem` (certificate) and
   `192.168.1.20+3-key.pem` (private key).

3. **Start the server** with a fixed port and the generated files:

   ```sh
   remote-touchpad -bind :8080 -cert 192.168.1.20+3.pem -key 192.168.1.20+3-key.pem
   ```

   The printed URL now starts with `https://`.

4. **Trust the CA on your phone too**, otherwise its browser won't
   recognize the certificate and will show a warning until you install the
   same CA there:

   * Find the CA file: `mkcert -CAROOT` prints the folder containing
     `rootCA.pem`.
   * Transfer `rootCA.pem` to the phone (e.g. email it to yourself, AirDrop,
     USB, or serve the folder with `python -m http.server` and download it
     from the phone's browser).
   * Install it as a trusted CA:
     * **Android**: Settings → Security → *Encryption & credentials* →
       *Install a certificate* → *CA certificate* → select the file.
     * **iOS**: open the file to install the profile (Settings → General →
       VPN & Device Management), then enable full trust under
       Settings → General → About → Certificate Trust Settings.

   Once the CA is trusted on the phone, connections succeed without any
   manual "proceed anyway" exception, and the server won't log TLS handshake
   errors from that device anymore.


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
