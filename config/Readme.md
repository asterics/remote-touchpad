
# demo config file

for voice command definitions.

`commands.json` is expected in the OS user config directory (e.g.
`~/.config/remote-touchpad/commands.json` on Linux, `%AppData%\remote-touchpad\commands.json`
on Windows), overridable with `-commands-file`. If the file doesn't exist,
voice commands are simply disabled. 


### Json format:

```json
[
    {
        "phrases": ["What is your name", "say name"],
        "action": "print",
        "text": "My name is Asterics"   // string to be printed (using key injection)
    },
    {
        "phrases": ["turn off light"],
        "action": "uart",
        "port": "COM11",
        "baudRate": 9600,
        "text": "LIGHT_OFF\n"      // string to be sent over the serial port (with optional newline)
    }
]
```

### Each entry has:

* `phrases`: list of accepted spoken phrases (matched as a substring of what
  was recognized, so minor extra words are tolerated).
* `action`: either `"print"` (types `text` on the remote computer, as if
  entered on the keyboard) or `"uart"` (sends `text` as raw bytes over the
  serial port given by `port`, e.g. `"COM11"` on Windows or
  `"/dev/ttyACM0"` on Linux/macOS, at `baudRate`, default `9600`).
* `text`: the string to type or send.
* `port` / `baudRate`: only required/used for the `"uart"` action.
